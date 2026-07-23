package score

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Match is a finished match stored in history.
type Match struct {
	ID            string    `json:"id"`
	Sport         string    `json:"sport"`
	PlayerAName   string    `json:"player_a_name"`
	PlayerBName   string    `json:"player_b_name"`
	PlayerAUserID *string   `json:"player_a_user_id"`
	PlayerBUserID *string   `json:"player_b_user_id"`
	SetsA         int       `json:"sets_a"`
	SetsB         int       `json:"sets_b"`
	WinnerSide    string    `json:"winner_side"`
	Winner        string    `json:"winner"` // display name derived from winner_side
	CreatedBy     string    `json:"created_by"`
	PlayedAt      time.Time `json:"played_at"`
	CreatedAt     time.Time `json:"created_at"`
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
       sets_a, sets_b, winner_side, created_by, played_at, created_at`

func scanMatch(row pgx.Row) (Match, error) {
	var m Match
	err := row.Scan(&m.ID, &m.Sport, &m.PlayerAName, &m.PlayerBName, &m.PlayerAUserID, &m.PlayerBUserID,
		&m.SetsA, &m.SetsB, &m.WinnerSide, &m.CreatedBy, &m.PlayedAt, &m.CreatedAt)
	if err != nil {
		return Match{}, err
	}
	if m.WinnerSide == "A" {
		m.Winner = m.PlayerAName
	} else {
		m.Winner = m.PlayerBName
	}
	return m, nil
}

func (r *repo) insert(ctx context.Context, createdBy string, in CreateInput) (Match, error) {
	playedAt := time.Now()
	if in.PlayedAt != nil {
		playedAt = *in.PlayedAt
	}

	q := fmt.Sprintf(`
		INSERT INTO matches (sport, player_a_name, player_b_name, player_a_user_id, player_b_user_id,
		                     sets_a, sets_b, winner_side, created_by, played_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING %s`, matchCols)

	m, err := scanMatch(r.pool.QueryRow(ctx, q,
		in.Sport, in.PlayerAName, in.PlayerBName, in.PlayerAUserID, in.PlayerBUserID,
		in.SetsA, in.SetsB, in.WinnerSide, createdBy, playedAt))
	if err != nil {
		return Match{}, fmt.Errorf("insert match: %w", err)
	}
	return m, nil
}

// listForUser returns matches where the user is a participant or the recorder,
// newest first, with the total count for pagination.
func (r *repo) listForUser(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Match, int64, error) {
	where := `(created_by = $1 OR player_a_user_id = $1 OR player_b_user_id = $1)`
	args := []any{userID}

	if f.Sport != "" {
		args = append(args, f.Sport)
		where += fmt.Sprintf(" AND sport = $%d", len(args))
	}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		where += fmt.Sprintf(" AND (player_a_name ILIKE $%d OR player_b_name ILIKE $%d)", len(args), len(args))
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM matches WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count matches: %w", err)
	}

	args = append(args, limit, offset)
	q := fmt.Sprintf(`SELECT %s FROM matches WHERE %s ORDER BY played_at DESC LIMIT $%d OFFSET $%d`,
		matchCols, where, len(args)-1, len(args))

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
)
