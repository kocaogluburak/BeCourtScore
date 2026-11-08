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
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// PlatformToken is a device token plus its registered platform.
type PlatformToken struct {
	Token    string
	Platform string
}

// LocalizedPush is an i18n-clean payload: iOS uses APNs loc-key, Android is data-only.
type LocalizedPush struct {
	Type        string
	TitleLocKey string
	BodyLocKey  string
	LocArgs     []string
	Data        map[string]string
}

// Sender delivers data+notification messages to device tokens.
type Sender interface {
	Send(ctx context.Context, tokens []string, title, body string, data map[string]string) error
	SendLocalized(ctx context.Context, tokens []PlatformToken, msg LocalizedPush) error
}

// NoopSender logs and succeeds — used when FCM credentials are not configured.
type NoopSender struct{}

func (NoopSender) Send(_ context.Context, tokens []string, title, body string, data map[string]string) error {
	slog.Info("notify: noop push", "tokens", len(tokens), "title", title, "body", body, "data", data)
	return nil
}

func (NoopSender) SendLocalized(_ context.Context, tokens []PlatformToken, msg LocalizedPush) error {
	slog.Info("notify: noop localized push", "tokens", len(tokens), "type", msg.Type, "title_key", msg.TitleLocKey)
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
//
// Docker: mount the service-account JSON at GOOGLE_APPLICATION_CREDENTIALS
// (compose defaults to /run/secrets/firebase.json) or set FIREBASE_CREDENTIALS_JSON.
// A host-only path like /Users/... inside a container falls back to NoopSender.
func NewFCMSenderFromEnv() Sender {
	raw := strings.TrimSpace(os.Getenv("FIREBASE_CREDENTIALS_JSON"))
	if raw == "" {
		if path := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				slog.Warn("notify: cannot read GOOGLE_APPLICATION_CREDENTIALS — using NoopSender",
					"path", path, "err", err)
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
	slog.Info("notify: FCM sender ready", "project_id", s.projectID)
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
	return s.post(ctx, accessToken, payload)
}

func (s *FCMSender) SendLocalized(ctx context.Context, tokens []PlatformToken, msg LocalizedPush) error {
	if len(tokens) == 0 {
		return nil
	}
	tok, err := s.ts.Token()
	if err != nil {
		return err
	}
	for _, device := range tokens {
		if err := s.sendLocalizedOne(ctx, tok.AccessToken, device, msg); err != nil {
			slog.Warn("notify: fcm localized send failed", "err", err, "platform", device.Platform)
		}
	}
	return nil
}

func (s *FCMSender) sendLocalizedOne(ctx context.Context, accessToken string, device PlatformToken, msg LocalizedPush) error {
	message, ok := localizedMessage(device, msg)
	if !ok {
		return nil
	}
	return s.post(ctx, accessToken, map[string]any{"message": message})
}

// localizedMessage builds one FCM message. Web is data-only: no notification
// title or body. Unknown platforms are skipped.
func localizedMessage(device PlatformToken, msg LocalizedPush) (map[string]any, bool) {
	data := map[string]string{}
	for k, v := range msg.Data {
		data[k] = v
	}
	if msg.Type != "" {
		data["type"] = msg.Type
	}
	message := map[string]any{
		"token": device.Token,
		"data":  data,
	}
	switch device.Platform {
	case "ios":
		args := make([]string, len(msg.LocArgs))
		copy(args, msg.LocArgs)
		message["apns"] = map[string]any{
			"payload": map[string]any{
				"aps": map[string]any{
					"alert": map[string]any{
						"title-loc-key": msg.TitleLocKey,
						"loc-key":       msg.BodyLocKey,
						"loc-args":      args,
					},
					"sound": "default",
				},
			},
		}
	case "android":
		message["android"] = map[string]any{"priority": "high"}
	case "web":
		// data-only; the service worker resolves the loc keys
	default:
		return nil, false
	}
	return message, true
}

func (s *FCMSender) post(ctx context.Context, accessToken string, payload map[string]any) error {
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
