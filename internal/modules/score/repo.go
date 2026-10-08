package score

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	ID              string       `json:"id"`
	Sport           string       `json:"sport"`
	PlayerAName     string       `json:"player_a_name"`
	PlayerBName     string       `json:"player_b_name"`
	PlayerAUserID   *string      `json:"player_a_user_id"`
	PlayerBUserID   *string      `json:"player_b_user_id"`
	PlayerA2Name    *string      `json:"player_a2_name,omitempty"`
	PlayerB2Name    *string      `json:"player_b2_name,omitempty"`
	PlayerA2UserID  *string      `json:"player_a2_user_id,omitempty"`
	PlayerB2UserID  *string      `json:"player_b2_user_id,omitempty"`
	SetsA           int          `json:"sets_a"`
	SetsB           int          `json:"sets_b"`
	SetScores       []SetScore   `json:"set_scores,omitempty"`
	WinnerSide      string       `json:"winner_side"`
	Winner          string       `json:"winner"` // display name derived from winner_side
	SetRule         string       `json:"set_rule"`
	FinalSet        string       `json:"final_set"`
	PointLog        []PointEvent `json:"point_log,omitempty"`
	DurationSeconds *int         `json:"duration_seconds,omitempty"`
	CreatedBy       string       `json:"created_by"`
	PlayedAt        time.Time    `json:"played_at"`
	CreatedAt       time.Time    `json:"created_at"`
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
       set_rule, final_set, created_at, point_log, duration_seconds`

func scanMatch(row pgx.Row) (Match, error) {
	var m Match
	var setScoresRaw []byte
	var pointLogRaw []byte
	err := row.Scan(&m.ID, &m.Sport, &m.PlayerAName, &m.PlayerBName, &m.PlayerAUserID, &m.PlayerBUserID,
		&m.PlayerA2Name, &m.PlayerB2Name, &m.PlayerA2UserID, &m.PlayerB2UserID,
		&m.SetsA, &m.SetsB, &setScoresRaw, &m.WinnerSide, &m.CreatedBy, &m.PlayedAt,
		&m.SetRule, &m.FinalSet, &m.CreatedAt, &pointLogRaw, &m.DurationSeconds)
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

	q := fmt.Sprintf(`
		INSERT INTO matches (sport, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
		                     player_a2_name, player_b2_name, player_a2_user_id, player_b2_user_id,
		                     sets_a, sets_b, set_scores, winner_side, created_by, played_at,
		                     set_rule, final_set, point_log, duration_seconds)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING %s`, matchCols)

	m, err := scanMatch(r.pool.QueryRow(ctx, q,
		in.Sport, in.PlayerAName, in.PlayerBName, in.PlayerAUserID, in.PlayerBUserID,
		in.PlayerA2Name, in.PlayerB2Name, in.PlayerA2UserID, in.PlayerB2UserID,
		in.SetsA, in.SetsB, raw, in.WinnerSide, createdBy, playedAt, in.SetRule, in.FinalSet,
		pointRaw, in.DurationSeconds))
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

func (r *repo) applySinglesElo(ctx context.Context, sport, winnerID, loserID string) error {
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
	first, err := lockRating(ctx, tx, firstID, sport)
	if err != nil {
		return err
	}
	second, err := lockRating(ctx, tx, secondID, sport)
	if err != nil {
		return err
	}
	nextFirst, nextSecond := nextRatings(first.rating, second.rating, first.counted, second.counted, firstWon)
	if err := saveRating(ctx, tx, firstID, sport, nextFirst, first.counted+1); err != nil {
		return err
	}
	if err := saveRating(ctx, tx, secondID, sport, nextSecond, second.counted+1); err != nil {
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

func lockRating(ctx context.Context, tx pgx.Tx, userID, sport string) (lockedRating, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_ratings (user_id, sport, rating, matches_counted)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (user_id, sport) DO NOTHING`, userID, sport, initialRating); err != nil {
		return lockedRating{}, fmt.Errorf("seed rating: %w", err)
	}
	var row lockedRating
	err := tx.QueryRow(ctx, `
		SELECT rating, matches_counted FROM user_ratings
		WHERE user_id=$1 AND sport=$2 FOR UPDATE`, userID, sport).Scan(&row.rating, &row.counted)
	if err != nil {
		return lockedRating{}, fmt.Errorf("lock rating: %w", err)
	}
	return row, nil
}

func saveRating(ctx context.Context, tx pgx.Tx, userID, sport string, rating, counted int) error {
	_, err := tx.Exec(ctx, `
		UPDATE user_ratings SET rating=$3, matches_counted=$4
		WHERE user_id=$1 AND sport=$2`, userID, sport, rating, counted)
	if err != nil {
		return fmt.Errorf("save rating: %w", err)
	}
	return nil
}

func (r *repo) listRatings(ctx context.Context, userID string) ([]UserRating, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sport, rating, matches_counted FROM user_ratings WHERE user_id=$1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list ratings: %w", err)
	}
	defer rows.Close()
	out := []UserRating{}
	for rows.Next() {
		var row UserRating
		if err := rows.Scan(&row.Sport, &row.Rating, &row.MatchesCounted); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Sentinel errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid input")
	ErrConflict  = errors.New("conflict")
)
