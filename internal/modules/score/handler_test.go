package score

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

// ── stub service ─────────────────────────────────────────────────────────────

type stubService struct {
	match     Match
	live      LiveMatch
	matches   []Match
	total     int64
	createErr error
	listErr   error
	getErr    error
	deleteErr error
	liveErr   error

	gotLimit  int
	gotOffset int
}

func (s *stubService) CreateMatch(_ context.Context, userID string, in CreateInput) (Match, error) {
	if s.createErr != nil {
		return Match{}, s.createErr
	}
	m := s.match
	m.CreatedBy = userID
	m.Sport = in.Sport
	return m, nil
}

func (s *stubService) ListMyMatches(_ context.Context, _ string, _ ListFilter, limit, offset int) ([]Match, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	return s.matches, s.total, s.listErr
}

func (s *stubService) ListUserMatches(_ context.Context, _, _ string, _ ListFilter, limit, offset int) ([]Match, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	return s.matches, s.total, s.listErr
}

func (s *stubService) GetMatch(_ context.Context, _, _ string) (Match, error) {
	return s.match, s.getErr
}

func (s *stubService) DeleteMatch(_ context.Context, _, _ string) error { return s.deleteErr }

func (s *stubService) StartLiveMatch(_ context.Context, userID string, in LiveStartInput) (LiveMatch, error) {
	if s.liveErr != nil {
		return LiveMatch{}, s.liveErr
	}
	m := s.live
	m.CreatedBy = userID
	m.Sport = in.Sport
	m.Status = "IN_PROGRESS"
	return m, nil
}

func (s *stubService) GetLiveMatch(_ context.Context, _, _ string) (LiveMatch, error) {
	return s.live, s.liveErr
}

func (s *stubService) UpdateLiveMatch(_ context.Context, _, _ string, _ LiveScoreUpdate) (LiveMatch, error) {
	return s.live, s.liveErr
}

func (s *stubService) EndLiveMatch(_ context.Context, _, _ string, _ LiveEndInput) (LiveMatch, error) {
	return s.live, s.liveErr
}

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

func baseMatch() Match {
	return Match{
		ID: "match-1", Sport: "TENNIS",
		PlayerAName: "Ada", PlayerBName: "Grace",
		SetsA: 2, SetsB: 1, WinnerSide: "A", Winner: "Ada",
		CreatedBy: "user-abc", PlayedAt: time.Now(), CreatedAt: time.Now(),
	}
}

// ── POST /v1/matches ─────────────────────────────────────────────────────────

func TestCreateMatch_Returns201(t *testing.T) {
	svc := &stubService{match: baseMatch()}
	h := &handler{svc: svc}

	body, _ := json.Marshal(map[string]any{
		"sport": "TENNIS", "player_a_name": "Ada", "player_b_name": "Grace",
		"sets_a": 2, "sets_b": 1, "winner_side": "A",
	})
	w := httptest.NewRecorder()
	h.createMatch(w, authedRequest(http.MethodPost, "/v1/matches", body, "user-abc"))

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
	var got Match
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CreatedBy != "user-abc" {
		t.Errorf("created_by: got %q, want user-abc", got.CreatedBy)
	}
}

func TestCreateMatch_Returns400OnInvalidInput(t *testing.T) {
	svc := &stubService{createErr: ErrInvalid}
	h := &handler{svc: svc}

	body, _ := json.Marshal(map[string]any{"sport": "CHESS"})
	w := httptest.NewRecorder()
	h.createMatch(w, authedRequest(http.MethodPost, "/v1/matches", body, "user-abc"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

// ── GET /v1/matches ──────────────────────────────────────────────────────────

func TestListMyMatches_ReturnsPagedEnvelope(t *testing.T) {
	svc := &stubService{matches: []Match{baseMatch()}, total: 45}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listMyMatches(w, authedRequest(http.MethodGet, "/v1/matches?page=2", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if svc.gotLimit != 20 || svc.gotOffset != 20 {
		t.Errorf("pagination: got limit=%d offset=%d, want 20/20", svc.gotLimit, svc.gotOffset)
	}
	var got httpx.Page[Match]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 45 || got.Page != 2 || got.PageSize != 20 || !got.HasMore {
		t.Errorf("envelope: %+v", got)
	}
}

// ── GET /v1/users/{userID}/matches ───────────────────────────────────────────

func TestListUserMatches_Returns403WhenNotFriends(t *testing.T) {
	svc := &stubService{listErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listUserMatches(w, authedRequest(http.MethodGet, "/v1/users/other/matches", nil, "user-abc"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

// ── GET /v1/matches/{id} ─────────────────────────────────────────────────────

func TestGetMatch_Returns404WhenMissing(t *testing.T) {
	svc := &stubService{getErr: ErrNotFound}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatch(w, authedRequest(http.MethodGet, "/v1/matches/nope", nil, "user-abc"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

func TestGetMatch_Returns403WhenNoAccess(t *testing.T) {
	svc := &stubService{getErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatch(w, authedRequest(http.MethodGet, "/v1/matches/private", nil, "stranger"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

// ── DELETE /v1/matches/{id} ──────────────────────────────────────────────────

func TestDeleteMatch_Returns204(t *testing.T) {
	svc := &stubService{}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.deleteMatch(w, authedRequest(http.MethodDelete, "/v1/matches/match-1", nil, "user-abc"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", w.Code)
	}
}

func TestDeleteMatch_Returns404WhenNotOwnerOrMissing(t *testing.T) {
	svc := &stubService{deleteErr: ErrNotFound}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.deleteMatch(w, authedRequest(http.MethodDelete, "/v1/matches/match-1", nil, "other"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}
