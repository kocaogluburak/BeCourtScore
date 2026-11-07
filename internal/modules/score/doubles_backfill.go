package score

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// doublesEvent is one rated 2v2, either a history match or a social round.
type doublesEvent struct {
	Sport             string
	PlayedAt          time.Time
	MatchID           string
	TournamentMatchID string
	SideA             [2]string
	SideB             [2]string
	AWon              bool
	Seq               int
}

type doublesFinal struct {
	UserID  string
	Sport   string
	Rating  int
	Counted int
}

// replayDoublesHistory walks 2v2 results in time order from 1500.
func replayDoublesHistory(events []doublesEvent) ([]ratingHistoryRow, []doublesFinal) {
	type key struct{ user, sport string }
	type state struct {
		rating  int
		counted int
	}
	cur := map[key]state{}
	history := make([]ratingHistoryRow, 0, len(events)*4)
	for _, ev := range events {
		if ev.PlayedAt.IsZero() || !distinctIDs(ev.SideA[0], ev.SideA[1], ev.SideB[0], ev.SideB[1]) {
			continue
		}
		load := func(id string) lockedRating {
			st := cur[key{id, ev.Sport}]
			if st.rating == 0 && st.counted == 0 {
				st.rating = initialRating
			}
			return lockedRating{rating: st.rating, counted: st.counted}
		}
		var aRows, bRows [2]lockedRating
		for i := 0; i < 2; i++ {
			aRows[i] = load(ev.SideA[i])
			bRows[i] = load(ev.SideB[i])
		}
		nextA, nextB := nextDoublesRatings(aRows, bRows, ev.AWon)
		record := func(id string, next, counted int) {
			cur[key{id, ev.Sport}] = state{rating: next, counted: counted}
			history = append(history, ratingHistoryRow{
				UserID: id, Sport: ev.Sport, Discipline: DisciplineDoubles,
				Rating: next, Counted: counted,
				MatchID: ev.MatchID, TournamentMatchID: ev.TournamentMatchID,
				PlayedAt: ev.PlayedAt,
			})
		}
		for i := 0; i < 2; i++ {
			record(ev.SideA[i], nextA[i], aRows[i].counted+1)
			record(ev.SideB[i], nextB[i], bRows[i].counted+1)
		}
	}
	finals := make([]doublesFinal, 0, len(cur))
	for k, st := range cur {
		finals = append(finals, doublesFinal{UserID: k.user, Sport: k.sport, Rating: st.rating, Counted: st.counted})
	}
	return history, finals
}

// BackfillDoublesRatings replays registered doubles matches and finished
// social rounds from 1500 into DOUBLES ratings and history.
// Idempotent: history conflicts are ignored and the final curve is upserted.
// SINGLES rows are left untouched. Not run on API startup.
func (s *Service) BackfillDoublesRatings(ctx context.Context) (int, error) {
	r, ok := s.repo.(*repo)
	if !ok {
		return 0, fmt.Errorf("doubles backfill requires postgres repo")
	}
	return r.backfillDoublesRatings(ctx)
}

func (r *repo) backfillDoublesRatings(ctx context.Context) (int, error) {
	events, err := r.listDoublesEvents(ctx)
	if err != nil {
		return 0, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].PlayedAt.Equal(events[j].PlayedAt) {
			return events[i].PlayedAt.Before(events[j].PlayedAt)
		}
		if events[i].Seq != events[j].Seq {
			return events[i].Seq < events[j].Seq
		}
		if events[i].MatchID != events[j].MatchID {
			return events[i].MatchID < events[j].MatchID
		}
		return events[i].TournamentMatchID < events[j].TournamentMatchID
	})
	history, finals := replayDoublesHistory(events)
	inserted := 0
	for _, row := range history {
		ok, err := r.insertDoublesHistoryIfMissing(ctx, row)
		if err != nil {
			return inserted, err
		}
		if ok {
			inserted++
		}
	}
	for _, fin := range finals {
		if err := r.upsertDoublesRating(ctx, fin); err != nil {
			return inserted, err
		}
	}
	slog.Info("doubles elo backfill", "events", len(events), "history_inserted", inserted, "players", len(finals))
	return inserted, nil
}

func (r *repo) listDoublesEvents(ctx context.Context) ([]doublesEvent, error) {
	matches, err := r.listDoublesMatchesChrono(ctx)
	if err != nil {
		return nil, err
	}
	social, err := r.listSocialDoublesChrono(ctx)
	if err != nil {
		return nil, err
	}
	return append(matches, social...), nil
}

func (r *repo) listDoublesMatchesChrono(ctx context.Context) ([]doublesEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, sport, played_at,
		       player_a_user_id::text, player_a2_user_id::text,
		       player_b_user_id::text, player_b2_user_id::text,
		       winner_side::text
		FROM matches
		WHERE player_a_user_id IS NOT NULL AND player_a2_user_id IS NOT NULL
		  AND player_b_user_id IS NOT NULL AND player_b2_user_id IS NOT NULL
		  AND winner_side IN ('A', 'B')
		ORDER BY played_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list doubles matches: %w", err)
	}
	defer rows.Close()
	var out []doublesEvent
	for rows.Next() {
		var ev doublesEvent
		var winner string
		if err := rows.Scan(&ev.MatchID, &ev.Sport, &ev.PlayedAt, &ev.SideA[0], &ev.SideA[1], &ev.SideB[0], &ev.SideB[1], &winner); err != nil {
			return nil, fmt.Errorf("scan doubles match: %w", err)
		}
		ev.AWon = winner == "A"
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *repo) listSocialDoublesChrono(ctx context.Context) ([]doublesEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tm.id::text, t.sport, t.updated_at,
		       pa.user_id::text, pa2.user_id::text, pb.user_id::text, pb2.user_id::text,
		       COALESCE(tm.score_summary, ''), tm.round, tm.position_in_round
		FROM tournament_matches tm
		JOIN tournaments t ON t.id = tm.tournament_id
		JOIN tournament_participants pa ON pa.id = tm.participant_a_id
		JOIN tournament_participants pa2 ON pa2.id = tm.participant_a2_id
		JOIN tournament_participants pb ON pb.id = tm.participant_b_id
		JOIN tournament_participants pb2 ON pb2.id = tm.participant_b2_id
		WHERE t.status = 'COMPLETED'
		  AND t.format IN ('AMERICANO','MEXICANO','KING_OF_THE_COURT')
		  AND tm.status = 'COMPLETED'
		ORDER BY t.updated_at ASC, tm.round ASC, tm.position_in_round ASC`)
	if err != nil {
		return nil, fmt.Errorf("list social doubles: %w", err)
	}
	defer rows.Close()
	var out []doublesEvent
	for rows.Next() {
		var ev doublesEvent
		var score string
		var round, pos int
		if err := rows.Scan(&ev.TournamentMatchID, &ev.Sport, &ev.PlayedAt, &ev.SideA[0], &ev.SideA[1], &ev.SideB[0], &ev.SideB[1], &score, &round, &pos); err != nil {
			return nil, fmt.Errorf("scan social doubles: %w", err)
		}
		aPts, bPts, ok := parseTeamPointScore(score)
		if !ok || aPts == bPts {
			continue
		}
		ev.AWon = aPts > bPts
		ev.Seq = round*1000 + pos
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *repo) insertDoublesHistoryIfMissing(ctx context.Context, row ratingHistoryRow) (bool, error) {
	var tag interface{ RowsAffected() int64 }
	var err error
	if row.TournamentMatchID != "" {
		tag, err = r.pool.Exec(ctx, `
			INSERT INTO user_rating_history
				(user_id, sport, discipline, rating, matches_counted, tournament_match_id, played_at)
			VALUES ($1, $2, 'DOUBLES', $3, $4, $5, $6)
			ON CONFLICT (user_id, tournament_match_id) WHERE tournament_match_id IS NOT NULL DO NOTHING`,
			row.UserID, row.Sport, row.Rating, row.Counted, row.TournamentMatchID, row.PlayedAt)
	} else {
		tag, err = r.pool.Exec(ctx, `
			INSERT INTO user_rating_history
				(user_id, sport, discipline, rating, matches_counted, match_id, played_at)
			VALUES ($1, $2, 'DOUBLES', $3, $4, $5, $6)
			ON CONFLICT (user_id, match_id) WHERE match_id IS NOT NULL DO NOTHING`,
			row.UserID, row.Sport, row.Rating, row.Counted, row.MatchID, row.PlayedAt)
	}
	if err != nil {
		return false, fmt.Errorf("doubles history backfill: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *repo) upsertDoublesRating(ctx context.Context, fin doublesFinal) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_ratings (user_id, sport, discipline, rating, matches_counted)
		VALUES ($1, $2, 'DOUBLES', $3, $4)
		ON CONFLICT (user_id, sport, discipline)
		DO UPDATE SET rating = EXCLUDED.rating, matches_counted = EXCLUDED.matches_counted`,
		fin.UserID, fin.Sport, fin.Rating, fin.Counted)
	if err != nil {
		return fmt.Errorf("upsert doubles rating: %w", err)
	}
	return nil
}
