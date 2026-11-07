package achievements

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store interface {
	insertBadge(ctx context.Context, userID, code string, meta json.RawMessage) (UserBadge, bool, error)
	listBadges(ctx context.Context, userID string, limit, offset int) ([]UserBadge, int64, error)
	getGoal(ctx context.Context, userID string) (GoalConfig, error)
	upsertGoal(ctx context.Context, userID string, weeklyTarget int, reminderOptIn bool) (GoalConfig, error)
	getStreak(ctx context.Context, userID string) (Streak, error)
	upsertStreak(ctx context.Context, userID string, s Streak) error
	listStreaks(ctx context.Context) ([]streakRow, error)
	tryRecordReminder(ctx context.Context, userID string, weekStart time.Time) (bool, error)
	listReminderCandidates(ctx context.Context, weekStart, weekEnd time.Time) ([]ReminderCandidate, error)
	listParticipantUserIDs(ctx context.Context) ([]string, error)
	countedMatches(ctx context.Context, userID string) ([]CountedMatch, error)
	maxRating(ctx context.Context, userID string) (int, error)
	championCount(ctx context.Context, userID string) (int, error)
}

type streakRow struct {
	UserID string
	Streak Streak
}

type repo struct {
	pool *pgxpool.Pool
}

func (r *repo) insertBadge(ctx context.Context, userID, code string, meta json.RawMessage) (UserBadge, bool, error) {
	if meta == nil {
		meta = []byte("null")
	}
	const q = `
		INSERT INTO user_badges (user_id, badge_code, meta)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, badge_code) DO NOTHING
		RETURNING badge_code, unlocked_at, meta`
	var b UserBadge
	err := r.pool.QueryRow(ctx, q, userID, code, meta).Scan(&b.BadgeCode, &b.UnlockedAt, &b.Meta)
	if err != nil {
		if err == pgx.ErrNoRows {
			return UserBadge{}, false, nil
		}
		return UserBadge{}, false, fmt.Errorf("insert badge: %w", err)
	}
	return b, true, nil
}

func (r *repo) listBadges(ctx context.Context, userID string, limit, offset int) ([]UserBadge, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_badges WHERE user_id=$1`, userID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count badges: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT badge_code, unlocked_at, meta
		FROM user_badges
		WHERE user_id=$1
		ORDER BY unlocked_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list badges: %w", err)
	}
	defer rows.Close()
	out := []UserBadge{}
	for rows.Next() {
		var b UserBadge
		if err := rows.Scan(&b.BadgeCode, &b.UnlockedAt, &b.Meta); err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

func (r *repo) getGoal(ctx context.Context, userID string) (GoalConfig, error) {
	var g GoalConfig
	err := r.pool.QueryRow(ctx, `
		SELECT weekly_target, reminder_opt_in FROM user_goals WHERE user_id=$1`, userID,
	).Scan(&g.WeeklyTarget, &g.ReminderOptIn)
	if err != nil {
		if err == pgx.ErrNoRows {
			return GoalConfig{WeeklyTarget: 0, ReminderOptIn: true}, nil
		}
		return GoalConfig{}, fmt.Errorf("get goal: %w", err)
	}
	return g, nil
}

func (r *repo) upsertGoal(ctx context.Context, userID string, weeklyTarget int, reminderOptIn bool) (GoalConfig, error) {
	const q = `
		INSERT INTO user_goals (user_id, weekly_target, reminder_opt_in, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id) DO UPDATE
		  SET weekly_target = EXCLUDED.weekly_target,
		      reminder_opt_in = EXCLUDED.reminder_opt_in,
		      updated_at = NOW()
		RETURNING weekly_target, reminder_opt_in`
	var g GoalConfig
	err := r.pool.QueryRow(ctx, q, userID, weeklyTarget, reminderOptIn).Scan(&g.WeeklyTarget, &g.ReminderOptIn)
	if err != nil {
		return GoalConfig{}, fmt.Errorf("upsert goal: %w", err)
	}
	return g, nil
}

func (r *repo) getStreak(ctx context.Context, userID string) (Streak, error) {
	var s Streak
	err := r.pool.QueryRow(ctx, `
		SELECT current_weeks, longest_weeks, last_active_week
		FROM user_streaks WHERE user_id=$1`, userID,
	).Scan(&s.CurrentWeeks, &s.LongestWeeks, &s.LastActiveWeek)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Streak{}, nil
		}
		return Streak{}, fmt.Errorf("get streak: %w", err)
	}
	return s, nil
}

func (r *repo) upsertStreak(ctx context.Context, userID string, s Streak) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_streaks (user_id, current_weeks, longest_weeks, last_active_week, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id) DO UPDATE
		  SET current_weeks = EXCLUDED.current_weeks,
		      longest_weeks = EXCLUDED.longest_weeks,
		      last_active_week = EXCLUDED.last_active_week,
		      updated_at = NOW()`,
		userID, s.CurrentWeeks, s.LongestWeeks, s.LastActiveWeek)
	if err != nil {
		return fmt.Errorf("upsert streak: %w", err)
	}
	return nil
}

func (r *repo) listStreaks(ctx context.Context) ([]streakRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, current_weeks, longest_weeks, last_active_week FROM user_streaks`)
	if err != nil {
		return nil, fmt.Errorf("list streaks: %w", err)
	}
	defer rows.Close()
	var out []streakRow
	for rows.Next() {
		var row streakRow
		if err := rows.Scan(&row.UserID, &row.Streak.CurrentWeeks, &row.Streak.LongestWeeks, &row.Streak.LastActiveWeek); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *repo) tryRecordReminder(ctx context.Context, userID string, weekStart time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO goal_reminders_sent (user_id, week_start)
		VALUES ($1, $2)
		ON CONFLICT (user_id, week_start) DO NOTHING`, userID, weekStart)
	if err != nil {
		return false, fmt.Errorf("record reminder: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *repo) listReminderCandidates(ctx context.Context, weekStart, weekEnd time.Time) ([]ReminderCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.user_id, g.weekly_target, g.reminder_opt_in,
		       COALESCE((
		         SELECT COUNT(*) FROM (
		           SELECT id FROM matches WHERE player_a_user_id = g.user_id AND played_at >= $1 AND played_at < $2
		           UNION
		           SELECT id FROM matches WHERE player_b_user_id = g.user_id AND played_at >= $1 AND played_at < $2
		           UNION
		           SELECT id FROM matches WHERE player_a2_user_id = g.user_id AND played_at >= $1 AND played_at < $2
		           UNION
		           SELECT id FROM matches WHERE player_b2_user_id = g.user_id AND played_at >= $1 AND played_at < $2
		         ) m
		       ), 0)
		FROM user_goals g
		WHERE g.weekly_target > 0 AND g.reminder_opt_in = TRUE`, weekStart, weekEnd)
	if err != nil {
		return nil, fmt.Errorf("list reminder candidates: %w", err)
	}
	defer rows.Close()
	var out []ReminderCandidate
	for rows.Next() {
		var c ReminderCandidate
		if err := rows.Scan(&c.UserID, &c.WeeklyTarget, &c.ReminderOptIn, &c.Current); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *repo) listParticipantUserIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT uid FROM (
		  SELECT player_a_user_id AS uid FROM matches WHERE player_a_user_id IS NOT NULL
		  UNION
		  SELECT player_b_user_id FROM matches WHERE player_b_user_id IS NOT NULL
		  UNION
		  SELECT player_a2_user_id FROM matches WHERE player_a2_user_id IS NOT NULL
		  UNION
		  SELECT player_b2_user_id FROM matches WHERE player_b2_user_id IS NOT NULL
		) s`)
	if err != nil {
		return nil, fmt.Errorf("list participant users: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *repo) countedMatches(ctx context.Context, userID string) ([]CountedMatch, error) {
	const q = `
		SELECT id, sport, played_at, winner_side,
		       player_a_user_id, player_b_user_id, player_a2_user_id, player_b2_user_id,
		       set_scores
		FROM (
		  SELECT id, sport, played_at, winner_side, player_a_user_id, player_b_user_id,
		         player_a2_user_id, player_b2_user_id, set_scores
		  FROM matches WHERE player_a_user_id = $1
		  UNION
		  SELECT id, sport, played_at, winner_side, player_a_user_id, player_b_user_id,
		         player_a2_user_id, player_b2_user_id, set_scores
		  FROM matches WHERE player_b_user_id = $1
		  UNION
		  SELECT id, sport, played_at, winner_side, player_a_user_id, player_b_user_id,
		         player_a2_user_id, player_b2_user_id, set_scores
		  FROM matches WHERE player_a2_user_id = $1
		  UNION
		  SELECT id, sport, played_at, winner_side, player_a_user_id, player_b_user_id,
		         player_a2_user_id, player_b2_user_id, set_scores
		  FROM matches WHERE player_b2_user_id = $1
		) m
		ORDER BY played_at ASC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("counted matches: %w", err)
	}
	defer rows.Close()
	var out []CountedMatch
	for rows.Next() {
		var (
			id, sport, winner string
			playedAt          time.Time
			a, b, a2, b2      *string
			raw               []byte
		)
		if err := rows.Scan(&id, &sport, &playedAt, &winner, &a, &b, &a2, &b2, &raw); err != nil {
			return nil, err
		}
		side := sideOf(userID, a, a2, b, b2)
		var lines []struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &lines)
		}
		sets := make([]SetLine, 0, len(lines))
		for _, ln := range lines {
			if side == "B" {
				sets = append(sets, SetLine{For: ln.B, Against: ln.A})
			} else {
				sets = append(sets, SetLine{For: ln.A, Against: ln.B})
			}
		}
		out = append(out, CountedMatch{
			ID: id, Sport: sport, PlayedAt: playedAt,
			Won: side != "" && side == winner, SetScores: sets,
		})
	}
	return out, rows.Err()
}

func (r *repo) maxRating(ctx context.Context, userID string) (int, error) {
	var rating *int
	err := r.pool.QueryRow(ctx, `
		-- Highest of singles or doubles. A padel doubles rating can earn Elo 1600.
		SELECT MAX(rating) FROM user_ratings WHERE user_id=$1`, userID).Scan(&rating)
	if err != nil {
		return 0, fmt.Errorf("max rating: %w", err)
	}
	if rating == nil {
		return 0, nil
	}
	return *rating, nil
}

func (r *repo) championCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tournaments t
		JOIN tournament_participants p ON p.id = t.champion_id
		WHERE p.user_id = $1 AND t.status = 'COMPLETED'`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("champion count: %w", err)
	}
	return n, nil
}

func sideOf(userID string, a, a2, b, b2 *string) string {
	if idEq(a, userID) || idEq(a2, userID) {
		return "A"
	}
	if idEq(b, userID) || idEq(b2, userID) {
		return "B"
	}
	return ""
}

func idEq(p *string, id string) bool {
	return p != nil && *p == id
}
