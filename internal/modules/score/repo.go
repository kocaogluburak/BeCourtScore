package score

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetScore is one completed set (tennis/padel games) or game (squash/PP points).
type SetScore struct {
	A int `json:"a"`
	B int `json:"b"`
}

// Match is a finished match stored in history.
type Match struct {
	ID              string          `json:"id"`
	Sport           string          `json:"sport"`
	PlayerAName     string          `json:"player_a_name"`
	PlayerBName     string          `json:"player_b_name"`
	PlayerAUserID   *string         `json:"player_a_user_id"`
	PlayerBUserID   *string         `json:"player_b_user_id"`
	PlayerA2Name    *string         `json:"player_a2_name,omitempty"`
	PlayerB2Name    *string         `json:"player_b2_name,omitempty"`
	PlayerA2UserID  *string         `json:"player_a2_user_id,omitempty"`
	PlayerB2UserID  *string         `json:"player_b2_user_id,omitempty"`
	SetsA           int             `json:"sets_a"`
	SetsB           int             `json:"sets_b"`
	SetScores       []SetScore      `json:"set_scores,omitempty"`
	WinnerSide      string          `json:"winner_side"`
	Winner          string          `json:"winner"` // display name derived from winner_side
	SetRule         string          `json:"set_rule"`
	FinalSet        string          `json:"final_set"`
	PointLog        []PointEvent    `json:"point_log,omitempty"`
	DurationSeconds *int            `json:"duration_seconds,omitempty"`
	AvgHeartRate    *int            `json:"avg_heart_rate,omitempty"`
	MaxHeartRate    *int            `json:"max_heart_rate,omitempty"`
	ActiveCalories  *int            `json:"active_calories,omitempty"`
	FitnessSource   *string         `json:"fitness_source,omitempty"`
	FitnessSamples  []FitnessSample `json:"fitness_samples,omitempty"`
	CreatedBy       string          `json:"created_by"`
	PlayedAt        time.Time       `json:"played_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

// CreateInput holds the fields required to record a match.
type CreateInput struct {
	Sport           string
	PlayerAName     string
	PlayerBName     string
	PlayerAUserID   *string
	PlayerBUserID   *string
	PlayerA2Name    *string
	PlayerB2Name    *string
	PlayerA2UserID  *string
	PlayerB2UserID  *string
	SetRule         string
	FinalSet        string
	SetsA           int
	SetsB           int
	SetScores       []SetScore
	WinnerSide      string
	PlayedAt        *time.Time
	PointLog        []PointEvent
	DurationSeconds *int
	AvgHeartRate    *int
	MaxHeartRate    *int
	ActiveCalories  *int
	FitnessSource   *string
	FitnessSamples  []FitnessSample
}

// ListFilter narrows a match history listing.
type ListFilter struct {
	Query string // substring match on player names
	Sport string
}

type repo struct{ pool *pgxpool.Pool }

const matchCols = `id, sport, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
       player_a2_name, player_b2_name, player_a2_user_id, player_b2_user_id,
       sets_a, sets_b, set_scores, winner_side, created_by, played_at,
       set_rule, final_set, created_at, point_log, duration_seconds,
       avg_heart_rate, max_heart_rate, active_calories, fitness_source, fitness_samples`

func scanMatch(row pgx.Row) (Match, error) {
	var m Match
	var setScoresRaw []byte
	var pointLogRaw []byte
	var fitnessRaw []byte
	err := row.Scan(&m.ID, &m.Sport, &m.PlayerAName, &m.PlayerBName, &m.PlayerAUserID, &m.PlayerBUserID,
		&m.PlayerA2Name, &m.PlayerB2Name, &m.PlayerA2UserID, &m.PlayerB2UserID,
		&m.SetsA, &m.SetsB, &setScoresRaw, &m.WinnerSide, &m.CreatedBy, &m.PlayedAt,
		&m.SetRule, &m.FinalSet, &m.CreatedAt, &pointLogRaw, &m.DurationSeconds,
		&m.AvgHeartRate, &m.MaxHeartRate, &m.ActiveCalories, &m.FitnessSource, &fitnessRaw)
	if err != nil {
		return Match{}, err
	}
	if len(setScoresRaw) > 0 {
		_ = json.Unmarshal(setScoresRaw, &m.SetScores)
	}
	if m.SetScores == nil {
		m.SetScores = []SetScore{}
	}
	if len(pointLogRaw) > 0 {
		_ = json.Unmarshal(pointLogRaw, &m.PointLog)
	}
	if len(fitnessRaw) > 0 {
		_ = json.Unmarshal(fitnessRaw, &m.FitnessSamples)
	}
	if m.WinnerSide == "A" {
		m.Winner = sideLabel(m.PlayerAName, m.PlayerA2Name)
	} else {
		m.Winner = sideLabel(m.PlayerBName, m.PlayerB2Name)
	}
	return m, nil
}

func marshalPointLog(points []PointEvent) ([]byte, error) {
	points = capPointLog(points)
	if points == nil {
		return nil, nil
	}
	return json.Marshal(points)
}

func marshalFitnessSamples(samples []FitnessSample) ([]byte, error) {
	samples = capFitnessSamples(samples)
	if samples == nil {
		return nil, nil
	}
	return json.Marshal(samples)
}

func marshalSetScores(scores []SetScore) ([]byte, error) {
	if scores == nil {
		scores = []SetScore{}
	}
	return json.Marshal(scores)
}

func (r *repo) insert(ctx context.Context, createdBy string, in CreateInput) (Match, error) {
	playedAt := time.Now()
	if in.PlayedAt != nil {
		playedAt = *in.PlayedAt
	}
	raw, err := marshalSetScores(in.SetScores)
	if err != nil {
		return Match{}, fmt.Errorf("marshal set_scores: %w", err)
	}
	pointRaw, err := marshalPointLog(in.PointLog)
	if err != nil {
		return Match{}, fmt.Errorf("marshal point_log: %w", err)
	}
	fitnessRaw, err := marshalFitnessSamples(in.FitnessSamples)
	if err != nil {
		return Match{}, fmt.Errorf("marshal fitness_samples: %w", err)
	}

	q := fmt.Sprintf(`
		INSERT INTO matches (sport, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
		                     player_a2_name, player_b2_name, player_a2_user_id, player_b2_user_id,
		                     sets_a, sets_b, set_scores, winner_side, created_by, played_at,
		                     set_rule, final_set, point_log, duration_seconds,
		                     avg_heart_rate, max_heart_rate, active_calories, fitness_source, fitness_samples)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		RETURNING %s`, matchCols)

	m, err := scanMatch(r.pool.QueryRow(ctx, q,
		in.Sport, in.PlayerAName, in.PlayerBName, in.PlayerAUserID, in.PlayerBUserID,
		in.PlayerA2Name, in.PlayerB2Name, in.PlayerA2UserID, in.PlayerB2UserID,
		in.SetsA, in.SetsB, raw, in.WinnerSide, createdBy, playedAt, in.SetRule, in.FinalSet,
		pointRaw, in.DurationSeconds,
		in.AvgHeartRate, in.MaxHeartRate, in.ActiveCalories, in.FitnessSource, fitnessRaw))
	if err != nil {
		return Match{}, fmt.Errorf("insert match: %w", err)
	}
	return m, nil
}

// listForUser returns matches where the user is a participant or the recorder,
// newest first, with the total count for pagination.
// Uses UNION of three index-driven legs instead of OR across columns.
func (r *repo) listForUser(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Match, int64, error) {
	union := `
		SELECT ` + matchCols + ` FROM matches WHERE created_by = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_a_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_b_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_a2_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_b2_user_id = $1`
	args := []any{userID}
	filter := ""

	if f.Sport != "" {
		args = append(args, f.Sport)
		filter += fmt.Sprintf(" AND sport = $%d", len(args))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		filter += fmt.Sprintf(" AND (player_a_name ILIKE $%d OR player_b_name ILIKE $%d OR player_a2_name ILIKE $%d OR player_b2_name ILIKE $%d)", len(args), len(args), len(args), len(args))
	}

	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM (`+union+`) m WHERE TRUE`+filter, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count matches: %w", err)
	}

	args = append(args, limit, offset)
	q := fmt.Sprintf(
		`SELECT %s FROM (%s) m WHERE TRUE%s ORDER BY played_at DESC LIMIT $%d OFFSET $%d`,
		matchCols, union, filter, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list matches: %w", err)
	}
	defer rows.Close()

	matches := []Match{}
	for rows.Next() {
		m, err := scanMatch(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, total, rows.Err()
}

func (r *repo) getByID(ctx context.Context, id string) (Match, error) {
	q := fmt.Sprintf(`SELECT %s FROM matches WHERE id = $1`, matchCols)
	m, err := scanMatch(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Match{}, ErrNotFound
	}
	if err != nil {
		return Match{}, fmt.Errorf("get match: %w", err)
	}
	return m, nil
}

// deleteByID removes a match; only the creator may delete.
func (r *repo) deleteByID(ctx context.Context, id, createdBy string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM matches WHERE id = $1 AND created_by = $2`, id, createdBy)
	if err != nil {
		return fmt.Errorf("delete match: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// listParticipantMatches returns finished matches where userID is a registered
// player (not recorder-only), newest first. Empty sport = all sports.
func (r *repo) listParticipantMatches(ctx context.Context, userID, sport string) ([]Match, error) {
	union := `
		SELECT ` + matchCols + ` FROM matches WHERE player_a_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_b_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_a2_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_b2_user_id = $1`
	args := []any{userID}
	filter := ""
	if sport != "" {
		args = append(args, sport)
		filter = fmt.Sprintf(" AND sport = $%d", len(args))
	}
	q := fmt.Sprintf(
		`SELECT %s FROM (%s) m WHERE TRUE%s ORDER BY played_at DESC`,
		matchCols, union, filter)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list participant matches: %w", err)
	}
	defer rows.Close()
	return scanMatchRows(rows)
}

// listParticipantMatchesInRange is the year-window variant of listParticipantMatches.
// The half-open range [from, to) is UTC and prunes monthly partitions on played_at.
func (r *repo) listParticipantMatchesInRange(ctx context.Context, userID string, from, to time.Time) ([]Match, error) {
	union := `
		SELECT ` + matchCols + ` FROM matches WHERE player_a_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_b_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_a2_user_id = $1
		UNION
		SELECT ` + matchCols + ` FROM matches WHERE player_b2_user_id = $1`
	q := fmt.Sprintf(
		`SELECT %s FROM (%s) m WHERE played_at >= $2 AND played_at < $3 ORDER BY played_at DESC`,
		matchCols, union)
	rows, err := r.pool.Query(ctx, q, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list participant matches in range: %w", err)
	}
	defer rows.Close()
	return scanMatchRows(rows)
}

// firstMatchPerSport returns the earliest participant match per sport (any year).
func (r *repo) firstMatchPerSport(ctx context.Context, userID string) ([]SportFirst, error) {
	union := `
		SELECT sport, played_at FROM matches WHERE player_a_user_id = $1
		UNION ALL
		SELECT sport, played_at FROM matches WHERE player_b_user_id = $1
		UNION ALL
		SELECT sport, played_at FROM matches WHERE player_a2_user_id = $1
		UNION ALL
		SELECT sport, played_at FROM matches WHERE player_b2_user_id = $1`
	q := fmt.Sprintf(`SELECT sport, MIN(played_at) FROM (%s) x GROUP BY sport`, union)
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("first match per sport: %w", err)
	}
	defer rows.Close()
	out := []SportFirst{}
	for rows.Next() {
		var row SportFirst
		if err := rows.Scan(&row.Sport, &row.PlayedAt); err != nil {
			return nil, fmt.Errorf("scan first sport: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// listH2HMatches returns matches where both userA and userB are registered players,
// newest first. Empty sport = all sports.
func (r *repo) listH2HMatches(ctx context.Context, userA, userB, sport string) ([]Match, error) {
	args := []any{userA, userB}
	filter := `
		WHERE (
			((player_a_user_id = $1 OR player_a2_user_id = $1) AND (player_b_user_id = $2 OR player_b2_user_id = $2))
			OR
			((player_a_user_id = $2 OR player_a2_user_id = $2) AND (player_b_user_id = $1 OR player_b2_user_id = $1))
		)`
	if sport != "" {
		args = append(args, sport)
		filter += fmt.Sprintf(" AND sport = $%d", len(args))
	}
	q := fmt.Sprintf(`SELECT %s FROM matches %s ORDER BY played_at DESC`, matchCols, filter)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list h2h matches: %w", err)
	}
	defer rows.Close()
	return scanMatchRows(rows)
}

func scanMatchRows(rows pgx.Rows) ([]Match, error) {
	matches := []Match{}
	for rows.Next() {
		m, err := scanMatch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func (r *repo) applySinglesElo(ctx context.Context, sport, winnerID, loserID, matchID string, playedAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rating: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the lower id first so two concurrent matches cannot deadlock.
	firstID, secondID := winnerID, loserID
	firstWon := true
	if loserID < winnerID {
		firstID, secondID = loserID, winnerID
		firstWon = false
	}
	first, err := lockRating(ctx, tx, firstID, sport, DisciplineSingles)
	if err != nil {
		return err
	}
	second, err := lockRating(ctx, tx, secondID, sport, DisciplineSingles)
	if err != nil {
		return err
	}
	nextFirst, nextSecond := nextRatings(first.rating, second.rating, first.counted, second.counted, firstWon)
	if err := saveRating(ctx, tx, firstID, sport, DisciplineSingles, nextFirst, first.counted+1); err != nil {
		return err
	}
	if err := saveRating(ctx, tx, secondID, sport, DisciplineSingles, nextSecond, second.counted+1); err != nil {
		return err
	}
	if playedAt.IsZero() {
		playedAt = time.Now().UTC()
	}
	if err := insertRatingHistory(ctx, tx, firstID, sport, DisciplineSingles, nextFirst, first.counted+1, matchID, "", playedAt); err != nil {
		return err
	}
	if err := insertRatingHistory(ctx, tx, secondID, sport, DisciplineSingles, nextSecond, second.counted+1, matchID, "", playedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rating: %w", err)
	}
	return nil
}

type lockedRating struct {
	rating  int
	counted int
}

func (r *repo) applyDoublesElo(ctx context.Context, sport string, sideA, sideB [2]string, aWon bool, matchID, tournamentMatchID string, playedAt time.Time) error {
	ids := []string{sideA[0], sideA[1], sideB[0], sideB[1]}
	sort.Strings(ids)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin doubles rating: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked := make(map[string]lockedRating, len(ids))
	for _, id := range ids {
		row, err := lockRating(ctx, tx, id, sport, DisciplineDoubles)
		if err != nil {
			return err
		}
		locked[id] = row
	}
	already, err := doublesHistoryExists(ctx, tx, matchID, tournamentMatchID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}
	var aRows, bRows [2]lockedRating
	for i := 0; i < 2; i++ {
		aRows[i] = locked[sideA[i]]
		bRows[i] = locked[sideB[i]]
	}
	nextA, nextB := nextDoublesRatings(aRows, bRows, aWon)
	if playedAt.IsZero() {
		playedAt = time.Now().UTC()
	}
	updates := []struct {
		id   string
		next int
		prev lockedRating
	}{
		{sideA[0], nextA[0], aRows[0]},
		{sideA[1], nextA[1], aRows[1]},
		{sideB[0], nextB[0], bRows[0]},
		{sideB[1], nextB[1], bRows[1]},
	}
	for _, u := range updates {
		if err := saveRating(ctx, tx, u.id, sport, DisciplineDoubles, u.next, u.prev.counted+1); err != nil {
			return err
		}
		if err := insertRatingHistory(ctx, tx, u.id, sport, DisciplineDoubles, u.next, u.prev.counted+1, matchID, tournamentMatchID, playedAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit doubles rating: %w", err)
	}
	return nil
}

func doublesHistoryExists(ctx context.Context, tx pgx.Tx, matchID, tournamentMatchID string) (bool, error) {
	var n int
	var err error
	switch {
	case tournamentMatchID != "":
		err = tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM user_rating_history
			WHERE tournament_match_id = $1`, tournamentMatchID).Scan(&n)
	case matchID != "":
		err = tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM user_rating_history
			WHERE match_id = $1 AND discipline = $2`, matchID, DisciplineDoubles).Scan(&n)
	default:
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("doubles history: %w", err)
	}
	return n > 0, nil
}

func lockRating(ctx context.Context, tx pgx.Tx, userID, sport, discipline string) (lockedRating, error) {
	if discipline == "" {
		discipline = DisciplineSingles
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_ratings (user_id, sport, discipline, rating, matches_counted)
		VALUES ($1, $2, $3, $4, 0)
		ON CONFLICT (user_id, sport, discipline) DO NOTHING`, userID, sport, discipline, initialRating); err != nil {
		return lockedRating{}, fmt.Errorf("seed rating: %w", err)
	}
	var row lockedRating
	err := tx.QueryRow(ctx, `
		SELECT rating, matches_counted FROM user_ratings
		WHERE user_id=$1 AND sport=$2 AND discipline=$3 FOR UPDATE`, userID, sport, discipline).Scan(&row.rating, &row.counted)
	if err != nil {
		return lockedRating{}, fmt.Errorf("lock rating: %w", err)
	}
	return row, nil
}

func saveRating(ctx context.Context, tx pgx.Tx, userID, sport, discipline string, rating, counted int) error {
	if discipline == "" {
		discipline = DisciplineSingles
	}
	_, err := tx.Exec(ctx, `
		UPDATE user_ratings SET rating=$4, matches_counted=$5
		WHERE user_id=$1 AND sport=$2 AND discipline=$3`, userID, sport, discipline, rating, counted)
	if err != nil {
		return fmt.Errorf("save rating: %w", err)
	}
	return nil
}

func insertRatingHistory(ctx context.Context, tx pgx.Tx, userID, sport, discipline string, rating, counted int, matchID, tournamentMatchID string, playedAt time.Time) error {
	if discipline == "" {
		discipline = DisciplineSingles
	}
	var matchArg any
	if matchID != "" {
		matchArg = matchID
	}
	var tournamentArg any
	if tournamentMatchID != "" {
		tournamentArg = tournamentMatchID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO user_rating_history
			(user_id, sport, discipline, rating, matches_counted, match_id, tournament_match_id, played_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		userID, sport, discipline, rating, counted, matchArg, tournamentArg, playedAt)
	if err != nil {
		return fmt.Errorf("rating history: %w", err)
	}
	return nil
}

func (r *repo) listRatings(ctx context.Context, userID string) ([]UserRating, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sport, discipline, rating, matches_counted FROM user_ratings WHERE user_id=$1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list ratings: %w", err)
	}
	defer rows.Close()
	out := []UserRating{}
	for rows.Next() {
		var row UserRating
		if err := rows.Scan(&row.Sport, &row.Discipline, &row.Rating, &row.MatchesCounted); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *repo) ratingPeakInRange(ctx context.Context, userID string, from, to time.Time) (*WrappedElo, error) {
	var sport string
	var rating int
	var playedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT sport, rating, played_at
		FROM user_rating_history
		WHERE user_id=$1 AND played_at >= $2 AND played_at < $3
		  AND discipline = 'SINGLES'
		ORDER BY rating DESC, played_at ASC
		LIMIT 1`, userID, from, to).Scan(&sport, &rating, &playedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("rating peak: %w", err)
	}
	return &WrappedElo{Sport: sport, Rating: rating, PlayedAt: &playedAt}, nil
}

// Sentinel errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid input")
	ErrConflict  = errors.New("conflict")
)
