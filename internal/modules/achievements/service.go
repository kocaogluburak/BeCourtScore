package achievements

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"courtscore/internal/platform/sse"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FriendChecker is the self-or-accepted-friend gate (implemented by social.Service).
type FriendChecker interface {
	AreFriends(ctx context.Context, userA, userB string) (bool, error)
}

// MatchReader supplies counted matches + Elo. Implemented by *repo in prod;
// tests substitute a fake. Defined so score.Service can satisfy it via adapter.
type MatchReader interface {
	CountedMatches(ctx context.Context, userID string) ([]CountedMatch, error)
	MaxRating(ctx context.Context, userID string) (int, error)
	ListParticipantUserIDs(ctx context.Context) ([]string, error)
}

// TournamentReader supplies champion counts (implemented by *repo / tournament adapter).
type TournamentReader interface {
	ChampionCountForUser(ctx context.Context, userID string) (int, error)
}

// LocalizedPusher is notify.Service.SendLocalizedToUser (i18n-clean reminder).
type LocalizedPusher interface {
	SendLocalizedToUser(ctx context.Context, userID, typ, titleLocKey, bodyLocKey string, locArgs []string, data map[string]string) error
}

// Service evaluates badges/goals/streaks and serves the HTTP surface.
type Service struct {
	repo   store
	matches MatchReader
	tours  TournamentReader
	friends FriendChecker
	hub    *sse.Hub
	push   LocalizedPusher
	now    func() time.Time
}

func NewService(pool *pgxpool.Pool, friends FriendChecker, hub *sse.Hub, push LocalizedPusher) *Service {
	r := &repo{pool: pool}
	return &Service{
		repo:    r,
		matches: repoMatchReader{r},
		tours:   repoTourReader{r},
		friends: friends,
		hub:     hub,
		push:    push,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

type repoMatchReader struct{ r *repo }

func (m repoMatchReader) CountedMatches(ctx context.Context, userID string) ([]CountedMatch, error) {
	return m.r.countedMatches(ctx, userID)
}
func (m repoMatchReader) MaxRating(ctx context.Context, userID string) (int, error) {
	return m.r.maxRating(ctx, userID)
}
func (m repoMatchReader) ListParticipantUserIDs(ctx context.Context) ([]string, error) {
	return m.r.listParticipantUserIDs(ctx)
}

type repoTourReader struct{ r *repo }

func (t repoTourReader) ChampionCountForUser(ctx context.Context, userID string) (int, error) {
	return t.r.championCount(ctx, userID)
}

func (s *Service) SetMatchReader(r MatchReader)         { s.matches = r }
func (s *Service) SetTournamentReader(r TournamentReader) { s.tours = r }
func (s *Service) SetNow(fn func() time.Time)           { s.now = fn }

// OnMatchFinished implements score.AchievementHook — best-effort, never fails the caller.
func (s *Service) OnMatchFinished(ctx context.Context, participantUserIDs []string, sport string) {
	seen := map[string]bool{}
	for _, id := range participantUserIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if err := s.evaluateUser(ctx, id, sport); err != nil {
			slog.Warn("achievements: evaluate after match failed", "user", id, "err", err)
		}
	}
}

func (s *Service) evaluateUser(ctx context.Context, userID, sport string) error {
	now := s.now()
	matches, err := s.matches.CountedMatches(ctx, userID)
	if err != nil {
		return err
	}
	maxRating, err := s.matches.MaxRating(ctx, userID)
	if err != nil {
		return err
	}
	champions := 0
	if s.tours != nil {
		champions, err = s.tours.ChampionCountForUser(ctx, userID)
		if err != nil {
			slog.Warn("achievements: champion count", "user", userID, "err", err)
			champions = 0
		}
	}
	goal, err := s.repo.getGoal(ctx, userID)
	if err != nil {
		return err
	}
	streak, err := s.rebuildStreak(ctx, userID, matches, now)
	if err != nil {
		return err
	}

	in := EvalInput{
		Matches:       matches,
		MaxRating:     maxRating,
		ChampionCount: champions,
		CurrentWeeks:  streak.CurrentWeeks,
		LongestWeeks:  streak.LongestWeeks,
		WeeklyTarget:  goal.WeeklyTarget,
	}
	earned := EarnedBadges(in)
	var unlocked []UserBadge
	for _, code := range earned {
		meta, _ := json.Marshal(map[string]any{"sport": sport})
		b, inserted, err := s.repo.insertBadge(ctx, userID, code, meta)
		if err != nil {
			return err
		}
		if inserted {
			unlocked = append(unlocked, b)
		}
	}
	s.publishUnlocks(userID, unlocked)

	weekStart, weekEnd := ISOWeek(now)
	current := matchesInWeek(matches, weekStart)
	reached := goal.WeeklyTarget > 0 && current >= goal.WeeklyTarget
	s.publishGoalProgress(userID, GoalProgressEvent{
		WeekStart: weekStart,
		Target:    goal.WeeklyTarget,
		Current:   current,
		Reached:   reached,
	})
	_ = weekEnd
	return nil
}

func (s *Service) rebuildStreak(ctx context.Context, userID string, matches []CountedMatch, now time.Time) (Streak, error) {
	var st Streak
	// Replay weeks in chronological order (unique ISO weeks).
	seen := map[time.Time]bool{}
	var weeks []time.Time
	for _, m := range matches {
		w := ISOWeekStart(m.PlayedAt)
		if !seen[w] {
			seen[w] = true
			weeks = append(weeks, w)
		}
	}
	for _, w := range weeks {
		st = ApplyStreak(st, w, now)
	}
	st = ReconcileStreak(st, now)
	if err := s.repo.upsertStreak(ctx, userID, st); err != nil {
		return Streak{}, err
	}
	return st, nil
}

func (s *Service) publishUnlocks(userID string, unlocked []UserBadge) {
	if s.hub == nil || len(unlocked) == 0 {
		return
	}
	items := make([]UnlockedEvent, 0, len(unlocked))
	for _, b := range unlocked {
		items = append(items, UnlockedEvent{BadgeCode: b.BadgeCode, UnlockedAt: b.UnlockedAt, Meta: b.Meta})
	}
	// One celebration event; first badge at the top level for simple clients.
	s.hub.Publish(userID, sse.Event{
		Type: "achievement.unlocked",
		Data: map[string]any{
			"badge_code":  items[0].BadgeCode,
			"unlocked_at": items[0].UnlockedAt,
			"meta":        items[0].Meta,
			"items":       items,
		},
	})
}

func (s *Service) publishGoalProgress(userID string, ev GoalProgressEvent) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(userID, sse.Event{Type: "goal.progress", Data: ev})
}

func (s *Service) Catalog() []CatalogEntry { return Catalog() }

func (s *Service) ListUserBadges(ctx context.Context, viewerID, targetID string, limit, offset int) ([]UserBadge, int64, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil, 0, fmt.Errorf("%w: user id required", ErrInvalid)
	}
	if viewerID != targetID {
		if s.friends == nil {
			return nil, 0, ErrForbidden
		}
		ok, err := s.friends.AreFriends(ctx, viewerID, targetID)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			return nil, 0, ErrForbidden
		}
	}
	return s.repo.listBadges(ctx, targetID, limit, offset)
}

func (s *Service) GetGoal(ctx context.Context, userID string) (GoalState, error) {
	// Lazy eval so existing users get credited on first fetch.
	if err := s.evaluateUser(ctx, userID, ""); err != nil {
		slog.Warn("achievements: lazy eval", "user", userID, "err", err)
	}
	return s.goalState(ctx, userID)
}

func (s *Service) SetGoal(ctx context.Context, userID string, weeklyTarget int, reminderOptIn bool) (GoalState, error) {
	if weeklyTarget < 0 || weeklyTarget > maxWeeklyTarget {
		return GoalState{}, fmt.Errorf("%w: weekly_target must be 0..%d", ErrInvalid, maxWeeklyTarget)
	}
	if _, err := s.repo.upsertGoal(ctx, userID, weeklyTarget, reminderOptIn); err != nil {
		return GoalState{}, err
	}
	if err := s.evaluateUser(ctx, userID, ""); err != nil {
		slog.Warn("achievements: eval after set goal", "user", userID, "err", err)
	}
	return s.goalState(ctx, userID)
}

func (s *Service) goalState(ctx context.Context, userID string) (GoalState, error) {
	now := s.now()
	goal, err := s.repo.getGoal(ctx, userID)
	if err != nil {
		return GoalState{}, err
	}
	streak, err := s.repo.getStreak(ctx, userID)
	if err != nil {
		return GoalState{}, err
	}
	matches, err := s.matches.CountedMatches(ctx, userID)
	if err != nil {
		return GoalState{}, err
	}
	start, end := ISOWeek(now)
	current := matchesInWeek(matches, start)
	return GoalState{
		WeeklyTarget:  goal.WeeklyTarget,
		ReminderOptIn: goal.ReminderOptIn,
		Week: WeekProgress{
			Start: start, End: end, Current: current, Target: goal.WeeklyTarget,
			Reached: goal.WeeklyTarget > 0 && current >= goal.WeeklyTarget,
		},
		Streak: streak,
	}, nil
}

// SendWeeklyReminders fires at most one reminder per user per ISO week when
// remaining days ≤ 3, progress < target, and opt-in is on.
func (s *Service) SendWeeklyReminders(ctx context.Context) error {
	now := s.now()
	if RemainingWeekDays(now) > 3 {
		return nil
	}
	weekStart, weekEnd := ISOWeek(now)
	cands, err := s.repo.listReminderCandidates(ctx, weekStart, weekEnd)
	if err != nil {
		return err
	}
	for _, c := range cands {
		if !c.ReminderOptIn || c.WeeklyTarget <= 0 || c.Current >= c.WeeklyTarget {
			continue
		}
		ok, err := s.repo.tryRecordReminder(ctx, c.UserID, weekStart)
		if err != nil || !ok {
			continue
		}
		if s.push == nil {
			continue
		}
		_ = s.push.SendLocalizedToUser(ctx, c.UserID, "goal.reminder",
			"goal_reminder_push_title", "goal_reminder_push_body",
			[]string{strconv.Itoa(c.Current), strconv.Itoa(c.WeeklyTarget)},
			map[string]string{
				"type":         "goal.reminder",
				"week_current": strconv.Itoa(c.Current),
				"week_target":  strconv.Itoa(c.WeeklyTarget),
			})
	}
	return nil
}

// ReconcileStreaks resets current_weeks for users whose last active week is stale.
func (s *Service) ReconcileStreaks(ctx context.Context) error {
	now := s.now()
	rows, err := s.repo.listStreaks(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		next := ReconcileStreak(row.Streak, now)
		if next.CurrentWeeks == row.Streak.CurrentWeeks {
			continue
		}
		if err := s.repo.upsertStreak(ctx, row.UserID, next); err != nil {
			slog.Warn("achievements: streak reconcile", "user", row.UserID, "err", err)
		}
	}
	return nil
}

// Backfill unlocks already-earned badges for every participant user (once at boot).
func (s *Service) Backfill(ctx context.Context) error {
	ids, err := s.matches.ListParticipantUserIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.evaluateUser(ctx, id, ""); err != nil {
			slog.Warn("achievements: backfill", "user", id, "err", err)
		}
	}
	return nil
}
