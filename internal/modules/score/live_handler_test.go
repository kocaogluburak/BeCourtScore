package score

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func baseLive() LiveMatch {
	opp := "user-opp"
	return LiveMatch{
		ID: "live-1", Sport: "TENNIS", Status: "IN_PROGRESS",
		PlayerAName: "Ada", PlayerBName: "Grace",
		PlayerBUserID: &opp,
		SetsToWin:     2, AdvantageEnabled: true, PointsToWin: 11,
		CreatedBy: "user-abc", StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestStartLiveMatch_Returns201(t *testing.T) {
	svc := &stubService{live: baseLive()}
	h := &handler{svc: svc}
	body := []byte(`{"sport":"TENNIS","player_a_name":"Ada","player_b_name":"Grace","player_b_user_id":"user-opp","sets_to_win":2}`)
	r := authedRequest(http.MethodPost, "/v1/live-matches", body, "user-abc")
	w := httptest.NewRecorder()
	h.startLiveMatch(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got LiveMatch
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Sport != "TENNIS" || got.CreatedBy != "user-abc" {
		t.Fatalf("unexpected live match: %+v", got)
	}
}

func TestGetLiveMatch_NotFound(t *testing.T) {
	svc := &stubService{liveErr: ErrNotFound}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodGet, "/v1/live-matches/x", nil, "user-abc")
	r.SetPathValue("id", "x")
	w := httptest.NewRecorder()
	h.getLiveMatch(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestUpdateLiveMatch_Forbidden(t *testing.T) {
	svc := &stubService{liveErr: ErrForbidden}
	h := &handler{svc: svc}
	body := []byte(`{"sets_a":1}`)
	r := authedRequest(http.MethodPatch, "/v1/live-matches/live-1", body, "user-other")
	r.SetPathValue("id", "live-1")
	w := httptest.NewRecorder()
	h.updateLiveMatch(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestEndLiveMatch_Returns200(t *testing.T) {
	live := baseLive()
	live.Status = "ENDED"
	side := "A"
	live.WinnerSide = &side
	svc := &stubService{live: live}
	h := &handler{svc: svc}
	body := []byte(`{"sets_a":2,"sets_b":0,"winner_side":"A"}`)
	r := authedRequest(http.MethodPost, "/v1/live-matches/live-1/end", body, "user-abc")
	r.SetPathValue("id", "live-1")
	w := httptest.NewRecorder()
	h.endLiveMatch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListMyOpenLiveMatches_Returns200(t *testing.T) {
	svc := &stubService{live: baseLive()}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodGet, "/v1/live-matches", nil, "user-abc")
	w := httptest.NewRecorder()
	h.listMyOpenLiveMatches(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCancelLiveMatch_Returns200(t *testing.T) {
	svc := &stubService{live: baseLive()}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/live-matches/live-1/cancel", nil, "user-abc")
	r.SetPathValue("id", "live-1")
	w := httptest.NewRecorder()
	h.cancelLiveMatch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAcceptLiveMatch_Returns200(t *testing.T) {
	live := baseLive()
	live.Status = "PENDING"
	svc := &stubService{live: live}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/live-matches/live-1/accept", nil, "user-opp")
	r.SetPathValue("id", "live-1")
	w := httptest.NewRecorder()
	h.acceptLiveMatch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got LiveMatch
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "IN_PROGRESS" {
		t.Fatalf("status=%q", got.Status)
	}
}

func TestDeclineLiveMatch_Returns200(t *testing.T) {
	live := baseLive()
	live.Status = "PENDING"
	svc := &stubService{live: live}
	h := &handler{svc: svc}
	r := authedRequest(http.MethodPost, "/v1/live-matches/live-1/decline", nil, "user-opp")
	r.SetPathValue("id", "live-1")
	w := httptest.NewRecorder()
	h.declineLiveMatch(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestStartLiveMatch_Conflict(t *testing.T) {
	svc := &stubService{liveErr: ErrConflict}
	h := &handler{svc: svc}
	body := []byte(`{"sport":"TENNIS","player_a_name":"Ada","player_b_name":"Grace","player_b_user_id":"user-opp"}`)
	r := authedRequest(http.MethodPost, "/v1/live-matches", body, "user-abc")
	w := httptest.NewRecorder()
	h.startLiveMatch(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestInviteeIDs(t *testing.T) {
	opp := "opp"
	partner := "partner"
	m := LiveMatch{CreatedBy: "me", PlayerA2UserID: &partner, PlayerBUserID: &opp}
	got := liveInvitees(m)
	if len(got) != 2 || got[0] != "partner" || got[1] != "opp" {
		t.Fatalf("got %v", got)
	}
	if len(inviteeIDs("opp", m.PlayerA2UserID, m.PlayerBUserID)) != 1 {
		t.Fatal("starter excluded")
	}
}
