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
}

func (s *stubEquip) List(context.Context, string, int, int) ([]Usage, int64, error) {
	return s.items, s.total, s.err
}
func (s *stubEquip) Create(context.Context, string, CreateInput) (Usage, error) { return s.one, s.err }
func (s *stubEquip) Get(context.Context, string, string) (Usage, error)         { return s.one, s.err }
func (s *stubEquip) Update(context.Context, string, string, UpdateInput) (Usage, error) {
	return s.one, s.err
}
func (s *stubEquip) Delete(context.Context, string, string) error { return s.err }
func (s *stubEquip) Activate(context.Context, string, string) (Usage, error) {
	return s.one, s.err
}
func (s *stubEquip) Reset(context.Context, string, string) (Usage, error) { return s.one, s.err }
func (s *stubEquip) GetCatalog(context.Context, string) ([]CatalogView, error) {
	return nil, s.err
}

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, userID))
}

func TestList_PageEnvelope(t *testing.T) {
	h := &handler{svc: &stubEquip{
		items: []Usage{{ID: "e1", Type: TypeString, Name: "Luxilon", LifespanHours: 25}},
		total: 1,
	}}
	r := withUser(httptest.NewRequest(http.MethodGet, "/v1/me/equipment?page=1&page_size=20", nil), "u1")
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
}

func TestCreate_201AndProRequired(t *testing.T) {
	h := &handler{svc: &stubEquip{one: Usage{ID: "e1", Type: TypeString, Name: "Luxilon"}}}
	body := bytes.NewBufferString(`{"type":"STRING","name":"Luxilon","lifespan_hours":25}`)
	r := withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment", body), "u1")
	w := httptest.NewRecorder()
	h.create(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}

	h = &handler{svc: &stubEquip{err: entitlement.ErrProRequired}}
	r = withUser(httptest.NewRequest(http.MethodPost, "/v1/me/equipment",
		bytes.NewBufferString(`{"type":"STRING","name":"X","lifespan_hours":25}`)), "u1")
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
