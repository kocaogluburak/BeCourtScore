package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/sse"
)

// ── stub service ─────────────────────────────────────────────────────────────

type stubService struct {
	user       User
	getUserErr error
	updateErr  error
	session    Session
	isNewUser  bool
	authErr    error
	refreshErr error
	revokeErr  error
}

func (s *stubService) AuthWithProvider(_ context.Context, _, _ string) (Session, User, bool, error) {
	if s.authErr != nil {
		return Session{}, User{}, false, s.authErr
	}
	return s.session, s.user, s.isNewUser, nil
}
func (s *stubService) RefreshSession(_ context.Context, _ string) (Session, User, error) {
	if s.refreshErr != nil {
		return Session{}, User{}, s.refreshErr
	}
	return s.session, s.user, nil
}
func (s *stubService) RevokeSession(_ context.Context, _ string) error { return s.revokeErr }

func (s *stubService) GetUser(_ context.Context, _ string) (User, error) {
	return s.user, s.getUserErr
}

func (s *stubService) UpdateUser(_ context.Context, _ string, in UpdateInput) (User, error) {
	if s.updateErr != nil {
		return User{}, s.updateErr
	}
	u := s.user
	if in.Nickname != nil {
		u.Nickname = in.Nickname
	}
	if in.Name != nil {
		u.Name = in.Name
	}
	if in.Surname != nil {
		u.Surname = in.Surname
	}
	if in.ProfileIcon != nil {
		u.ProfileIcon = in.ProfileIcon
	}
	return u, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

var testSecret = []byte("handler-test-secret")

// authedRequest builds an *http.Request with a valid Bearer JWT in the context
// (simulating what authkit.Middleware would inject).
func authedRequest(method, target string, body []byte, userID string) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	ctx := context.WithValue(r.Context(), authkit.UserIDKey, userID)
	return r.WithContext(ctx)
}

func ptr(s string) *string { return &s }

func baseUser() User {
	email := "test@example.com"
	return User{
		ID:            "user-abc",
		Email:         &email,
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// ── GET /v1/me ────────────────────────────────────────────────────────────────

func TestGetMe_Returns200WithUser(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.getMe(w, authedRequest(http.MethodGet, "/v1/me", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got User
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-abc" {
		t.Errorf("user ID: got %q, want %q", got.ID, "user-abc")
	}
}

func TestGetMe_Returns404WhenNotFound(t *testing.T) {
	svc := &stubService{getUserErr: ErrNotFound}
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.getMe(w, authedRequest(http.MethodGet, "/v1/me", nil, "missing-user"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

func TestGetMe_Returns500OnUnexpectedError(t *testing.T) {
	svc := &stubService{getUserErr: ErrInvalidToken} // any non-ErrNotFound error
	h := &handler{svc: svc, hub: sse.NewHub()}

	w := httptest.NewRecorder()
	h.getMe(w, authedRequest(http.MethodGet, "/v1/me", nil, "user-abc"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", w.Code)
	}
}

// ── PATCH /v1/me ─────────────────────────────────────────────────────────────

func TestPatchMe_Returns200WithUpdatedUser(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{
		"nickname": "ace",
		"name":     "Merve",
	})

	w := httptest.NewRecorder()
	h.patchMe(w, authedRequest(http.MethodPatch, "/v1/me", body, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got User
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Nickname == nil || *got.Nickname != "ace" {
		t.Errorf("nickname: got %v, want %q", got.Nickname, "ace")
	}
	if got.Name == nil || *got.Name != "Merve" {
		t.Errorf("name: got %v, want %q", got.Name, "Merve")
	}
}

func TestPatchMe_AllProfileFields(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{
		"nickname":     "champ",
		"name":         "Ada",
		"surname":      "Lovelace",
		"profile_icon": "🎾",
	})

	w := httptest.NewRecorder()
	h.patchMe(w, authedRequest(http.MethodPatch, "/v1/me", body, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got User
	json.NewDecoder(w.Body).Decode(&got) //nolint:errcheck
	check := func(field string, got *string, want string) {
		t.Helper()
		if got == nil || *got != want {
			t.Errorf("%s: got %v, want %q", field, got, want)
		}
	}
	check("nickname", got.Nickname, "champ")
	check("name", got.Name, "Ada")
	check("surname", got.Surname, "Lovelace")
	check("profile_icon", got.ProfileIcon, "🎾")
}

func TestPatchMe_EmptyBody_StillReturns200(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]any{})
	w := httptest.NewRecorder()
	h.patchMe(w, authedRequest(http.MethodPatch, "/v1/me", body, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestPatchMe_Returns400OnMalformedJSON(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}

	r := httptest.NewRequest(http.MethodPatch, "/v1/me", strings.NewReader("{invalid"))
	r.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(r.Context(), authkit.UserIDKey, "user-abc")
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	h.patchMe(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestPatchMe_Returns404WhenUserNotFound(t *testing.T) {
	svc := &stubService{user: baseUser(), updateErr: ErrNotFound}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{"nickname": "ghost"})
	w := httptest.NewRecorder()
	h.patchMe(w, authedRequest(http.MethodPatch, "/v1/me", body, "gone-user"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

func TestPatchMe_Returns409WhenNicknameTaken(t *testing.T) {
	svc := &stubService{user: baseUser(), updateErr: ErrConflict}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{"nickname": "Burkinefasso"})
	w := httptest.NewRecorder()
	h.patchMe(w, authedRequest(http.MethodPatch, "/v1/me", body, "user-abc"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

func TestPatchMe_Returns500OnUnexpectedError(t *testing.T) {
	svc := &stubService{user: baseUser(), updateErr: ErrInvalidToken}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{"nickname": "x"})
	w := httptest.NewRecorder()
	h.patchMe(w, authedRequest(http.MethodPatch, "/v1/me", body, "user-abc"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", w.Code)
	}
}

// ── Auth handlers (ID-BE-01) ──────────────────────────────────────────────────

func TestAuthWithProvider_Returns200(t *testing.T) {
	svc := &stubService{
		user:      baseUser(),
		isNewUser: true,
		session: Session{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresIn:    3600,
		},
	}
	h := &handler{svc: svc, hub: sse.NewHub()}

	body, _ := json.Marshal(map[string]string{"id_token": "google-id"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("provider", "google")

	w := httptest.NewRecorder()
	h.authWithProvider(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["access_token"] != "access" {
		t.Errorf("access_token: %v", got["access_token"])
	}
	if got["is_new_user"] != true {
		t.Errorf("is_new_user: %v", got["is_new_user"])
	}
}

func TestAuthWithProvider_MissingToken(t *testing.T) {
	h := &handler{svc: &stubService{}, hub: sse.NewHub()}
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("provider", "google")

	w := httptest.NewRecorder()
	h.authWithProvider(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestAuthWithProvider_Unauthorized(t *testing.T) {
	svc := &stubService{authErr: errors.New("verify failed")}
	h := &handler{svc: svc, hub: sse.NewHub()}
	body, _ := json.Marshal(map[string]string{"id_token": "bad"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("provider", "google")

	w := httptest.NewRecorder()
	h.authWithProvider(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", w.Code)
	}
}

func TestRefresh_Returns200(t *testing.T) {
	svc := &stubService{
		user: baseUser(),
		session: Session{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		},
	}
	h := &handler{svc: svc, hub: sse.NewHub()}
	body, _ := json.Marshal(map[string]string{"refresh_token": "old-refresh"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.refresh(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	svc := &stubService{refreshErr: ErrInvalidToken}
	h := &handler{svc: svc, hub: sse.NewHub()}
	body, _ := json.Marshal(map[string]string{"refresh_token": "expired"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.refresh(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", w.Code)
	}
}

func TestLogout_Returns204(t *testing.T) {
	h := &handler{svc: &stubService{}, hub: sse.NewHub()}
	body, _ := json.Marshal(map[string]string{"refresh_token": "tok"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.logout(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", w.Code)
	}
}

// ── Middleware integration: missing / invalid token ───────────────────────────

func buildRouter(h *handler) http.Handler {
	mux := http.NewServeMux()
	authMW := authkit.Middleware(testSecret)
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/me":
			h.getMe(w, r)
		case "PATCH /v1/me":
			h.patchMe(w, r)
		}
	})
	mux.Handle("/v1/me", authMW(protected))
	return mux
}

func TestGetMe_Returns401WithNoToken(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}
	srv := httptest.NewServer(buildRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestGetMe_Returns401WithInvalidToken(t *testing.T) {
	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}
	srv := httptest.NewServer(buildRouter(h))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/me", nil)
	req.Header.Set("Authorization", "Bearer notavalidtoken")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
}

func TestPatchMe_Returns401WithExpiredToken(t *testing.T) {
	expiredToken, _ := authkit.IssueAccessToken("user-abc", testSecret, -time.Second)

	svc := &stubService{user: baseUser()}
	h := &handler{svc: svc, hub: sse.NewHub()}
	srv := httptest.NewServer(buildRouter(h))
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"nickname": "ghost"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/me", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
}
