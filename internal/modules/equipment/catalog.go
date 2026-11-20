package equipment

import (
	"context"
	"fmt"
	"strings"
)

// Supported sport codes (same as score.validSports; no new sports are introduced).
var catalogSports = []string{"TENNIS", "PADEL", "SQUASH", "PING_PONG", "PICKLEBALL"}

func validSport(s string) bool {
	for _, v := range catalogSports {
		if v == s {
			return true
		}
	}
	return false
}

// CatalogModel is a popular model under a brand within one category.
type CatalogModel struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	DefaultLifespanHours *int   `json:"default_lifespan_hours"`
	DefaultLifespanDays  *int   `json:"default_lifespan_days"`
}

// CatalogBrand is a brand listed under a category; models are the rows of that category.
type CatalogBrand struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	AccentColorHex string         `json:"accent_color_hex"`
	Models         []CatalogModel `json:"models"`
}

// CatalogCategory is one sport x category row with nested brands/models.
type CatalogCategory struct {
	ID                   string         `json:"id"`
	Code                 string         `json:"code"`
	ParentCategoryID     *string        `json:"parent_category_id"`
	IsCritical           bool           `json:"is_critical"`
	SupportsRenew        bool           `json:"supports_renew"`
	ThresholdType        string         `json:"threshold_type"`
	DefaultLifespanHours *int           `json:"default_lifespan_hours"`
	DefaultLifespanDays  *int           `json:"default_lifespan_days"`
	DisplayOrder         int            `json:"display_order"`
	Brands               []CatalogBrand `json:"brands"`
}

// CatalogView is the per-sport catalog tree returned by GET /v1/equipment/catalog.
type CatalogView struct {
	Sport      string            `json:"sport"`
	Categories []CatalogCategory `json:"categories"`
}

// GetCatalog returns the catalog for one sport, or all five sports (in fixed order)
// when sport is empty. Unknown sport -> ErrInvalid.
func (s *Service) GetCatalog(ctx context.Context, sport string) ([]CatalogView, error) {
	sport = strings.ToUpper(strings.TrimSpace(sport))
	sports := catalogSports
	if sport != "" {
		if !validSport(sport) {
			return nil, fmt.Errorf("%w: unknown sport", ErrInvalid)
		}
		sports = []string{sport}
	}
	cats, err := s.repo.catalogCategories(ctx, sports)
	if err != nil {
		return nil, err
	}
	return buildCatalog(sports, cats), nil
}

// catalogRow is one flat (category, brand, model) row from the catalog join.
// Brand/model fields are empty for a category without models.
type catalogRow struct {
	Category CatalogCategory
	Sport    string
	Brand    *CatalogBrand
	Model    *CatalogModel
}

// buildCatalog folds flat rows into the nested tree. Rows must be ordered by
// sport, category display_order, brand name, model name; empty sports still appear.
func buildCatalog(sports []string, rows []catalogRow) []CatalogView {
	views := make([]CatalogView, 0, len(sports))
	index := map[string]int{}
	for _, sp := range sports {
		index[sp] = len(views)
		views = append(views, CatalogView{Sport: sp, Categories: []CatalogCategory{}})
	}
	catPos := map[string]int{}   // category id -> position in its sport view
	brandPos := map[string]int{} // category id + brand id -> position in category
	for _, row := range rows {
		vi, ok := index[row.Sport]
		if !ok {
			continue
		}
		v := &views[vi]
		ci, ok := catPos[row.Category.ID]
		if !ok {
			cat := row.Category
			cat.Brands = []CatalogBrand{}
			ci = len(v.Categories)
			catPos[cat.ID] = ci
			v.Categories = append(v.Categories, cat)
		}
		if row.Brand == nil || row.Model == nil {
			continue
		}
		cat := &v.Categories[ci]
		key := row.Category.ID + "|" + row.Brand.ID
		bi, ok := brandPos[key]
		if !ok {
			b := *row.Brand
			b.Models = []CatalogModel{}
			bi = len(cat.Brands)
			brandPos[key] = bi
			cat.Brands = append(cat.Brands, b)
		}
		cat.Brands[bi].Models = append(cat.Brands[bi].Models, *row.Model)
	}
	return views
}

// catalogCategories reads active categories (+ active brands/models) for the sports,
// ordered for buildCatalog.
func (r *repo) catalogCategories(ctx context.Context, sports []string) ([]catalogRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.sport, c.id, c.code, c.parent_category_id, c.is_critical, c.supports_renew,
		       c.threshold_type, c.default_lifespan_hours, c.default_lifespan_days, c.display_order,
		       b.id, b.name, b.accent_color_hex,
		       m.id, m.name, m.default_lifespan_hours, m.default_lifespan_days
		  FROM equipment_categories c
		  LEFT JOIN equipment_models m ON m.category_id = c.id AND m.is_active
		  LEFT JOIN equipment_brands b ON b.id = m.brand_id AND b.is_active
		 WHERE c.is_active AND c.sport = ANY($1)
		   AND (m.id IS NULL OR b.id IS NOT NULL)
		 ORDER BY array_position(ARRAY['TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL'], c.sport),
		          c.display_order, c.code, b.name, m.name`, sports)
	if err != nil {
		return nil, fmt.Errorf("catalog query: %w", err)
	}
	defer rows.Close()
	var out []catalogRow
	for rows.Next() {
		var cr catalogRow
		var brandID, brandName, brandHex, modelID, modelName *string
		var modelHours, modelDays *int
		cat := &cr.Category
		if err := rows.Scan(&cr.Sport, &cat.ID, &cat.Code, &cat.ParentCategoryID, &cat.IsCritical,
			&cat.SupportsRenew, &cat.ThresholdType, &cat.DefaultLifespanHours, &cat.DefaultLifespanDays,
			&cat.DisplayOrder, &brandID, &brandName, &brandHex, &modelID, &modelName, &modelHours, &modelDays); err != nil {
			return nil, fmt.Errorf("catalog scan: %w", err)
		}
		if brandID != nil && modelID != nil {
			cr.Brand = &CatalogBrand{ID: *brandID, Name: derefStr(brandName), AccentColorHex: derefStr(brandHex)}
			cr.Model = &CatalogModel{ID: *modelID, Name: derefStr(modelName), DefaultLifespanHours: modelHours, DefaultLifespanDays: modelDays}
		}
		out = append(out, cr)
	}
	return out, rows.Err()
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
