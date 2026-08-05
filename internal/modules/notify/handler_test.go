package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
)

type stubSvc struct {
	token DeviceToken
	err   error
}

func (s *stubSvc) Register(_ context.Context, userID, token, platform string) (DeviceToken, error) {
	if s.err != nil {
		return DeviceToken{}, s.err
	}
	d := s.token
	d.UserID = userID
	d.Token = token
	d.Platform = platform
	return d, nil
}

func (s *stubSvc) Unregister(_ context.Context, _, _ string) error { return s.err }

func TestRegisterDevice_Returns201(t *testing.T) {
	svc := &stubSvc{token: DeviceToken{ID: "d1", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	h := &handler{svc: svc}
	body := []byte(`{"token":"abc","platform":"android"}`)
	r := httptest.NewRequest(http.MethodPost, "/v1/devices", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.register(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got DeviceToken
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Token != "abc" || got.Platform != "android" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRegisterDevice_Invalid(t *testing.T) {
	svc := &stubSvc{err: ErrInvalid}
	h := &handler{svc: svc}
	body := []byte(`{"token":"","platform":"android"}`)
	r := httptest.NewRequest(http.MethodPost, "/v1/devices", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.register(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestUnregisterDevice_Returns204(t *testing.T) {
	h := &handler{svc: &stubSvc{}}
	r := httptest.NewRequest(http.MethodDelete, "/v1/devices/abc%2Fdef", nil)
	r.SetPathValue("token", "abc/def")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.unregister(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestUnregisterDevice_Returns404(t *testing.T) {
	h := &handler{svc: &stubSvc{err: ErrNotFound}}
	r := httptest.NewRequest(http.MethodDelete, "/v1/devices/tok", nil)
	r.SetPathValue("token", "tok")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.unregister(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestUnregisterDevice_EmptyToken(t *testing.T) {
	h := &handler{svc: &stubSvc{}}
	r := httptest.NewRequest(http.MethodDelete, "/v1/devices/", nil)
	r.SetPathValue("token", "")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.unregister(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestNoopSender_Send(t *testing.T) {
	if err := (NoopSender{}).Send(context.Background(), []string{"t"}, "hi", "body", map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
}
