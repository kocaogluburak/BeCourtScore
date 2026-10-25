package invite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserSummary is the public projection of an inviter (no email).
type UserSummary struct {
	ID          string  `json:"id"`
	Nickname    *string `json:"nickname"`
	Name        *string `json:"name"`
	Surname     *string `json:"surname"`
	ProfileIcon *string `json:"profile_icon"`
}

// Share is a minted invite_shares row.
type Share struct {
	ID            string
	Code          string
	InviterID     string
	Source        string
	Sport         *string
	SourceMatchID *string
	ClickCount    int
	SignupCount   int
	CreatedAt     time.Time
	LastClickedAt *time.Time
	Inviter       UserSummary
}

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

type repo struct{ pool *pgxpool.Pool }

func (r *repo) insert(ctx context.Context, inviterID, code, source string, sport, sourceMatchID *string) (Share, error) {
	var s Share
	err := r.pool.QueryRow(ctx, `
		INSERT INTO invite_shares (code, inviter_id, source, sport, source_match_id)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, code, inviter_id, source, sport, source_match_id, click_count, signup_count, created_at, last_clicked_at`,
		code, inviterID, source, sport, sourceMatchID,
	).Scan(&s.ID, &s.Code, &s.InviterID, &s.Source, &s.Sport, &s.SourceMatchID,
		&s.ClickCount, &s.SignupCount, &s.CreatedAt, &s.LastClickedAt)
	if err != nil {
		return Share{}, fmt.Errorf("insert invite: %w", err)
	}
	return s, nil
}

func (r *repo) getByCode(ctx context.Context, code string) (Share, error) {
	var s Share
	err := r.pool.QueryRow(ctx, `
		SELECT id, code, inviter_id, source, sport, source_match_id, click_count, signup_count, created_at, last_clicked_at
		  FROM invite_shares WHERE code = $1`, code,
	).Scan(&s.ID, &s.Code, &s.InviterID, &s.Source, &s.Sport, &s.SourceMatchID,
		&s.ClickCount, &s.SignupCount, &s.CreatedAt, &s.LastClickedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	if err != nil {
		return Share{}, fmt.Errorf("get invite: %w", err)
	}
	return s, nil
}

func (r *repo) incrementClick(ctx context.Context, code string) (Share, error) {
	var s Share
	err := r.pool.QueryRow(ctx, `
		UPDATE invite_shares
		   SET click_count = click_count + 1, last_clicked_at = NOW()
		 WHERE code = $1
		RETURNING id, code, inviter_id, source, sport, source_match_id, click_count, signup_count, created_at, last_clicked_at`,
		code,
	).Scan(&s.ID, &s.Code, &s.InviterID, &s.Source, &s.Sport, &s.SourceMatchID,
		&s.ClickCount, &s.SignupCount, &s.CreatedAt, &s.LastClickedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	if err != nil {
		return Share{}, fmt.Errorf("click invite: %w", err)
	}
	return s, nil
}

func (r *repo) recordSignup(ctx context.Context, code, userID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET invited_via_code = $1
		 WHERE id = $2 AND invited_via_code IS NULL`, code, userID)
	if err != nil {
		return fmt.Errorf("set invited_via_code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE invite_shares SET signup_count = signup_count + 1 WHERE code = $1`, code)
	if err != nil {
		return fmt.Errorf("inc signup_count: %w", err)
	}
	return nil
}

func (r *repo) userSummary(ctx context.Context, id string) (UserSummary, error) {
	var u UserSummary
	err := r.pool.QueryRow(ctx,
		`SELECT id, nickname, name, surname, profile_icon FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Nickname, &u.Name, &u.Surname, &u.ProfileIcon)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSummary{ID: id}, nil
	}
	if err != nil {
		return UserSummary{}, err
	}
	return u, nil
}
