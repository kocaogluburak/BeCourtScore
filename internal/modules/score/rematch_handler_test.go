package score

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/platform/httpx"
)

func baseRematch() RematchChallenge {
	return RematchChallenge{
		ID: "rm-1", ChallengerID: "user-abc", OpponentID: "user-opp",
		Sport: "TENNIS", SetsToWin: 2, AdvantageEnabled: true, PointsToWin: 11,
		Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestCreateRematch_Returns201(t *testing.T) {
	svc := &stubService{rematch: baseRematch()}
	h := &handler{svc: svc}
	body := []byte(`{"opponent_id":"user-opp","sport":"TENNIS"}`)
	w := httptest.NewRecorder()
	h.createRematch(w, authedRequest(http.MethodPost, "/v1/rematches", body, "user-abc"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got RematchChallenge
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OpponentID != "user-opp" || got.Sport != "TENNIS" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestCreateRematch_ForbiddenNonFriend(t *testing.T) {
	svc := &stubService{rematchErr: ErrForbidden}
	h := &handler{svc: svc}
	body := []byte(`{"opponent_id":"stranger","sport":"TENNIS"}`)
	w := httptest.NewRecorder()
	h.createRematch(w, authedRequest(http.MethodPost, "/v1/rematches", body, "user-abc"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCreateRematch_ConflictPending(t *testing.T) {
	svc := &stubService{rematchErr: ErrConflict}
	h := &handler{svc: svc}
	body := []byte(`{"opponent_id":"user-opp","sport":"TENNIS"}`)
	w := httptest.NewRecorder()
	h.createRematch(w, authedRequest(http.MethodPost, "/v1/rematches", body, "user-abc"))
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestListIncomingRematches_Paged(t *testing.T) {
	svc := &stubService{rematch: baseRematch()}
	h := &handler{svc: svc}
	w := httptest.NewRecorder()
	h.listIncomingRematches(w, authedRequest(http.MethodGet, "/v1/rematches/incoming?page=1", nil, "user-opp"))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var got httpx.Page[RematchChallenge]
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("envelope: %+v", got)
	}
}

func TestAcceptRematch_ReturnsPrefill(t *testing.T) {
	svc := &stubService{prefill: RematchPrefill{
		Sport: "TENNIS", Opponent: UserSummary{ID: "user-abc"},
		SetsToWin: 2, AdvantageEnabled: true, PointsToWin: 11,
	}}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/rematches/rm-1/accept", nil, "user-opp")
	r.SetPathValue("id", "rm-1")
	w := httptest.NewRecorder()
	h.acceptRematch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got RematchPrefill
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Sport != "TENNIS" || got.Opponent.ID != "user-abc" {
		t.Fatalf("prefill: %+v", got)
	}
}

func TestAcceptRematch_WrongState409(t *testing.T) {
	svc := &stubService{rematchErr: ErrWrongState}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/rematches/rm-1/accept", nil, "user-opp")
	r.SetPathValue("id", "rm-1")
	w := httptest.NewRecorder()
	h.acceptRematch(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestRejectRematch_Returns200(t *testing.T) {
	svc := &stubService{rematch: baseRematch()}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/rematches/rm-1/reject", nil, "user-opp")
	r.SetPathValue("id", "rm-1")
	w := httptest.NewRecorder()
	h.rejectRematch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCancelRematch_Returns200(t *testing.T) {
	svc := &stubService{rematch: baseRematch()}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/rematches/rm-1/cancel", nil, "user-abc")
	r.SetPathValue("id", "rm-1")
	w := httptest.NewRecorder()
	h.cancelRematch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}
