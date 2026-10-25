package invite

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"courtscore/internal/platform/authkit"
)

type stubService struct {
	created Created
	pub     PublicInvite
	err     error
}

func (s *stubService) Create(_ context.Context, _ string, _ CreateInput) (Created, error) {
	return s.created, s.err
}
func (s *stubService) ResolvePublic(_ context.Context, _ string) (PublicInvite, error) {
	return s.pub, s.err
}

func TestCreateInvite_Returns201(t *testing.T) {
	h := &handler{svc: &stubService{created: Created{Code: "abc123", URL: "https://court-score.com/i/abc123"}}}
	body := []byte(`{"source":"share_card","sport":"TENNIS"}`)
	r := httptest.NewRequest(http.MethodPost, "/v1/invites", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got Created
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Code != "abc123" || got.URL == "" {
		t.Fatalf("created: %+v", got)
	}
}

func TestCreateInvite_InvalidSource400(t *testing.T) {
	h := &handler{svc: &stubService{err: ErrInvalid}}
	body := []byte(`{"source":"nope"}`)
	r := httptest.NewRequest(http.MethodPost, "/v1/invites", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "user-1"))
	w := httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestResolvePublic_IncrementsViaService(t *testing.T) {
	h := &handler{svc: &stubService{pub: PublicInvite{
		Inviter: UserSummary{ID: "inviter"},
		URL:     "https://court-score.com/i/abc",
	}}}
	r := httptest.NewRequest(http.MethodGet, "/v1/public/invite/abc", nil)
	r.SetPathValue("code", "abc")
	w := httptest.NewRecorder()
	h.resolvePublic(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestResolvePublic_NotFound(t *testing.T) {
	h := &handler{svc: &stubService{err: ErrNotFound}}
	r := httptest.NewRequest(http.MethodGet, "/v1/public/invite/missing", nil)
	r.SetPathValue("code", "missing")
	w := httptest.NewRecorder()
	h.resolvePublic(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}
