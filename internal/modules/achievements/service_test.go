package achievements

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"courtscore/internal/platform/sse"
)

type fakeStore struct {
	badges     map[string][]UserBadge
	goal       GoalConfig
	streak     Streak
	streaks    []streakRow
	reminders  map[string]bool
	candidates []ReminderCandidate
	pulseCands []PulseCandidate
	pulses     map[string]bool
	userIDs    []string
	insertErr  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		badges:    map[string][]UserBadge{},
		goal:      GoalConfig{WeeklyTarget: 0, ReminderOptIn: true},
		reminders: map[string]bool{},
	}
}

func (f *fakeStore) insertBadge(_ context.Context, userID, code string, meta json.RawMessage) (UserBadge, bool, error) {
	if f.insertErr != nil {
		return UserBadge{}, false, f.insertErr
	}
	for _, b := range f.badges[userID] {
		if b.BadgeCode == code {
			return b, false, nil
		}
	}
	b := UserBadge{BadgeCode: code, UnlockedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), Meta: meta}
	f.badges[userID] = append(f.badges[userID], b)
	return b, true, nil
}

func (f *fakeStore) listBadges(_ context.Context, userID string, limit, offset int) ([]UserBadge, int64, error) {
	all := f.badges[userID]
	if all == nil {
		all = []UserBadge{}
	}
	total := int64(len(all))
	if offset > len(all) {
		return []UserBadge{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (f *fakeStore) getGoal(context.Context, string) (GoalConfig, error) { return f.goal, nil }
func (f *fakeStore) upsertGoal(_ context.Context, _ string, weeklyTarget int, reminderOptIn bool) (GoalConfig, error) {
	f.goal = GoalConfig{WeeklyTarget: weeklyTarget, ReminderOptIn: reminderOptIn}
	return f.goal, nil
}
func (f *fakeStore) getStreak(context.Context, string) (Streak, error) { return f.streak, nil }
func (f *fakeStore) upsertStreak(_ context.Context, _ string, s Streak) error {
	f.streak = s
	return nil
}
func (f *fakeStore) listStreaks(context.Context) ([]streakRow, error) { return f.streaks, nil }
func (f *fakeStore) tryRecordReminder(_ context.Context, userID string, weekStart time.Time) (bool, error) {
	key := userID + weekStart.Format("2006-01-02")
	if f.reminders[key] {
		return false, nil
	}
	f.reminders[key] = true
	return true, nil
}
func (f *fakeStore) listReminderCandidates(context.Context, time.Time, time.Time) ([]ReminderCandidate, error) {
	return f.candidates, nil
}
func (f *fakeStore) listPulseCandidates(context.Context) ([]PulseCandidate, error) {
	return f.pulseCands, nil
}
func (f *fakeStore) tryRecordPulse(_ context.Context, userID string, weekStart time.Time) (bool, error) {
	if f.pulses == nil {
		f.pulses = map[string]bool{}
	}
	key := userID + weekStart.UTC().Format("2006-01-02")
	if f.pulses[key] {
		return false, nil
	}
	f.pulses[key] = true
	return true, nil
}
func (f *fakeStore) listParticipantUserIDs(context.Context) ([]string, error) {
	return f.userIDs, nil
}
func (f *fakeStore) countedMatches(context.Context, string) ([]CountedMatch, error) {
	return nil, nil
}
func (f *fakeStore) maxRating(context.Context, string) (int, error)     { return 0, nil }
func (f *fakeStore) championCount(context.Context, string) (int, error) { return 0, nil }

type fakeMatches struct {
	matches map[string][]CountedMatch
	rating  int
	ids     []string
}

func (f *fakeMatches) CountedMatches(_ context.Context, userID string) ([]CountedMatch, error) {
	return f.matches[userID], nil
}
func (f *fakeMatches) MaxRating(context.Context, string) (int, error) { return f.rating, nil }
func (f *fakeMatches) ListParticipantUserIDs(context.Context) ([]string, error) {
	return f.ids, nil
}

type fakeTours struct{ n int }

func (f *fakeTours) ChampionCountForUser(context.Context, string) (int, error) { return f.n, nil }

type fakeFriends struct{ ok bool }

func (f *fakeFriends) AreFriends(context.Context, string, string) (bool, error) { return f.ok, nil }

type fakePush struct {
	user string
	typ  string
	n    int
}

func (f *fakePush) SendLocalizedToUser(_ context.Context, userID, typ, _, _ string, _ []string, _ map[string]string) error {
	f.user, f.typ, f.n = userID, typ, f.n+1
	return nil
}

func newTestService(store *fakeStore, matches *fakeMatches) *Service {
	if store == nil {
		store = newFakeStore()
	}
	if matches == nil {
		matches = &fakeMatches{matches: map[string][]CountedMatch{}}
	}
	return &Service{
		repo:    store,
		matches: matches,
		tours:   &fakeTours{},
		friends: &fakeFriends{ok: true},
		hub:     sse.NewHub(),
		now:     func() time.Time { return time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC) },
	}
}

func TestOnMatchFinished_PersistsAndPublishes(t *testing.T) {
	store := newFakeStore()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	matches := &fakeMatches{matches: map[string][]CountedMatch{
		"u1": {match("m1", true, now)},
	}, rating: 1500}
	svc := newTestService(store, matches)
	ch, unsub := svc.hub.Subscribe("u1")
	defer unsub()

	svc.OnMatchFinished(context.Background(), []string{"u1", "u1", ""}, "TENNIS")

	if len(store.badges["u1"]) < 2 {
		t.Fatalf("badges=%v", store.badges["u1"])
	}
	codes := map[string]bool{}
	for _, b := range store.badges["u1"] {
		codes[b.BadgeCode] = true
	}
	if !codes[BadgeFirstMatch] || !codes[BadgeFirstWin] {
		t.Fatalf("codes=%v", codes)
	}
	select {
	case ev := <-ch:
		if ev.Type != "achievement.unlocked" && ev.Type != "goal.progress" {
			t.Fatalf("type=%s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no SSE")
	}
	// drain remaining
	select {
	case <-ch:
	case <-time.After(50 * time.Millisecond):
	}

	// idempotent re-unlock
	svc.OnMatchFinished(context.Background(), []string{"u1"}, "TENNIS")
	if len(store.badges["u1"]) != len(codes) {
		t.Fatalf("duplicate unlocks: %v", store.badges["u1"])
	}
}

func TestSetGoal_ClampsValidation(t *testing.T) {
	svc := newTestService(nil, nil)
	_, err := svc.SetGoal(context.Background(), "u1", 15, true)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
	_, err = svc.SetGoal(context.Background(), "u1", -1, true)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
	got, err := svc.SetGoal(context.Background(), "u1", 3, false)
	if err != nil || got.WeeklyTarget != 3 || got.ReminderOptIn {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	got, err = svc.SetGoal(context.Background(), "u1", 0, true)
	if err != nil || got.WeeklyTarget != 0 {
		t.Fatalf("clear: %+v err=%v", got, err)
	}
}

func TestListUserBadges_FriendGate(t *testing.T) {
	store := newFakeStore()
	store.badges["u2"] = []UserBadge{{BadgeCode: BadgeFirstMatch, UnlockedAt: time.Now()}}
	svc := newTestService(store, nil)
	svc.friends = &fakeFriends{ok: false}

	_, _, err := svc.ListUserBadges(context.Background(), "u1", "u2", 20, 0)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
	self, total, err := svc.ListUserBadges(context.Background(), "u2", "u2", 20, 0)
	if err != nil || total != 1 || self[0].BadgeCode != BadgeFirstMatch {
		t.Fatalf("self=%v total=%d err=%v", self, total, err)
	}
	svc.friends = &fakeFriends{ok: true}
	friend, _, err := svc.ListUserBadges(context.Background(), "u1", "u2", 20, 0)
	if err != nil || len(friend) != 1 {
		t.Fatalf("friend=%v err=%v", friend, err)
	}
}
