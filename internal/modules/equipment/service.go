package equipment

import (
	"context"
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
	// Free players may own at most this many equipment rows; Pro is unlimited.
	freeEquipmentLimit = 2
)

// PushSender delivers OS push notifications to a user (FCM). Best-effort.
type PushSender interface {
	SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error
}

// Item is a persisted equipment row (internal + usage-derived fields).
type Item struct {
	ID                 string
	UserID             string
	Type               string
	Name               string
	PurchaseDate       *string
	LifespanHours      int
	AccumulatedSeconds int64
	IsActive           bool
	NotifiedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Usage is the API view with derived hours / remaining / percent.
type Usage struct {
	ID               string     `json:"id"`
	Type             string     `json:"type"`
	Name             string     `json:"name"`
	PurchaseDate     *string    `json:"purchase_date,omitempty"`
	LifespanHours    int        `json:"lifespan_hours"`
	AccumulatedHours float64    `json:"accumulated_hours"`
	RemainingHours   float64    `json:"remaining_hours"`
	UsagePercent     int        `json:"usage_percent"`
	IsActive         bool       `json:"is_active"`
	NotifiedAt       *time.Time `json:"notified_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// CreateInput is POST /v1/me/equipment.
type CreateInput struct {
	Type          string
	Name          string
	LifespanHours int
	PurchaseDate  *string
}

// UpdateInput is PATCH /v1/me/equipment/{id}.
type UpdateInput struct {
	Name            *string
	LifespanHours   *int
	PurchaseDate    *string
	SetPurchaseDate bool
	IsActive        *bool
}

// Service is the equipment domain.
type Service struct {
	repo  store
	gate  entitlement.Gate
	push  PushSender
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

func validType(t string) bool {
	return t == TypeString || t == TypeShoes
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: name required", ErrInvalid)
	}
	if len(name) > maxNameLen {
		return "", fmt.Errorf("%w: name too long", ErrInvalid)
	}
	return name, nil
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

func crossedThreshold(sec int64, lifespanHours int) bool {
	if lifespanHours <= 0 {
		return false
	}
	return sec >= int64(lifespanHours)*3600
}

// UsageFrom derives the API view. Exported for tests.
func UsageFrom(it Item) Usage {
	acc := hoursFromSeconds(it.AccumulatedSeconds)
	remaining := math.Round((float64(it.LifespanHours)-acc)*10) / 10
	if remaining < 0 {
		remaining = 0
	}
	return Usage{
		ID:               it.ID,
		Type:             it.Type,
		Name:             it.Name,
		PurchaseDate:     it.PurchaseDate,
		LifespanHours:    it.LifespanHours,
		AccumulatedHours: acc,
		RemainingHours:   remaining,
		UsagePercent:     usagePercent(it.AccumulatedSeconds, it.LifespanHours),
		IsActive:         it.IsActive,
		NotifiedAt:       it.NotifiedAt,
		CreatedAt:        it.CreatedAt,
		UpdatedAt:        it.UpdatedAt,
	}
}

func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (Usage, error) {
	pro, err := s.isPro(ctx, userID)
	if err != nil {
		return Usage{}, err
	}
	typ := strings.ToUpper(strings.TrimSpace(in.Type))
	if !validType(typ) {
		return Usage{}, fmt.Errorf("%w: type must be STRING or SHOES", ErrInvalid)
	}
	name, err := validateName(in.Name)
	if err != nil {
		return Usage{}, err
	}
	if in.LifespanHours <= 0 {
		return Usage{}, fmt.Errorf("%w: lifespan_hours must be > 0", ErrInvalid)
	}
	purchase, err := validatePurchaseDate(in.PurchaseDate)
	if err != nil {
		return Usage{}, err
	}
	in.Type, in.Name, in.PurchaseDate = typ, name, purchase
	it, err := s.repo.insertCreate(ctx, userID, in, pro)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
}

func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]Usage, int64, error) {
	items, total, err := s.repo.listForUser(ctx, userID, limit, offset)
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
	if in.Name != nil {
		name, err := validateName(*in.Name)
		if err != nil {
			return Usage{}, err
		}
		in.Name = &name
	}
	if in.LifespanHours != nil && *in.LifespanHours <= 0 {
		return Usage{}, fmt.Errorf("%w: lifespan_hours must be > 0", ErrInvalid)
	}
	if in.SetPurchaseDate {
		purchase, err := validatePurchaseDate(in.PurchaseDate)
		if err != nil {
			return Usage{}, err
		}
		in.PurchaseDate = purchase
	}
	clearNotified := in.LifespanHours != nil && *in.LifespanHours > existing.LifespanHours
	if in.IsActive != nil && *in.IsActive && !existing.IsActive {
		it, err := s.repo.activate(ctx, id, userID)
		if err != nil {
			return Usage{}, err
		}
		existing = it
	}
	it, err := s.repo.update(ctx, id, userID, in, clearNotified)
	if err != nil {
		return Usage{}, err
	}
	return UsageFrom(it), nil
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

// AccrueMatchDuration implements score.UsageAccruer. Best-effort; never fails the caller.
func (s *Service) AccrueMatchDuration(ctx context.Context, userID string, seconds int) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("equipment: accrue panic", "err", rec)
		}
	}()
	if seconds <= 0 || userID == "" {
		return
	}
	items, err := s.repo.accrueActive(ctx, userID, seconds)
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
	if it.NotifiedAt != nil || !crossedThreshold(it.AccumulatedSeconds, it.LifespanHours) {
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
	hours := fmt.Sprintf("%d", it.LifespanHours)
	title, body := reminderCopy(it.Type, it.LifespanHours)
	data := map[string]string{
		"type":           "equipment.reminder",
		"equipment_id":   it.ID,
		"equipment_type": it.Type,
		"hours":          hours,
	}
	if err := s.push.SendToUser(ctx, it.UserID, title, body, data); err != nil {
		slog.Warn("equipment: push", "err", err, "user", it.UserID)
	}
}

func reminderCopy(typ string, hours int) (string, string) {
	if typ == TypeShoes {
		return "Time for new shoes",
			fmt.Sprintf("Your shoes have %d hours — time for a new pair", hours)
	}
	return "Time to restring",
		fmt.Sprintf("Your string has %d hours — time to restring", hours)
}
