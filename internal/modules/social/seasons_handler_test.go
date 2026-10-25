package social

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
	"courtscore/internal/platform/httpx"
	"courtscore/internal/platform/sse"
)

func TestListSeasons_HandlerPageEnvelope(t *testing.T) {
	h := &handler{svc: &stubService{
		seasons:     []SeasonSummary{{ID: "s1", SeasonNumber: 1, Name: "Season 1", Status: seasonStatusActive}},
		seasonTotal: 1,
	}, hub: sse.NewHub()}
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/groups/g1/seasons?page=1&page_size=20", nil, "u1")
	req.SetPathValue("id", "g1")
	h.listSeasons(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var page httpx.Page[SeasonSummary]
	if err := json.NewDecoder(w.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 1 || page.PageSize != 20 {
		t.Fatalf("%+v", page)
	}
}

func TestGetCurrentSeason_Handler404(t *testing.T) {
	h := &handler{svc: &stubService{seasonErr: ErrNoActiveSeason}, hub: sse.NewHub()}
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/groups/g1/seasons/current", nil, "u1")
	req.SetPathValue("id", "g1")
	h.getCurrentSeason(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestGetSeason_Handler402Archive(t *testing.T) {
	h := &handler{svc: &stubService{seasonErr: entitlement.ErrProRequired}, hub: sse.NewHub()}
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/v1/groups/g1/seasons/s1", nil, "u1")
	req.SetPathValue("id", "g1")
	req.SetPathValue("seasonId", "s1")
	h.getSeason(w, req)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status %d", w.Code)
	}
}

func TestStartSeason_Handler402LargeLeague(t *testing.T) {
	h := &handler{svc: &stubService{seasonErr: entitlement.ErrProRequired}, hub: sse.NewHub()}
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/v1/groups/g1/seasons", []byte(`{"weeks":8}`), "owner")
	req.SetPathValue("id", "g1")
	h.startSeason(w, req)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status %d", w.Code)
	}
}

func TestStartSeason_Handler201(t *testing.T) {
	h := &handler{svc: &stubService{season: SeasonDetail{
		SeasonSummary: SeasonSummary{ID: "s1", SeasonNumber: 1, Name: "Season 1", Status: seasonStatusActive, StartsAt: time.Unix(1, 0), EndsAt: time.Unix(2, 0)},
		Divisions:     []Division{{Tier: 1, Name: "Division 1", Standings: []SeasonStanding{}}},
	}}, hub: sse.NewHub()}
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/v1/groups/g1/seasons", []byte(`{"weeks":8,"count_social_nights":true}`), "owner")
	req.SetPathValue("id", "g1")
	h.startSeason(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestFinalizeSeason_Handler200(t *testing.T) {
	h := &handler{svc: &stubService{season: SeasonDetail{
		SeasonSummary: SeasonSummary{ID: "s1", Status: seasonStatusCompleted},
		IsFrozen:      true,
	}}, hub: sse.NewHub()}
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/v1/groups/g1/seasons/s1/finalize", []byte(`{}`), "owner")
	req.SetPathValue("id", "g1")
	req.SetPathValue("seasonId", "s1")
	h.finalizeSeason(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
}
