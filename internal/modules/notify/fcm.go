package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Sender delivers data+notification messages to device tokens.
type Sender interface {
	Send(ctx context.Context, tokens []string, title, body string, data map[string]string) error
}

// NoopSender logs and succeeds — used when FCM credentials are not configured.
type NoopSender struct{}

func (NoopSender) Send(_ context.Context, tokens []string, title, body string, data map[string]string) error {
	slog.Info("notify: noop push", "tokens", len(tokens), "title", title, "body", body, "data", data)
	return nil
}

// FCMSender sends via Firebase Cloud Messaging HTTP v1.
type FCMSender struct {
	projectID string
	client    *http.Client
	ts        oauth2.TokenSource
	mu        sync.Mutex
}

// NewFCMSenderFromEnv builds an FCM sender from FIREBASE_CREDENTIALS_JSON
// or GOOGLE_APPLICATION_CREDENTIALS. Returns NoopSender when unset.
func NewFCMSenderFromEnv() Sender {
	raw := os.Getenv("FIREBASE_CREDENTIALS_JSON")
	if raw == "" {
		if path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				slog.Warn("notify: cannot read GOOGLE_APPLICATION_CREDENTIALS", "err", err)
				return NoopSender{}
			}
			raw = string(b)
		}
	}
	if raw == "" {
		slog.Info("notify: FCM credentials not set — using NoopSender")
		return NoopSender{}
	}
	s, err := newFCMSender([]byte(raw))
	if err != nil {
		slog.Warn("notify: FCM init failed — using NoopSender", "err", err)
		return NoopSender{}
	}
	return s
}

func newFCMSender(credsJSON []byte) (*FCMSender, error) {
	var meta struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(credsJSON, &meta); err != nil || meta.ProjectID == "" {
		return nil, fmt.Errorf("invalid credentials: missing project_id")
	}
	cfg, err := google.JWTConfigFromJSON(credsJSON, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, err
	}
	return &FCMSender{
		projectID: meta.ProjectID,
		client:    &http.Client{Timeout: 10 * time.Second},
		ts:        cfg.TokenSource(context.Background()),
	}, nil
}

func (s *FCMSender) Send(ctx context.Context, tokens []string, title, body string, data map[string]string) error {
	if len(tokens) == 0 {
		return nil
	}
	tok, err := s.ts.Token()
	if err != nil {
		return err
	}
	for _, device := range tokens {
		if err := s.sendOne(ctx, tok.AccessToken, device, title, body, data); err != nil {
			slog.Warn("notify: fcm send failed", "err", err)
		}
	}
	return nil
}

func (s *FCMSender) sendOne(ctx context.Context, accessToken, device, title, body string, data map[string]string) error {
	payload := map[string]any{
		"message": map[string]any{
			"token": device,
			"notification": map[string]string{
				"title": title,
				"body":  body,
			},
			"data": data,
			"android": map[string]any{
				"priority": "high",
			},
			"apns": map[string]any{
				"payload": map[string]any{
					"aps": map[string]any{
						"sound": "default",
					},
				},
			},
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("fcm status %d: %s", res.StatusCode, string(b))
	}
	return nil
}
