package tournament

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"courtscore/internal/platform/authkit"
)

// ── stub service ─────────────────────────────────────────────────────────────

type stubService struct {
	tournament  Tournament
	view        TournamentView
	participant Participant
	bracket     Bracket
	match       TournamentMatch
	tournaments []Tournament
	participants []Participant
	total       int64

	createErr  error
	getErr     error
	joinErr    error
	listErr    error
	deleteErr  error
	leaveErr   error
	lockErr    error
	drawErr    error
	bracketErr error
	resultErr  error
	confirmErr error
}

func (s *stubService) CreateTournament(_ context.Context, userID string, _ CreateInput) (Tournament, error) {
	if s.createErr != nil {
		return Tournament{}, s.createErr
	}
	t := s.tournament
	t.OrganizerID = userID
	return t, nil
}
func (s *stubService) GetTournament(_ context.Context, _, _ string) (TournamentView, error) {
	return s.view, s.getErr
}
func (s *stubService) ListMine(_ context.Context, _ string, _ ListFilter, _, _ int) ([]Tournament, int64, error) {
	return s.tournaments, s.total, s.listErr
}
func (s *stubService) JoinTournament(_ context.Context, _, _, _ string) (Participant, error) {
	return s.participant, s.joinErr
}
func (s *stubService) ListParticipants(_ context.Context, _ string, _, _ int) ([]Participant, int64, error) {
	return s.participants, s.total, s.listErr
}
func (s *stubService) Delete(_ context.Context, _, _ string) error { return s.deleteErr }
func (s *stubService) Leave(_ context.Context, _, _ string) error  { return s.leaveErr }
func (s *stubService) Lock(_ context.Context, _, _ string) (Tournament, error) {
	return s.tournament, s.lockErr
}
func (s *stubService) Draw(_ context.Context, _, _, _ string, _ []string) (Bracket, error) {
	return s.bracket, s.drawErr
}
func (s *stubService) GetBracket(_ context.Context, _ string) (Bracket, error) {
	return s.bracket, s.bracketErr
}
func (s *stubService) SubmitResult(_ context.Context, _, _, _, _ string) (TournamentMatch, error) {
	return s.match, s.resultErr
}
func (s *stubService) ConfirmResult(_ context.Context, _, _ string, _ bool) (TournamentMatch, error) {
	return s.match, s.confirmErr
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

func jsonBody(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ── create ───────────────────────────────────────────────────────────────────

func TestCreate_Returns201(t *testing.T) {
	svc := &stubService{tournament: Tournament{ID: "t1", Slug: "s", JoinCode: "PADEL-ABCD"}}
	h := &handler{svc: svc}

	body := jsonBody(map[string]any{"name": "35 Yas Padel", "sport": "PADEL", "format": "SINGLE_ELIM"})
	w := httptest.NewRecorder()
	h.create(w, authedRequest(http.MethodPost, "/v1/tournaments", body, "org"))

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
	var got Tournament
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.OrganizerID != "org" {
		t.Errorf("organizer: got %q, want org", got.OrganizerID)
	}
}

func TestCreate_ReturnsRulesInResponse(t *testing.T) {
	svc := &stubService{tournament: Tournament{
		ID: "t1", Slug: "s", JoinCode: "PADEL-ABCD",
		SetsToWin: 2, AdvantageEnabled: true, PointsToWin: 11,
		OrganizerName: "Alice",
	}}
	h := &handler{svc: svc}

	body := jsonBody(map[string]any{
		"name": "Rules Test", "sport": "PADEL", "format": "SINGLE_ELIM",
		"sets_to_win": 2, "advantage_enabled": true, "points_to_win": 11,
	})
	w := httptest.NewRecorder()
	h.create(w, authedRequest(http.MethodPost, "/v1/tournaments", body, "org"))

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
	var got Tournament
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.SetsToWin != 2 {
		t.Errorf("sets_to_win: got %d, want 2", got.SetsToWin)
	}
	if !got.AdvantageEnabled {
		t.Error("advantage_enabled: want true")
	}
	if got.OrganizerName != "Alice" {
		t.Errorf("organizer_name: got %q, want Alice", got.OrganizerName)
	}
}

func TestCreate_Returns400OnInvalid(t *testing.T) {
	svc := &stubService{createErr: ErrInvalid}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.create(w, authedRequest(http.MethodPost, "/v1/tournaments", jsonBody(map[string]any{"sport": "CHESS"}), "org"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

// ── get / join ───────────────────────────────────────────────────────────────

func TestGetBySlug_Returns200(t *testing.T) {
	svc := &stubService{view: TournamentView{Tournament: Tournament{ID: "t1"}, IsOrganizer: true}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getBySlug(w, authedRequest(http.MethodGet, "/v1/tournaments/s", nil, "org"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestJoin_Returns200(t *testing.T) {
	svc := &stubService{participant: Participant{ID: "p1", DisplayName: "Ada"}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.join(w, authedRequest(http.MethodPost, "/v1/tournaments/s/join", jsonBody(map[string]any{"join_code": "PADEL-ABCD"}), "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestJoin_Returns403OnWrongCode(t *testing.T) {
	svc := &stubService{joinErr: ErrWrongCode}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.join(w, authedRequest(http.MethodPost, "/v1/tournaments/s/join", jsonBody(map[string]any{"join_code": "NOPE"}), "u1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestJoin_Returns409WhenFull(t *testing.T) {
	svc := &stubService{joinErr: ErrFull}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.join(w, authedRequest(http.MethodPost, "/v1/tournaments/s/join", jsonBody(map[string]any{"join_code": "PADEL-ABCD"}), "u1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

func TestJoin_Returns409WhenAlreadyJoined(t *testing.T) {
	svc := &stubService{joinErr: ErrAlreadyJoined}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.join(w, authedRequest(http.MethodPost, "/v1/tournaments/s/join", jsonBody(map[string]any{"join_code": "PADEL-ABCD"}), "u1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

func TestJoin_Returns409OnWrongState(t *testing.T) {
	svc := &stubService{joinErr: ErrWrongState}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.join(w, authedRequest(http.MethodPost, "/v1/tournaments/s/join", jsonBody(map[string]any{"join_code": "PADEL-ABCD"}), "u1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

// ── delete ───────────────────────────────────────────────────────────────────

func TestDelete_Returns204(t *testing.T) {
	svc := &stubService{}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.delete(w, authedRequest(http.MethodDelete, "/v1/tournaments/t1", nil, "org"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", w.Code)
	}
}

func TestDelete_Returns403WhenNotOrganizer(t *testing.T) {
	svc := &stubService{deleteErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.delete(w, authedRequest(http.MethodDelete, "/v1/tournaments/t1", nil, "u1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

// ── listMine filter ──────────────────────────────────────────────────────────

func TestListMine_Returns200WithSportFilter(t *testing.T) {
	svc := &stubService{tournaments: []Tournament{{ID: "t1", Sport: "PADEL"}}, total: 1}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listMine(w, authedRequest(http.MethodGet, "/v1/tournaments/mine?sport=PADEL", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestListMine_Returns400OnInvalidSport(t *testing.T) {
	svc := &stubService{listErr: ErrInvalid}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listMine(w, authedRequest(http.MethodGet, "/v1/tournaments/mine?sport=CHESS", nil, "u1"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestListMine_Returns200WithSearchQuery(t *testing.T) {
	svc := &stubService{tournaments: []Tournament{{ID: "t1", Name: "Padel Turnuvası"}}, total: 1}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listMine(w, authedRequest(http.MethodGet, "/v1/tournaments/mine?q=padel", nil, "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

// ── lock / draw ──────────────────────────────────────────────────────────────

func TestLock_Returns403WhenNotOrganizer(t *testing.T) {
	svc := &stubService{lockErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.lock(w, authedRequest(http.MethodPost, "/v1/tournaments/t1/lock", nil, "u1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestDraw_Returns200(t *testing.T) {
	svc := &stubService{bracket: Bracket{Format: "SINGLE_ELIM"}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.draw(w, authedRequest(http.MethodPost, "/v1/tournaments/t1/draw", jsonBody(map[string]any{"mode": "RANDOM"}), "org"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestDraw_Returns409OnWrongState(t *testing.T) {
	svc := &stubService{drawErr: ErrWrongState}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.draw(w, authedRequest(http.MethodPost, "/v1/tournaments/t1/draw", jsonBody(map[string]any{"mode": "RANDOM"}), "org"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

// ── result / confirm ─────────────────────────────────────────────────────────

func TestResult_Returns200(t *testing.T) {
	svc := &stubService{match: TournamentMatch{ID: "m1", Status: "COMPLETED"}}
	h := &handler{svc: svc}

	body := jsonBody(map[string]any{"winner_participant_id": "p1", "score_summary": "6-4 6-2"})
	w := httptest.NewRecorder()
	h.result(w, authedRequest(http.MethodPost, "/v1/tournaments/matches/m1/result", body, "org"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestResult_Returns400WhenWinnerMissing(t *testing.T) {
	svc := &stubService{}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.result(w, authedRequest(http.MethodPost, "/v1/tournaments/matches/m1/result", jsonBody(map[string]any{}), "org"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestResult_Returns403WhenForbidden(t *testing.T) {
	svc := &stubService{resultErr: ErrForbidden}
	h := &handler{svc: svc}

	body := jsonBody(map[string]any{"winner_participant_id": "p1"})
	w := httptest.NewRecorder()
	h.result(w, authedRequest(http.MethodPost, "/v1/tournaments/matches/m1/result", body, "stranger"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestResult_Returns409WhenNotReady(t *testing.T) {
	svc := &stubService{resultErr: ErrWrongState}
	h := &handler{svc: svc}

	body := jsonBody(map[string]any{"winner_participant_id": "p1"})
	w := httptest.NewRecorder()
	h.result(w, authedRequest(http.MethodPost, "/v1/tournaments/matches/m1/result", body, "org"))

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409", w.Code)
	}
}

func TestConfirm_Returns200(t *testing.T) {
	svc := &stubService{match: TournamentMatch{ID: "m1", Status: "COMPLETED"}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.confirm(w, authedRequest(http.MethodPost, "/v1/tournaments/matches/m1/confirm", jsonBody(map[string]any{"approve": true}), "u2"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestConfirm_Returns403WhenReporterConfirms(t *testing.T) {
	svc := &stubService{confirmErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.confirm(w, authedRequest(http.MethodPost, "/v1/tournaments/matches/m1/confirm", jsonBody(map[string]any{"approve": true}), "u1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}
