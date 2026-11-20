package equipment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/entitlement"
	"courtscore/internal/platform/httpx"
)

type stubEquip struct {
	items []Usage
	total int64
	one   Usage
	err   error

	winRate WinRate

	gotFilter ListFilter
	gotCreate CreateInput
	gotUpdate UpdateInput
	gotRenew  RenewInput
}

func (s *stubEquip) List(_ context.Context, _ string, f ListFilter, _, _ int) ([]Usage, int64, error) {
	s.gotFilter = f
	return s.items, s.total, s.err
}
func (s *stubEquip) Create(_ context.Context, _ string, in CreateInput) (Usage, error) {
	s.gotCreate = in
	return s.one, s.err
}
func (s *stubEquip) Get(context.Context, string, string) (Usage, error) { return s.one, s.err }
func (s *stubEquip) Update(_ context.Context, _, _ string, in UpdateInput) (Usage, error) {
	s.gotUpdate = in
	return s.one, s.err
}
func (s *stubEquip) Delete(context.Context, string, string) error { return s.err }
func (s *stubEquip) Activate(context.Context, string, string) (Usage, error) {
	return s.one, s.err
}
func (s *stubEquip) Reset(context.Context, string, string) (Usage, error)   { return s.one, s.err }
func (s *stubEquip) Archive(context.Context, string, string) (Usage, error) { return s.one, s.err }
func (s *stubEquip) Renew(_ context.Context, _, _ string, in RenewInput) (Usage, error) {
	s.gotRenew = in
	return s.one, s.err
}
func (s *stubEquip) WinRate(context.Context, string, string) (WinRate, error) {
	return s.winRate, s.err
}
func (s *stubEquip) GetCatalog(context.Context, string) ([]CatalogView, error) {
	return nil, s.err
}

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, userID))
}

func TestList_PageEnvelopeAndFilters(t *testing.T) {
	stub := &stubEquip{
		items: []Usage{{ID: "e1", DisplayName: "Luxilon", Category: CategoryView{Code: "STRING"}}},
		total: 1,
	}
	h := &handler{svc: stub}
	r := withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment?page=1&page_size=20&view=archive&sport=PADEL&category_id=c1", nil), "u1")
	w := httptest.NewRecorder()
	h.list(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var page httpx.Page[Usage]
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 20 || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}
	if stub.gotFilter != (ListFilter{View: "archive", Sport: "PADEL", CategoryID: "c1"}) {
		t.Fatalf("filter=%+v", stub.gotFilter)
	}

	r = withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment?view=bogus", nil), "u1")
	w = httptest.NewRecorder()
	(&handler{svc: &stubEquip{err: ErrInvalid}}).list(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad view status=%d", w.Code)
	}
}

func TestCreate_201ParsesNewContractAndProRequired(t *testing.T) {
	stub := &stubEquip{one: Usage{ID: "e1", DisplayName: "Luxilon ALU Power"}}
	h := &handler{svc: stub}
	body := bytes.NewBufferString(`{"sport":"TENNIS","category_id":"c1","brand_id":"b1","model_id":"m1","nickname":"Match",
		"lifespan_hours":15,"lifespan_days":90,"purchase_date":"2026-01-10","attributes":{"tension_kg":"24/23"},"parent_item_id":"p1"}`)
	r := withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment", body), "u1")
	w := httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}
	in := stub.gotCreate
	if in.Sport != "TENNIS" || in.CategoryID != "c1" || *in.BrandID != "b1" || *in.ModelID != "m1" || *in.Nickname != "Match" ||
		*in.LifespanHours != 15 || *in.LifespanDays != 90 || *in.PurchaseDate != "2026-01-10" ||
		in.Attributes["tension_kg"] != "24/23" || *in.ParentItemID != "p1" || in.CustomBrandName != nil {
		t.Fatalf("input=%+v", in)
	}

	h = &handler{svc: &stubEquip{err: entitlement.ErrProRequired}}
	r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment",
		bytes.NewBufferString(`{"sport":"TENNIS","category_id":"c1","custom_brand_name":"X","lifespan_hours":25}`)), "u1")
	w = httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("pro status=%d", w.Code)
	}
	var payload map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	if payload["code"] != "pro_required" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestCreate_OldClientBodyIs400AndBadJSON(t *testing.T) {
	svc, _ := newSvc()
	h := &handler{svc: &realFacade{Service: svc}}
	// Pre-gear-bag body: {type,name,lifespan_hours} has no category_id.
	r := withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment",
		bytes.NewBufferString(`{"type":"STRING","name":"Luxilon","lifespan_hours":25}`)), "u1")
	w := httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("old body status=%d body=%s", w.Code, w.Body.String())
	}
	r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment", bytes.NewBufferString(`{`)), "u1")
	w = httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json status=%d", w.Code)
	}
}

// realFacade runs handlers against the real Service on the in-memory store.
type realFacade struct{ *Service }

func TestGet_404Foreign(t *testing.T) {
	h := &handler{svc: &stubEquip{err: ErrNotFound}}
	r := withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment/other", nil), "u1")
	r.SetPathValue("id", "other")
	w := httptest.NewRecorder()
	h.get(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestUpdate_LegacyNameAliasAndSetFlags(t *testing.T) {
	stub := &stubEquip{one: Usage{ID: "e1"}}
	h := &handler{svc: stub}
	r := withUser(httptest.NewRequest(http.MethodPatch, "/v1/me/equipment/e1",
		bytes.NewBufferString(`{"name":"Legacy label","custom_brand_name":"Home","lifespan_hours":30,"purchase_date":"2026-03-03"}`)), "u1")
	r.SetPathValue("id", "e1")
	w := httptest.NewRecorder()
	h.update(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	in := stub.gotUpdate
	if in.Nickname == nil || *in.Nickname != "Legacy label" || !in.SetBrand || in.SetModel || in.SetParent ||
		!in.SetPurchaseDate || *in.LifespanHours != 30 {
		t.Fatalf("update input=%+v", in)
	}
}

func TestDelete_204(t *testing.T) {
	h := &handler{svc: &stubEquip{}}
	r := withUser(httptest.NewRequest(http.MethodDelete, "/v1/me/equipment/e1", nil), "u1")
	r.SetPathValue("id", "e1")
	w := httptest.NewRecorder()
	h.del(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestActivateAndReset_200(t *testing.T) {
	h := &handler{svc: &stubEquip{one: Usage{ID: "e1", IsActive: true}}}
	r := withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/activate", nil), "u1")
	r.SetPathValue("id", "e1")
	w := httptest.NewRecorder()
	h.activate(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("activate=%d", w.Code)
	}

	r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/reset", nil), "u1")
	r.SetPathValue("id", "e1")
	w = httptest.NewRecorder()
	h.reset(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("reset=%d", w.Code)
	}
}

func TestArchive_200AndConflict(t *testing.T) {
	h := &handler{svc: &stubEquip{one: Usage{ID: "e1"}}}
	r := withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/archive", nil), "u1")
	r.SetPathValue("id", "e1")
	w := httptest.NewRecorder()
	h.archive(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("archive=%d", w.Code)
	}

	h = &handler{svc: &stubEquip{err: ErrConflict}}
	w = httptest.NewRecorder()
	h.archive(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("conflict=%d", w.Code)
	}
}

func TestRenew_EmptyBodyAllowedAndStatuses(t *testing.T) {
	stub := &stubEquip{one: Usage{ID: "e2"}}
	h := &handler{svc: stub}
	r := withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/renew", nil), "u1")
	r.SetPathValue("id", "e1")
	w := httptest.NewRecorder()
	h.renew(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("empty body renew=%d body=%s", w.Code, w.Body.String())
	}

	r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/renew",
		bytes.NewBufferString(`{"custom_brand_name":"Home","attributes":{"tension_kg":"25/24"}}`)), "u1")
	r.SetPathValue("id", "e1")
	w = httptest.NewRecorder()
	h.renew(w, r)
	if w.Code != http.StatusCreated || *stub.gotRenew.CustomBrandName != "Home" || stub.gotRenew.Attributes["tension_kg"] != "25/24" {
		t.Fatalf("renew body=%d in=%+v", w.Code, stub.gotRenew)
	}

	r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/renew", bytes.NewBufferString(`{`)), "u1")
	r.SetPathValue("id", "e1")
	w = httptest.NewRecorder()
	h.renew(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json renew=%d", w.Code)
	}

	for err, want := range map[error]int{ErrConflict: http.StatusConflict, ErrInvalid: http.StatusBadRequest, ErrNotFound: http.StatusNotFound} {
		h = &handler{svc: &stubEquip{err: err}}
		r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment/e1/renew", nil), "u1")
		r.SetPathValue("id", "e1")
		w = httptest.NewRecorder()
		h.renew(w, r)
		if w.Code != want {
			t.Fatalf("renew %v => %d want %d", err, w.Code, want)
		}
	}
}
