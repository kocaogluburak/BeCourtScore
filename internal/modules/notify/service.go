package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var validPlatforms = map[string]bool{
	"android": true, "ios": true, "web": true,
}

// store is the persistence surface used by Service.
type store interface {
	upsert(ctx context.Context, userID, token, platform string) (DeviceToken, error)
	delete(ctx context.Context, userID, token string) error
	tokensForUser(ctx context.Context, userID string) ([]string, error)
}

// Service manages device tokens and push fan-out.
type Service struct {
	repo   store
	sender Sender
}

func NewService(pool *pgxpool.Pool, sender Sender) *Service {
	if sender == nil {
		sender = NoopSender{}
	}
	return &Service{repo: &repo{pool: pool}, sender: sender}
}

func (s *Service) Register(ctx context.Context, userID, token, platform string) (DeviceToken, error) {
	token = strings.TrimSpace(token)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if token == "" {
		return DeviceToken{}, fmt.Errorf("%w: token required", ErrInvalid)
	}
	if !validPlatforms[platform] {
		return DeviceToken{}, fmt.Errorf("%w: platform must be android|ios|web", ErrInvalid)
	}
	return s.repo.upsert(ctx, userID, token, platform)
}

func (s *Service) Unregister(ctx context.Context, userID, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("%w: token required", ErrInvalid)
	}
	return s.repo.delete(ctx, userID, token)
}

// SendToUser implements score.PushSender — best-effort, never fails the caller.
func (s *Service) SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error {
	tokens, err := s.repo.tokensForUser(ctx, userID)
	if err != nil {
		slog.Warn("notify: list tokens failed", "user", userID, "err", err)
		return nil
	}
	if len(tokens) == 0 {
		return nil
	}
	if err := s.sender.Send(ctx, tokens, title, body, data); err != nil {
		slog.Warn("notify: send failed", "user", userID, "err", err)
	}
	return nil
}
