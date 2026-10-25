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
	tokensWithPlatform(ctx context.Context, userID string) ([]PlatformToken, error)
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

// SendLocalizedToUser fans out an i18n-clean push (APNs loc-key / Android data).
// Web tokens are skipped. Best-effort — never fails the caller.
func (s *Service) SendLocalizedToUser(ctx context.Context, userID, typ, titleLocKey, bodyLocKey string, locArgs []string, data map[string]string) error {
	tokens, err := s.repo.tokensWithPlatform(ctx, userID)
	if err != nil {
		slog.Warn("notify: list tokens failed", "user", userID, "err", err)
		return nil
	}
	var mobile []PlatformToken
	for _, t := range tokens {
		if t.Platform == "web" {
			continue
		}
		mobile = append(mobile, t)
	}
	if len(mobile) == 0 {
		return nil
	}
	msg := LocalizedPush{
		Type:        typ,
		TitleLocKey: titleLocKey,
		BodyLocKey:  bodyLocKey,
		LocArgs:     locArgs,
		Data:        data,
	}
	if err := s.sender.SendLocalized(ctx, mobile, msg); err != nil {
		slog.Warn("notify: localized send failed", "user", userID, "err", err)
	}
	return nil
}
