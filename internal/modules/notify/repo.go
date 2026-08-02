package notify

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid  = errors.New("invalid input")
	ErrNotFound = errors.New("device token not found")
)

type DeviceToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type repo struct {
	pool *pgxpool.Pool
}

func (r *repo) upsert(ctx context.Context, userID, token, platform string) (DeviceToken, error) {
	const q = `
		INSERT INTO device_tokens (user_id, token, platform)
		VALUES ($1, $2, $3)
		ON CONFLICT (token) DO UPDATE
		  SET user_id = EXCLUDED.user_id,
		      platform = EXCLUDED.platform,
		      updated_at = NOW()
		RETURNING id, user_id, token, platform, created_at, updated_at`
	var d DeviceToken
	err := r.pool.QueryRow(ctx, q, userID, token, platform).Scan(
		&d.ID, &d.UserID, &d.Token, &d.Platform, &d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

func (r *repo) delete(ctx context.Context, userID, token string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM device_tokens WHERE user_id = $1 AND token = $2`, userID, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) tokensForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT token FROM device_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *repo) deleteByToken(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM device_tokens WHERE token = $1`, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
