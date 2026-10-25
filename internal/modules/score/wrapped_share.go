package score

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// WrappedShare is returned by POST /v1/me/wrapped/share.
type WrappedShare struct {
	Token string `json:"token"`
	URL   string `json:"url"`
	Year  int    `json:"year"`
}

func (s *Service) wrappedPublicURL(token string) string {
	origin := s.publicWebOrigin
	if origin == "" {
		origin = "https://court-score.com"
	}
	return origin + "/wrapped/" + token
}

// ShareWrapped mints (or reuses) a public token for the caller's year report.
// FREE. Snapshot is frozen at mint / refresh time.
func (s *Service) ShareWrapped(ctx context.Context, userID string, year int) (WrappedShare, error) {
	report, err := s.GetWrapped(ctx, userID, year)
	if err != nil {
		return WrappedShare{}, err
	}
	r, ok := s.repo.(*repo)
	if !ok {
		return WrappedShare{}, fmt.Errorf("wrapped share requires postgres repo")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return WrappedShare{}, fmt.Errorf("marshal wrapped snapshot: %w", err)
	}
	token, err := r.upsertWrappedShare(ctx, userID, year, raw)
	if err != nil {
		return WrappedShare{}, err
	}
	return WrappedShare{Token: token, URL: s.wrappedPublicURL(token), Year: year}, nil
}

// GetPublicWrapped returns a frozen snapshot for an unauthenticated viewer.
func (s *Service) GetPublicWrapped(ctx context.Context, token string) (WrappedReport, error) {
	r, ok := s.repo.(*repo)
	if !ok {
		return WrappedReport{}, ErrNotFound
	}
	raw, err := r.getWrappedShare(ctx, token)
	if err != nil {
		return WrappedReport{}, err
	}
	var report WrappedReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return WrappedReport{}, fmt.Errorf("decode wrapped snapshot: %w", err)
	}
	return report, nil
}

func (r *repo) upsertWrappedShare(ctx context.Context, userID string, year int, snapshot []byte) (string, error) {
	var existing string
	err := r.pool.QueryRow(ctx, `
		SELECT token FROM wrapped_shares WHERE user_id = $1 AND year = $2`, userID, year).Scan(&existing)
	if err == nil && existing != "" {
		if _, err := r.pool.Exec(ctx, `
			UPDATE wrapped_shares SET snapshot = $2 WHERE token = $1`, existing, snapshot); err != nil {
			return "", fmt.Errorf("refresh wrapped share: %w", err)
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup wrapped share: %w", err)
	}
	token, err := newShareToken()
	if err != nil {
		return "", err
	}
	var stored string
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO wrapped_shares (token, user_id, year, snapshot)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, year) DO UPDATE SET snapshot = EXCLUDED.snapshot
		RETURNING token`, token, userID, year, snapshot).Scan(&stored); err != nil {
		return "", fmt.Errorf("insert wrapped share: %w", err)
	}
	return stored, nil
}

func (r *repo) getWrappedShare(ctx context.Context, token string) ([]byte, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT snapshot FROM wrapped_shares WHERE token = $1`, token).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get wrapped share: %w", err)
	}
	return raw, nil
}
