package equipment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func (m *memStore) catalogCategories(_ context.Context, sports []string) ([]catalogRow, error) {
	want := map[string]bool{}
	for _, s := range sports {
		want[s] = true
	}
	var out []catalogRow
	for _, r := range m.catalogRows {
		if want[r.Sport] {
			out = append(out, r)
		}
	}
	return out, nil
}

func sampleCatalogRows() []catalogRow {
	racket := CatalogCategory{ID: "c-racket", Code: "RACKET", ThresholdType: "NONE", DisplayOrder: 1}
	parent := "c-racket"
	str := CatalogCategory{ID: "c-string", Code: "STRING", ParentCategoryID: &parent, IsCritical: true,
		SupportsRenew: true, ThresholdType: "HOURS_OR_DAYS", DisplayOrder: 2}
	bab := &CatalogBrand{ID: "b-bab", Name: "Babolat", AccentColorHex: "#0B3D91"}
	lux := &CatalogBrand{ID: "b-lux", Name: "Luxilon", AccentColorHex: "#212121"}
	return []catalogRow{
		{Sport: "TENNIS", Category: racket, Brand: bab, Model: &CatalogModel{ID: "m1", Name: "Pure Aero"}},
		{Sport: "TENNIS", Category: racket, Brand: bab, Model: &CatalogModel{ID: "m2", Name: "Pure Drive"}},
		{Sport: "TENNIS", Category: str, Brand: lux, Model: &CatalogModel{ID: "m3", Name: "ALU Power"}},
		{Sport: "PADEL", Category: CatalogCategory{ID: "c-pala", Code: "PALA", ThresholdType: "HOURS", DisplayOrder: 1}},
	}
}

func TestBuildCatalog_NestsAndKeepsEmptySports(t *testing.T) {
	views := buildCatalog(catalogSports, sampleCatalogRows())
	if len(views) != 5 {
		t.Fatalf("want 5 sports, got %d", len(views))
	}
	tennis := views[0]
	if tennis.Sport != "TENNIS" || len(tennis.Categories) != 2 {
		t.Fatalf("tennis=%+v", tennis)
	}
	racket := tennis.Categories[0]
	if racket.Code != "RACKET" || len(racket.Brands) != 1 || len(racket.Brands[0].Models) != 2 {
		t.Fatalf("racket=%+v", racket)
	}
	str := tennis.Categories[1]
	if str.ParentCategoryID == nil || *str.ParentCategoryID != "c-racket" || !str.IsCritical || !str.SupportsRenew {
		t.Fatalf("string=%+v", str)
	}
	pala := views[1].Categories[0]
	if pala.Brands == nil || len(pala.Brands) != 0 {
		t.Fatalf("category without models must have empty (non-nil) brands: %+v", pala)
	}
	if views[2].Categories == nil || len(views[2].Categories) != 0 {
		t.Fatalf("empty sport must have empty categories array")
	}
}

func TestGetCatalog_SportFilterAndValidation(t *testing.T) {
	svc := &Service{repo: &memStore{catalogRows: sampleCatalogRows()}}
	ctx := context.Background()

	all, err := svc.GetCatalog(ctx, "")
	if err != nil || len(all) != 5 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}
	one, err := svc.GetCatalog(ctx, " tennis ")
	if err != nil || len(one) != 1 || one[0].Sport != "TENNIS" {
		t.Fatalf("one=%+v err=%v", one, err)
	}
	if _, err := svc.GetCatalog(ctx, "GOLF"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad sport err=%v", err)
	}
}

func TestCatalogHandler_ShapeAndErrors(t *testing.T) {
	svc := &Service{repo: &memStore{catalogRows: sampleCatalogRows()}}
	h := &handler{svc: &catalogOnly{svc: svc}}

	w := httptest.NewRecorder()
	h.catalog(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/equipment/catalog?sport=TENNIS", nil), "u1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var one CatalogView
	if err := json.Unmarshal(w.Body.Bytes(), &one); err != nil || one.Sport != "TENNIS" || len(one.Categories) != 2 {
		t.Fatalf("one=%+v err=%v body=%s", one, err, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.catalog(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/equipment/catalog", nil), "u1"))
	var all []CatalogView
	if err := json.Unmarshal(w.Body.Bytes(), &all); err != nil || len(all) != 5 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}

	w = httptest.NewRecorder()
	h.catalog(w, withUser(httptest.NewRequest(http.MethodGet, "/v1/equipment/catalog?sport=GOLF", nil), "u1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad sport status=%d", w.Code)
	}
}

// catalogOnly adapts *Service to svcFacade for handler tests that only hit GetCatalog.
type catalogOnly struct {
	stubEquip
	svc *Service
}

func (c *catalogOnly) GetCatalog(ctx context.Context, sport string) ([]CatalogView, error) {
	return c.svc.GetCatalog(ctx, sport)
}
