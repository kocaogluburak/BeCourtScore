package score

import (
	"context"
	"fmt"
	"log/slog"
	"time"

)

// ratingHistoryRow is one append-only Elo snapshot produced by a replay.
type ratingHistoryRow struct {
	UserID   string
	Sport    string
	Rating   int
	Counted  int
	MatchID  string
	PlayedAt time.Time
}

// replaySinglesHistory walks finished rated singles in played_at order and
// reconstructs the Elo curve from 1500. It does not touch user_ratings.
func replaySinglesHistory(matches []Match) []ratingHistoryRow {
	type key struct{ user, sport string }
	type state struct {
		rating  int
		counted int
	}
	cur := map[key]state{}
	out := make([]ratingHistoryRow, 0, len(matches)*2)
	for _, m := range matches {
		winner, loser, ok := singlesEloPair(m)
		if !ok {
			continue
		}
		playedAt := m.PlayedAt
		if playedAt.IsZero() {
			continue
		}
		wk := key{winner, m.Sport}
		lk := key{loser, m.Sport}
		w := cur[wk]
		l := cur[lk]
		if w.rating == 0 && w.counted == 0 {
			w.rating = initialRating
		}
		if l.rating == 0 && l.counted == 0 {
			l.rating = initialRating
		}
		nextW, nextL := nextRatings(w.rating, l.rating, w.counted, l.counted, true)
		w.rating, w.counted = nextW, w.counted+1
		l.rating, l.counted = nextL, l.counted+1
		cur[wk] = w
		cur[lk] = l
		out = append(out,
			ratingHistoryRow{UserID: winner, Sport: m.Sport, Rating: w.rating, Counted: w.counted, MatchID: m.ID, PlayedAt: playedAt},
			ratingHistoryRow{UserID: loser, Sport: m.Sport, Rating: l.rating, Counted: l.counted, MatchID: m.ID, PlayedAt: playedAt},
		)
	}
	return out
}

// BackfillRatingHistory inserts missing user_rating_history rows for finished
// rated singles so Wrapped elo_peak is meaningful for years before 027.
// Idempotent: unique (user_id, match_id) + ON CONFLICT DO NOTHING.
// Does not rewrite user_ratings.
func (s *Service) BackfillRatingHistory(ctx context.Context) (int, error) {
	r, ok := s.repo.(*repo)
	if !ok {
		return 0, fmt.Errorf("elo backfill requires postgres repo")
	}
	return r.backfillRatingHistory(ctx)
}

func (r *repo) listRatedSinglesChrono(ctx context.Context) ([]Match, error) {
	q := fmt.Sprintf(`
		SELECT %s FROM matches
		WHERE player_a_user_id IS NOT NULL AND player_b_user_id IS NOT NULL
		  AND winner_side IN ('A', 'B')
		  AND COALESCE(player_a2_user_id::text, '') = ''
		  AND COALESCE(player_b2_user_id::text, '') = ''
		  AND COALESCE(player_a2_name, '') = ''
		  AND COALESCE(player_b2_name, '') = ''
		ORDER BY played_at ASC`, matchCols)
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list rated singles: %w", err)
	}
	defer rows.Close()
	return scanMatchRows(rows)
}

func (r *repo) backfillRatingHistory(ctx context.Context) (int, error) {
	matches, err := r.listRatedSinglesChrono(ctx)
	if err != nil {
		return 0, err
	}
	rows := replaySinglesHistory(matches)
	if len(rows) == 0 {
		return 0, nil
	}
	inserted := 0
	for _, row := range rows {
		ok, err := r.insertRatingHistoryIfMissing(ctx, row)
		if err != nil {
			return inserted, err
		}
		if ok {
			inserted++
		}
	}
	slog.Info("elo history backfill", "candidates", len(rows), "inserted", inserted)
	return inserted, nil
}

func (r *repo) insertRatingHistoryIfMissing(ctx context.Context, row ratingHistoryRow) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO user_rating_history (user_id, sport, rating, matches_counted, match_id, played_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, match_id) WHERE match_id IS NOT NULL DO NOTHING`,
		row.UserID, row.Sport, row.Rating, row.Counted, row.MatchID, row.PlayedAt)
	if err != nil {
		return false, fmt.Errorf("backfill rating history: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
