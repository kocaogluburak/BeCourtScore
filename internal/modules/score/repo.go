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
	ID            string     `json:"id"`
	Sport         string     `json:"sport"`
	PlayerAName   string     `json:"player_a_name"`
	PlayerBName   string     `json:"player_b_name"`
	PlayerAUserID *string    `json:"player_a_user_id"`
	PlayerBUserID *string    `json:"player_b_user_id"`
	SetsA         int        `json:"sets_a"`
	SetsB         int        `json:"sets_b"`
	SetScores     []SetScore `json:"set_scores,omitempty"`
	WinnerSide    string     `json:"winner_side"`
	Winner        string     `json:"winner"` // display name derived from winner_side
	CreatedBy     string     `json:"created_by"`
	PlayedAt      time.Time  `json:"played_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// CreateInput holds the fields required to record a match.
type CreateInput struct {
	Sport         string
	PlayerAName   string
	PlayerBName   string
	PlayerAUserID *string
	PlayerBUserID *string
	SetsA         int
	SetsB         int
	SetScores     []SetScore
	WinnerSide    string
	PlayedAt      *time.Time
}

// ListFilter narrows a match history listing.
type ListFilter struct {
	Query string // substring match on player names
	Sport string
}

type repo struct{ pool *pgxpool.Pool }

const matchCols = `id, sport, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
       sets_a, sets_b, set_scores, winner_side, created_by, played_at, created_at`

func scanMatch(row pgx.Row) (Match, error) {
	var m Match
	var setScoresRaw []byte
	err := row.Scan(&m.ID, &m.Sport, &m.PlayerAName, &m.PlayerBName, &m.PlayerAUserID, &m.PlayerBUserID,
		&m.SetsA, &m.SetsB, &setScoresRaw, &m.WinnerSide, &m.CreatedBy, &m.PlayedAt, &m.CreatedAt)
	if err != nil {
		return Match{}, err
	}
	if len(setScoresRaw) > 0 {
		_ = json.Unmarshal(setScoresRaw, &m.SetScores)
	}
	if m.SetScores == nil {
		m.SetScores = []SetScore{}
	}
	if m.WinnerSide == "A" {
		m.Winner = m.PlayerAName
	} else {
		m.Winner = m.PlayerBName
	}
	return m, nil
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

	q := fmt.Sprintf(`
		INSERT INTO matches (sport, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
		                     sets_a, sets_b, set_scores, winner_side, created_by, played_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING %s`, matchCols)

	m, err := scanMatch(r.pool.QueryRow(ctx, q,
		in.Sport, in.PlayerAName, in.PlayerBName, in.PlayerAUserID, in.PlayerBUserID,
		in.SetsA, in.SetsB, raw, in.WinnerSide, createdBy, playedAt))
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
		SELECT ` + matchCols + ` FROM matches WHERE player_b_user_id = $1`
	args := []any{userID}
	filter := ""

	if f.Sport != "" {
		args = append(args, f.Sport)
		filter += fmt.Sprintf(" AND sport = $%d", len(args))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		filter += fmt.Sprintf(" AND (player_a_name ILIKE $%d OR player_b_name ILIKE $%d)", len(args), len(args))
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

// Sentinel errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid input")
	ErrConflict  = errors.New("conflict")
)
