package equipment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"courtscore/internal/platform/entitlement"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TypeString = "STRING"
	TypeShoes  = "SHOES"
	maxNameLen = 80
	// Free players may keep at most this many non-retired equipment rows; Pro is unlimited.
	// Archived (retired) rows do not count, so restring/regrip is net-zero (02-technical §2.4).
	freeEquipmentLimit = 2

	maxAttributeKeys     = 10
	maxAttributeKeyLen   = 40
	maxAttributeValueLen = 80

	ThresholdNone        = "NONE"
	ThresholdHours       = "HOURS"
	ThresholdHoursOrDays = "HOURS_OR_DAYS"

	ViewActive  = "active"
	ViewArchive = "archive"
)

// PushSender delivers OS push notifications to a user (FCM). Best-effort.
type PushSender interface {
	SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error
}

// Category is a catalog category as needed for validation / derived fields.
type Category struct {
	ID               string
	Sport            string
	Code             string
	ParentCategoryID *string
	IsCritical       bool
	SupportsRenew    bool
	ThresholdType    string
	IsActive         bool
}

// ModelRef is the catalog linkage of a model (for brand/category consistency checks).
type ModelRef struct {
	CategoryID string
	BrandID    string
}

// Item is a persisted equipment row joined with its catalog category/brand/model.
type Item struct {
	ID     string
	UserID string
	Sport  string

	LegacyType *string // physical `type` column (pre-gear-bag rows); NULL for new rows
	LegacyName *string // physical `name` column

	Category Category

	BrandID         *string
	BrandName       *string
	BrandAccentHex  *string
	ModelID         *string
	ModelName       *string
	CustomBrandName *string
	CustomModelName *string
	Nickname        *string
	Attributes      map[string]string

	PurchaseDate       *string
	LifespanHours      *int
	LifespanDays       *int
	AccumulatedSeconds int64
	IsActive           bool
	RetiredAt          *time.Time
	NotifiedAt         *time.Time
	ParentItemID       *string
	LineageID          string
	RenewalsThisYear   int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CategoryView is the category summary embedded in an item.
type CategoryView struct {
	ID               string  `json:"id"`
	Code             string  `json:"code"`
	IsCritical       bool    `json:"is_critical"`
	SupportsRenew    bool    `json:"supports_renew"`
	ParentCategoryID *string `json:"parent_category_id"`
	ThresholdType    string  `json:"threshold_type"`
}

// BrandView is the catalog brand embedded in an item.
type BrandView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	AccentColorHex string `json:"accent_color_hex"`
}

// ModelView is the catalog model embedded in an item.
type ModelView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Usage is the API view with derived hours / remaining / percent.
type Usage struct {
	ID               string            `json:"id"`
	Sport            string            `json:"sport"`
	Category         CategoryView      `json:"category"`
	Brand            *BrandView        `json:"brand"`
	CustomBrandName  *string           `json:"custom_brand_name"`
	Model            *ModelView        `json:"model"`
	CustomModelName  *string           `json:"custom_model_name"`
	Nickname         *string           `json:"nickname"`
	DisplayName      string            `json:"display_name"`
	Name             string            `json:"name"` // legacy alias of display_name (pre-gear-bag clients)
	Attributes       map[string]string `json:"attributes"`
	PurchaseDate     *string           `json:"purchase_date,omitempty"`
	LifespanHours    *int              `json:"lifespan_hours"`
	LifespanDays     *int              `json:"lifespan_days"`
	AccumulatedHours float64           `json:"accumulated_hours"`
	RemainingHours   float64           `json:"remaining_hours"`
	UsagePercent     int               `json:"usage_percent"`
	IsActive         bool              `json:"is_active"`
	RetiredAt        *time.Time        `json:"retired_at"`
	ParentItemID     *string           `json:"parent_item_id"`
	RenewalsThisYear int               `json:"renewals_this_year"`
	NotifiedAt       *time.Time        `json:"notified_at"`
	Type             string            `json:"type,omitempty"` // legacy: Tennis STRING/SHOES only
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// CreateInput is POST /v1/me/equipment.
type CreateInput struct {
	Sport           string
	CategoryID      string
	BrandID         *string
	CustomBrandName *string
	ModelID         *string
	CustomModelName *string
	Nickname        *string
	LifespanHours   *int
	LifespanDays    *int
	PurchaseDate    *string
	Attributes      map[string]string
	ParentItemID    *string
}

// UpdateInput is PATCH /v1/me/equipment/{id}. Sport and category are immutable.
type UpdateInput struct {
	Nickname *string // "" clears

	// SetBrand replaces the brand pair (exactly one of BrandID/CustomBrandName).
	// SetModel replaces the model pair. SetBrand without SetModel drops the catalog model.
	SetBrand        bool
	BrandID         *string
	CustomBrandName *string
	SetModel        bool
	ModelID         *string
	CustomModelName *string

	LifespanHours   *int
	LifespanDays    *int
	PurchaseDate    *string
	SetPurchaseDate bool
	Attributes      map[string]string // non-nil replaces
	SetParent       bool
	ParentItemID    *string // "" / nil with SetParent clears
	IsActive        *bool
}

// RenewInput is POST /v1/me/equipment/{id}/renew. Every empty field is copied from the old row.
type RenewInput struct {
	BrandID         *string
	CustomBrandName *string
	ModelID         *string
	CustomModelName *string
	Attributes      map[string]string // nil copies
}

// ListFilter narrows GET /v1/me/equipment.
type ListFilter struct {
	View       string // active (default) | archive
	Sport      string
	CategoryID string
}

// itemRow is a validated set of column values written by insert / update / renew.
type itemRow struct {
	Sport           string
	CategoryID      string
	BrandID         *string
	CustomBrandName *string
	ModelID         *string
	CustomModelName *string
	Nickname        *string
	LegacyName      *string
	PurchaseDate    *string
	LifespanHours   *int
	LifespanDays    *int
	Attributes      map[string]string
	ParentItemID    *string
}

// Service is the equipment domain.
type Service struct {
	repo store
	gate entitlement.Gate
	push PushSender
	now  func() time.Time // test seam; nil = time.Now
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// NewService wires equipment to Postgres.
func NewService(pool *pgxpool.Pool, push PushSender) *Service {
	return &Service{repo: &repo{pool: pool}, push: push}
}

func (s *Service) SetGate(g entitlement.Gate) { s.gate = g }

func (s *Service) isPro(ctx context.Context, userID string) (bool, error) {
	if s.gate == nil {
		return true, nil
	}
	return s.gate.IsPro(ctx, userID)
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func validateOptionalName(label string, p *string) (*string, error) {
	v := trimPtr(p)
	if v != nil && len(*v) > maxNameLen {
		return nil, fmt.Errorf("%w: %s too long", ErrInvalid, label)
	}
	return v, nil
}

func validatePurchaseDate(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return nil, fmt.Errorf("%w: purchase_date must be YYYY-MM-DD", ErrInvalid)
	}
	return &s, nil
}

func validateAttributes(attrs map[string]string) (map[string]string, error) {
	if len(attrs) > maxAttributeKeys {
		return nil, fmt.Errorf("%w: too many attributes", ErrInvalid)
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || len(k) > maxAttributeKeyLen || len(v) > maxAttributeValueLen {
			return nil, fmt.Errorf("%w: invalid attribute", ErrInvalid)
		}
		out[k] = v
	}
	return out, nil
}

func hoursFromSeconds(sec int64) float64 {
	return math.Round(float64(sec)/360.0) / 10.0
}

func usagePercent(accumulatedSeconds int64, lifespanHours int) int {
	if lifespanHours <= 0 {
		return 0
	}
	pct := int(math.Round(float64(accumulatedSeconds) / (float64(lifespanHours) * 3600.0) * 100.0))
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// crossedHours reports whether accumulated play time reached the hour lifespan.
func crossedHours(sec int64, lifespanHours int) bool {
	if lifespanHours <= 0 {
		return false
	}
	return sec >= int64(lifespanHours)*3600
}

// calendarDue reports whether the day lifespan elapsed: (purchase_date ?? created_at date) +
// lifespan_days <= today (UTC). Only HOURS_OR_DAYS categories use it.
func calendarDue(it Item, now time.Time) bool {
	if it.Category.ThresholdType != ThresholdHoursOrDays || it.LifespanDays == nil || *it.LifespanDays <= 0 {
		return false
	}
	base := it.CreatedAt.UTC()
	if it.PurchaseDate != nil {
		if d, err := time.Parse("2006-01-02", *it.PurchaseDate); err == nil {
			base = d
		}
	}
	baseDay := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.UTC)
	n := now.UTC()
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	return !today.Before(baseDay.AddDate(0, 0, *it.LifespanDays))
}

// hoursDue reports whether play time reached the hour lifespan (HOURS / HOURS_OR_DAYS).
func hoursDue(it Item) bool {
	switch it.Category.ThresholdType {
	case ThresholdHours, ThresholdHoursOrDays:
		return it.LifespanHours != nil && crossedHours(it.AccumulatedSeconds, *it.LifespanHours)
	default:
		return false
	}
}

// crossedThreshold reports whether the item is due for a reminder (by category threshold type).
func crossedThreshold(it Item, now time.Time) bool {
	return hoursDue(it) || calendarDue(it, now)
}

// legacyType derives the pre-gear-bag `type` field: only Tennis STRING/SHOES expose it.
func legacyType(sport, code string) string {
	if sport == "TENNIS" && (code == TypeString || code == TypeShoes) {
		return code
	}
	return ""
}

func derefDisplay(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// displayName = nickname ?? brand + model ?? legacy name ?? "—" (02-technical §2.2 / §3).
func displayName(it Item) string {
	if n := derefDisplay(it.Nickname); n != "" {
		return n
	}
	brand := derefDisplay(it.BrandName)
	if brand == "" {
		brand = derefDisplay(it.CustomBrandName)
	}
	model := derefDisplay(it.ModelName)
	if model == "" {
		model = derefDisplay(it.CustomModelName)
	}
	if derived := strings.TrimSpace(brand + " " + model); derived != "" {
		return derived
	}
	if n := derefDisplay(it.LegacyName); n != "" {
		return n
	}
	return "—"
}

// UsageFrom derives the API view. Exported for tests.
func UsageFrom(it Item) Usage {
	acc := hoursFromSeconds(it.AccumulatedSeconds)
	var remaining float64
	pct := 0
	if it.LifespanHours != nil {
		remaining = math.Round((float64(*it.LifespanHours)-acc)*10) / 10
		if remaining < 0 {
			remaining = 0
		}
		pct = usagePercent(it.AccumulatedSeconds, *it.LifespanHours)
	}
	attrs := it.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	var brand *BrandView
	if it.BrandID != nil {
		brand = &BrandView{ID: *it.BrandID, Name: derefStr(it.BrandName), AccentColorHex: derefStr(it.BrandAccentHex)}
	}
	var model *ModelView
	if it.ModelID != nil {
		model = &ModelView{ID: *it.ModelID, Name: derefStr(it.ModelName)}
	}
	dn := displayName(it)
	return Usage{
		ID:    it.ID,
		Sport: it.Sport,
		Category: CategoryView{
			ID: it.Category.ID, Code: it.Category.Code, IsCritical: it.Category.IsCritical,
			SupportsRenew: it.Category.SupportsRenew, ParentCategoryID: it.Category.ParentCategoryID,
			ThresholdType: it.Category.ThresholdType,
		},
		Brand:            brand,
		CustomBrandName:  it.CustomBrandName,
		Model:            model,
		CustomModelName:  it.CustomModelName,
		Nickname:         it.Nickname,
		DisplayName:      dn,
		Name:             dn,
		Attributes:       attrs,
		PurchaseDate:     it.PurchaseDate,
		LifespanHours:    it.LifespanHours,
		LifespanDays:     it.LifespanDays,
		AccumulatedHours: acc,
		RemainingHours:   remaining,
		UsagePercent:     pct,
		IsActive:         it.IsActive,
		RetiredAt:        it.RetiredAt,
		ParentItemID:     it.ParentItemID,
		RenewalsThisYear: it.RenewalsThisYear,
		NotifiedAt:       it.NotifiedAt,
		Type:             legacyType(it.Sport, it.Category.Code),
		CreatedAt:        it.CreatedAt,
		UpdatedAt:        it.UpdatedAt,
	}
}

// resolveLifespans applies the category threshold rules (02-technical §2.2 validation).
// 0 means "not provided"; negative is invalid.
func resolveLifespans(threshold string, hours, days *int) (*int, *int, error) {
	norm := func(label string, v *int) (*int, error) {
		if v == nil || *v == 0 {
			return nil, nil
		}
		if *v < 0 {
			return nil, fmt.Errorf("%w: %s must be > 0", ErrInvalid, label)
		}
		return v, nil
	}
	h, err := norm("lifespan_hours", hours)
	if err != nil {
		return nil, nil, err
	}
	d, err := norm("lifespan_days", days)
	if err != nil {
		return nil, nil, err
	}
	switch threshold {
	case ThresholdNone:
		return nil, nil, nil
	case ThresholdHours:
		if h == nil {
			return nil, nil, fmt.Errorf("%w: lifespan_hours must be > 0", ErrInvalid)
		}
		return h, nil, nil
	case ThresholdHoursOrDays:
		if h == nil && d == nil {
			return nil, nil, fmt.Errorf("%w: lifespan_hours or lifespan_days required", ErrInvalid)
		}
		return h, d, nil
	}
	return nil, nil, fmt.Errorf("%w: unknown threshold type", ErrInvalid)
}

// resolveBrandModel validates the brand/model pairs against the catalog for a category.
// brand_id XOR custom_brand_name is required; model_id must belong to category+brand.
func (s *Service) resolveBrandModel(ctx context.Context, cat Category, brandID, customBrand, modelID, customModel *string) (b, cb, m, cm *string, err error) {
	brandID, customBrand = trimPtr(brandID), trimPtr(customBrand)
	modelID, customModel = trimPtr(modelID), trimPtr(customModel)
	if (brandID == nil) == (customBrand == nil) {
		return nil, nil, nil, nil, fmt.Errorf("%w: exactly one of brand_id or custom_brand_name required", ErrInvalid)
	}
	if customBrand != nil && len(*customBrand) > maxNameLen {
		return nil, nil, nil, nil, fmt.Errorf("%w: custom_brand_name too long", ErrInvalid)
	}
	if customModel != nil && len(*customModel) > maxNameLen {
		return nil, nil, nil, nil, fmt.Errorf("%w: custom_model_name too long", ErrInvalid)
	}
	if brandID != nil {
		ok, err := s.repo.brandExists(ctx, *brandID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("%w: unknown brand_id", ErrInvalid)
		}
	}
	if modelID != nil {
		ref, err := s.repo.getModel(ctx, *modelID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, nil, nil, nil, fmt.Errorf("%w: unknown model_id", ErrInvalid)
			}
			return nil, nil, nil, nil, err
		}
		if brandID == nil || ref.BrandID != *brandID || ref.CategoryID != cat.ID {
			return nil, nil, nil, nil, fmt.Errorf("%w: model_id does not match category and brand", ErrInvalid)
		}
	}
	return brandID, customBrand, modelID, customModel, nil
}

// resolveParent validates parent_item_id: owned by the user, in the category's parent category.
func (s *Service) resolveParent(ctx context.Context, userID string, cat Category, parentID *string) (*string, error) {
	parentID = trimPtr(parentID)
	if parentID == nil {
		return nil, nil
	}
	if cat.ParentCategoryID == nil {
		return nil, fmt.Errorf("%w: category has no parent category", ErrInvalid)
	}
	parent, err := s.repo.getByID(ctx, *parentID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: parent_item_id not found", ErrInvalid)
		}
		return nil, err
	}
	if parent.Category.ID != *cat.ParentCategoryID {
		return nil, fmt.Errorf("%w: parent_item_id has wrong category", ErrInvalid)
	}
	return parentID, nil
}

func (s *Service) categoryByID(ctx context.Context, id string) (Category, error) {
	cat, err := s.repo.getCategory(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Category{}, fmt.Errorf("%w: unknown category_id", ErrInvalid)
		}
		return Category{}, err
	}
	return cat, nil
}

func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (Usage, error) {
	pro, err := s.isPro(ctx, userID)
	if err != nil {
		return Usage{}, err
	}
	sport := strings.ToUpper(strings.TrimSpace(in.Sport))
	if !validSport(sport) {
		return Usage{}, fmt.Errorf("%w: unknown sport", ErrInvalid)
	}
	if strings.TrimSpace(in.CategoryID) == "" {
		return Usage{}, fmt.Errorf("%w: category_id required", ErrInvalid)
	}
	cat, err := s.categoryByID(ctx, in.CategoryID)
	if err != nil {
		return Usage{}, err
	}
	if cat.Sport != sport || !cat.IsActive {
		return Usage{}, fmt.Errorf("%w: category not available for sport", ErrInvalid)
	}
	brandID, customBrand, modelID, customModel, err := s.resolveBrandModel(ctx, cat, in.BrandID, in.CustomBrandName, in.ModelID, in.CustomModelName)
	if err != nil {
		return Usage{}, err
	}
	nickname, err := validateOptionalName("nickname", in.Nickname)
	if err != nil {
		return Usage{}, err
	}
	hours, days, err := resolveLifespans(cat.ThresholdType, in.LifespanHours, in.LifespanDays)
	if err != nil {
		return Usage{}, err
	}
	purchase, err := validatePurchaseDate(in.PurchaseDate)
	if err != nil {
		return Usage{}, err
	}
	attrs, err := validateAttributes(in.Attributes)
	if err != nil {
		return Usage{}, err
	}
	parent, err := s.resolveParent(ctx, userID, cat, in.ParentItemID)
	if err != nil {
		return Usage{}, err
	}
	it, err := s.repo.insertCreate(ctx, userID, itemRow{
		Sport: sport, CategoryID: cat.ID,
		BrandID: brandID, CustomBrandName: customBrand, ModelID: modelID, CustomModelName: customModel,
		Nickname: nickname, PurchaseDate: purchase, LifespanHours: hours, LifespanDays: days,
		Attributes: attrs, ParentItemID: parent,
	}, pro)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

func (s *Service) List(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Usage, int64, error) {
	f.View = strings.ToLower(strings.TrimSpace(f.View))
	switch f.View {
	case "":
		f.View = ViewActive
	case ViewActive, ViewArchive:
	default:
		return nil, 0, fmt.Errorf("%w: view must be active or archive", ErrInvalid)
	}
	f.Sport = strings.ToUpper(strings.TrimSpace(f.Sport))
	if f.Sport != "" && !validSport(f.Sport) {
		return nil, 0, fmt.Errorf("%w: unknown sport", ErrInvalid)
	}
	f.CategoryID = strings.TrimSpace(f.CategoryID)
	items, total, err := s.repo.listForUser(ctx, userID, f, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Usage, 0, len(items))
	for _, it := range items {
		out = append(out, UsageFrom(it))
	}
	return out, total, nil
}

func (s *Service) Get(ctx context.Context, userID, id string) (Usage, error) {
	it, err := s.repo.getByID(ctx, id, userID)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

func (s *Service) Update(ctx context.Context, userID, id string, in UpdateInput) (Usage, error) {
	existing, err := s.repo.getByID(ctx, id, userID)
	if err != nil {
		return Usage{}, err
	}
	cat := existing.Category

	row := itemRow{
		Sport: existing.Sport, CategoryID: cat.ID,
		BrandID: existing.BrandID, CustomBrandName: existing.CustomBrandName,
		ModelID: existing.ModelID, CustomModelName: existing.CustomModelName,
		Nickname: existing.Nickname, PurchaseDate: existing.PurchaseDate,
		LifespanHours: existing.LifespanHours, LifespanDays: existing.LifespanDays,
		Attributes: existing.Attributes, ParentItemID: existing.ParentItemID,
	}

	if in.Nickname != nil {
		nick, err := validateOptionalName("nickname", in.Nickname)
		if err != nil {
			return Usage{}, err
		}
		row.Nickname = nick
	}
	if in.SetBrand || in.SetModel {
		brandID, customBrand, modelID, customModel := row.BrandID, row.CustomBrandName, row.ModelID, row.CustomModelName
		if in.SetBrand {
			brandID, customBrand = in.BrandID, in.CustomBrandName
			if !in.SetModel {
				modelID = nil
			}
		}
		if in.SetModel {
			modelID, customModel = in.ModelID, in.CustomModelName
		}
		row.BrandID, row.CustomBrandName, row.ModelID, row.CustomModelName, err =
			s.resolveBrandModel(ctx, cat, brandID, customBrand, modelID, customModel)
		if err != nil {
			return Usage{}, err
		}
	}
	if in.LifespanHours != nil || in.LifespanDays != nil {
		h, d := existing.LifespanHours, existing.LifespanDays
		if in.LifespanHours != nil {
			h = in.LifespanHours
		}
		if in.LifespanDays != nil {
			d = in.LifespanDays
		}
		row.LifespanHours, row.LifespanDays, err = resolveLifespans(cat.ThresholdType, h, d)
		if err != nil {
			return Usage{}, err
		}
	}
	if in.SetPurchaseDate {
		purchase, err := validatePurchaseDate(in.PurchaseDate)
		if err != nil {
			return Usage{}, err
		}
		row.PurchaseDate = purchase
	}
	if in.Attributes != nil {
		row.Attributes, err = validateAttributes(in.Attributes)
		if err != nil {
			return Usage{}, err
		}
	}
	if in.SetParent {
		row.ParentItemID, err = s.resolveParent(ctx, userID, cat, in.ParentItemID)
		if err != nil {
			return Usage{}, err
		}
		if row.ParentItemID != nil && *row.ParentItemID == existing.ID {
			return Usage{}, fmt.Errorf("%w: parent_item_id cannot be itself", ErrInvalid)
		}
	}

	clearNotified := lifespanRaised(existing.LifespanHours, row.LifespanHours) ||
		lifespanRaised(existing.LifespanDays, row.LifespanDays)
	if in.IsActive != nil && *in.IsActive && !existing.IsActive {
		if _, err := s.repo.activate(ctx, id, userID); err != nil {
			return Usage{}, err
		}
	}
	it, err := s.repo.update(ctx, id, userID, row, clearNotified)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

func lifespanRaised(old, neu *int) bool {
	if neu == nil {
		return false
	}
	return old == nil || *neu > *old
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	return s.repo.delete(ctx, id, userID)
}

func (s *Service) Activate(ctx context.Context, userID, id string) (Usage, error) {
	it, err := s.repo.activate(ctx, id, userID)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

func (s *Service) Reset(ctx context.Context, userID, id string) (Usage, error) {
	it, err := s.repo.reset(ctx, id, userID)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

// Archive retires an item by hand (no replacement row is created).
func (s *Service) Archive(ctx context.Context, userID, id string) (Usage, error) {
	it, err := s.repo.archive(ctx, id, userID)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

// Renew retires the old row and opens a new generation in the same lineage (restring / regrip).
func (s *Service) Renew(ctx context.Context, userID, id string, in RenewInput) (Usage, error) {
	old, err := s.repo.getByID(ctx, id, userID)
	if err != nil {
		return Usage{}, err
	}
	if !old.Category.SupportsRenew {
		return Usage{}, fmt.Errorf("%w: category does not support renew", ErrInvalid)
	}
	if old.RetiredAt != nil {
		return Usage{}, fmt.Errorf("%w: item already archived", ErrConflict)
	}

	brandID, customBrand, modelID, customModel := old.BrandID, old.CustomBrandName, old.ModelID, old.CustomModelName
	newBrand := trimPtr(in.BrandID) != nil || trimPtr(in.CustomBrandName) != nil
	newModel := trimPtr(in.ModelID) != nil || trimPtr(in.CustomModelName) != nil
	if newBrand {
		// A new brand starts a new model choice; an unspecified model is not carried over.
		brandID, customBrand = in.BrandID, in.CustomBrandName
		modelID, customModel = nil, nil
	}
	if newModel {
		modelID, customModel = in.ModelID, in.CustomModelName
	}
	// Legacy rows may have no brand at all: only validate when something is (or becomes) set.
	if brandID != nil || customBrand != nil || newBrand || newModel {
		if brandID == nil && customBrand == nil {
			return Usage{}, fmt.Errorf("%w: exactly one of brand_id or custom_brand_name required", ErrInvalid)
		}
		brandID, customBrand, modelID, customModel, err = s.resolveBrandModel(ctx, old.Category, brandID, customBrand, modelID, customModel)
		if err != nil {
			return Usage{}, err
		}
	}
	attrs := old.Attributes
	if in.Attributes != nil {
		attrs, err = validateAttributes(in.Attributes)
		if err != nil {
			return Usage{}, err
		}
	}
	it, err := s.repo.renew(ctx, id, userID, itemRow{
		Sport: old.Sport, CategoryID: old.Category.ID,
		BrandID: brandID, CustomBrandName: customBrand, ModelID: modelID, CustomModelName: customModel,
		LegacyName: old.LegacyName, LifespanHours: old.LifespanHours, LifespanDays: old.LifespanDays,
		Attributes: attrs, ParentItemID: old.ParentItemID,
	})
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

// AccrueMatchDuration implements score.UsageAccruer. It adds seconds onto every active,
// non-retired item of the user in that sport (all categories at once). Best-effort; never
// fails the caller.
func (s *Service) AccrueMatchDuration(ctx context.Context, userID, sport string, seconds int) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("equipment: accrue panic", "err", rec)
		}
	}()
	sport = strings.ToUpper(strings.TrimSpace(sport))
	if seconds <= 0 || userID == "" || !validSport(sport) {
		return
	}
	items, err := s.repo.accrueActive(ctx, userID, sport, seconds)
	if err != nil {
		slog.Warn("equipment: accrue", "err", err, "user", userID)
		return
	}
	for _, it := range items {
		s.maybeRemind(ctx, it)
	}
}

// SweepReminders is the hourly backstop for items that crossed threshold
// while push was unavailable. Idempotent via notified_at.
func (s *Service) SweepReminders(ctx context.Context) error {
	items, err := s.repo.listDueReminders(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		s.maybeRemind(ctx, it)
	}
	return nil
}

func (s *Service) maybeRemind(ctx context.Context, it Item) {
	if it.NotifiedAt != nil || it.RetiredAt != nil || !crossedThreshold(it, s.clock()) {
		return
	}
	ok, err := s.repo.markNotified(ctx, it.ID)
	if err != nil {
		slog.Warn("equipment: mark notified", "err", err, "id", it.ID)
		return
	}
	if !ok {
		return
	}
	s.sendReminder(ctx, it)
}

func (s *Service) sendReminder(ctx context.Context, it Item) {
	if s.push == nil {
		return
	}
	hours, days := "", ""
	if it.LifespanHours != nil {
		hours = fmt.Sprintf("%d", *it.LifespanHours)
	}
	if it.LifespanDays != nil {
		days = fmt.Sprintf("%d", *it.LifespanDays)
	}
	usage := hours + " hours"
	if !hoursDue(it) {
		usage = days + " days"
	}
	title, body := reminderCopy(it.Category.Code, usage)
	data := map[string]string{
		"type":           "equipment.reminder",
		"equipment_id":   it.ID,
		"equipment_type": it.Category.Code,
		"sport":          it.Sport,
		"hours":          hours,
		"days":           days,
	}
	if err := s.push.SendToUser(ctx, it.UserID, title, body, data); err != nil {
		slog.Warn("equipment: push", "err", err, "user", it.UserID)
	}
}

// reminderCopy is the English fallback title/body of the data-only push; the clients
// render their own localized text from the data payload. usage is e.g. "15 hours" / "90 days".
func reminderCopy(code, usage string) (string, string) {
	switch code {
	case "SHOES":
		return "Time for new shoes",
			fmt.Sprintf("Your shoes have %s — time for a new pair", usage)
	case "STRING":
		return "Time to restring",
			fmt.Sprintf("Your string has %s — time to restring", usage)
	case "RUBBER_FH", "RUBBER_BH":
		return "Time for a new rubber",
			fmt.Sprintf("Your rubber has %s — time to replace it", usage)
	case "OVERGRIP":
		return "Time for a new overgrip",
			fmt.Sprintf("Your overgrip has %s — time to replace it", usage)
	default:
		return "Time to replace your gear",
			fmt.Sprintf("Your gear has %s — time to replace it", usage)
	}
}
