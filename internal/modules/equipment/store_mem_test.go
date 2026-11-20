package equipment

import (
	"context"
	"sync"
	"time"

	"courtscore/internal/platform/entitlement"
)

// Fixture catalog ids used across tests.
const (
	catTRacket  = "cat-t-racket"
	catTString  = "cat-t-string"
	catTShoes   = "cat-t-shoes"
	catTOver    = "cat-t-overgrip"
	catPala     = "cat-p-pala"
	catBlade    = "cat-pp-blade"
	catRubberFH = "cat-pp-rubber-fh"
	catSqString = "cat-sq-string"

	brandLux = "b-lux"
	brandBab = "b-bab"

	modelAlu  = "m-alu"  // Luxilon ALU Power, Tennis/STRING
	modelPure = "m-pure" // Babolat Pure Drive, Tennis/RACKET
)

type memModel struct {
	Ref  ModelRef
	Name string
}

type memStore struct {
	mu          sync.Mutex
	items       []Item
	seq         int
	categories  map[string]Category
	brands      map[string]BrandView
	models      map[string]memModel
	catalogRows []catalogRow
	now         func() time.Time
	usage       []usageRow
}

func newMemStore() *memStore {
	racket := catTRacket
	blade := catBlade
	sqRacket := "cat-sq-racket"
	return &memStore{
		now: time.Now,
		categories: map[string]Category{
			catTRacket:  {ID: catTRacket, Sport: "TENNIS", Code: "RACKET", ThresholdType: ThresholdNone, IsActive: true},
			catTString:  {ID: catTString, Sport: "TENNIS", Code: "STRING", ParentCategoryID: &racket, IsCritical: true, SupportsRenew: true, ThresholdType: ThresholdHoursOrDays, IsActive: true},
			catTShoes:   {ID: catTShoes, Sport: "TENNIS", Code: "SHOES", ThresholdType: ThresholdHours, IsActive: true},
			catTOver:    {ID: catTOver, Sport: "TENNIS", Code: "OVERGRIP", ThresholdType: ThresholdHours, IsActive: true},
			catPala:     {ID: catPala, Sport: "PADEL", Code: "PALA", ThresholdType: ThresholdHours, IsActive: true},
			catBlade:    {ID: catBlade, Sport: "PING_PONG", Code: "BLADE", ThresholdType: ThresholdNone, IsActive: true},
			catRubberFH: {ID: catRubberFH, Sport: "PING_PONG", Code: "RUBBER_FH", ParentCategoryID: &blade, IsCritical: true, SupportsRenew: true, ThresholdType: ThresholdHours, IsActive: true},
			catSqString: {ID: catSqString, Sport: "SQUASH", Code: "STRING", ParentCategoryID: &sqRacket, IsCritical: true, SupportsRenew: true, ThresholdType: ThresholdHours, IsActive: true},
			"cat-off":   {ID: "cat-off", Sport: "TENNIS", Code: "OFF", ThresholdType: ThresholdHours, IsActive: false},
		},
		brands: map[string]BrandView{
			brandLux: {ID: brandLux, Name: "Luxilon", AccentColorHex: "#212121"},
			brandBab: {ID: brandBab, Name: "Babolat", AccentColorHex: "#0B3D91"},
		},
		models: map[string]memModel{
			modelAlu:  {Ref: ModelRef{CategoryID: catTString, BrandID: brandLux}, Name: "ALU Power"},
			modelPure: {Ref: ModelRef{CategoryID: catTRacket, BrandID: brandBab}, Name: "Pure Drive"},
		},
	}
}

func (m *memStore) nextID() string {
	m.seq++
	return "eq-" + itoa(m.seq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func (m *memStore) getCategory(_ context.Context, id string) (Category, error) {
	if c, ok := m.categories[id]; ok {
		return c, nil
	}
	return Category{}, ErrNotFound
}

func (m *memStore) brandExists(_ context.Context, id string) (bool, error) {
	_, ok := m.brands[id]
	return ok, nil
}

func (m *memStore) getModel(_ context.Context, id string) (ModelRef, error) {
	if mm, ok := m.models[id]; ok {
		return mm.Ref, nil
	}
	return ModelRef{}, ErrNotFound
}

// hydrate fills joined catalog fields and derived counters. Caller holds the lock.
func (m *memStore) hydrate(it Item) Item {
	c := m.categories[it.Category.ID]
	c.Sport = it.Sport
	it.Category = c
	if it.BrandID != nil {
		if b, ok := m.brands[*it.BrandID]; ok {
			n, h := b.Name, b.AccentColorHex
			it.BrandName, it.BrandAccentHex = &n, &h
		}
	}
	if it.ModelID != nil {
		if mm, ok := m.models[*it.ModelID]; ok {
			n := mm.Name
			it.ModelName = &n
		}
	}
	it.RenewalsThisYear = 0
	for _, o := range m.items {
		if o.LineageID == it.LineageID && o.RetiredAt != nil && o.RetiredAt.Year() == m.now().Year() {
			it.RenewalsThisYear++
		}
	}
	return it
}

func (m *memStore) countLive(userID string) int64 {
	var n int64
	for _, it := range m.items {
		if it.UserID == userID && it.RetiredAt == nil {
			n++
		}
	}
	return n
}

func (m *memStore) hasActiveLocked(userID, categoryID string) bool {
	for _, it := range m.items {
		if it.UserID == userID && it.Category.ID == categoryID && it.IsActive && it.RetiredAt == nil {
			return true
		}
	}
	return false
}

func (m *memStore) insertLocked(userID string, row itemRow, active bool, lineage string) Item {
	now := m.now()
	id := m.nextID()
	if lineage == "" {
		lineage = id
	}
	attrs := map[string]string{}
	for k, v := range row.Attributes {
		attrs[k] = v
	}
	it := Item{
		ID: id, UserID: userID, Sport: row.Sport, Category: Category{ID: row.CategoryID},
		LegacyName: row.LegacyName,
		BrandID:    row.BrandID, CustomBrandName: row.CustomBrandName, ModelID: row.ModelID, CustomModelName: row.CustomModelName,
		Nickname: row.Nickname, Attributes: attrs, PurchaseDate: row.PurchaseDate,
		LifespanHours: row.LifespanHours, LifespanDays: row.LifespanDays,
		IsActive: active, ParentItemID: row.ParentItemID, LineageID: lineage,
		CreatedAt: now, UpdatedAt: now,
	}
	m.items = append(m.items, it)
	return m.hydrate(it)
}

// seed inserts a row directly (test setup), bypassing validation and the free limit.
func (m *memStore) seed(userID, categoryID string, hours int, active bool) Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	cat := m.categories[categoryID]
	row := itemRow{Sport: cat.Sport, CategoryID: categoryID}
	if hours > 0 {
		row.LifespanHours = &hours
	}
	return m.insertLocked(userID, row, active, "")
}

func (m *memStore) insertCreate(_ context.Context, userID string, row itemRow, isPro bool) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !isPro && m.countLive(userID) >= freeEquipmentLimit {
		return Item{}, entitlement.ErrProRequired
	}
	return m.insertLocked(userID, row, !m.hasActiveLocked(userID, row.CategoryID), ""), nil
}

func (m *memStore) listForUser(_ context.Context, userID string, f ListFilter, limit, offset int) ([]Item, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Item
	for _, it := range m.items {
		if it.UserID != userID {
			continue
		}
		if (f.View == ViewArchive) != (it.RetiredAt != nil) {
			continue
		}
		if f.Sport != "" && it.Sport != f.Sport {
			continue
		}
		if f.CategoryID != "" && it.Category.ID != f.CategoryID {
			continue
		}
		all = append(all, m.hydrate(it))
	}
	total := int64(len(all))
	if offset > len(all) {
		return nil, total, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

func (m *memStore) getByID(_ context.Context, id, userID string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getLocked(id, userID)
}

func (m *memStore) getLocked(id, userID string) (Item, error) {
	for _, it := range m.items {
		if it.ID == id && it.UserID == userID {
			return m.hydrate(it), nil
		}
	}
	return Item{}, ErrNotFound
}

func (m *memStore) indexLocked(id, userID string) int {
	for i, it := range m.items {
		if it.ID == id && it.UserID == userID {
			return i
		}
	}
	return -1
}

func (m *memStore) update(_ context.Context, id, userID string, row itemRow, clearNotified bool) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(id, userID)
	if i < 0 {
		return Item{}, ErrNotFound
	}
	it := m.items[i]
	it.BrandID, it.CustomBrandName, it.ModelID, it.CustomModelName = row.BrandID, row.CustomBrandName, row.ModelID, row.CustomModelName
	it.Nickname, it.LifespanHours, it.LifespanDays, it.PurchaseDate = row.Nickname, row.LifespanHours, row.LifespanDays, row.PurchaseDate
	it.Attributes, it.ParentItemID = row.Attributes, row.ParentItemID
	if clearNotified {
		it.NotifiedAt = nil
	}
	it.UpdatedAt = m.now()
	m.items[i] = it
	return m.hydrate(it), nil
}

func (m *memStore) delete(_ context.Context, id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(id, userID)
	if i < 0 {
		return ErrNotFound
	}
	m.items = append(m.items[:i], m.items[i+1:]...)
	return nil
}

func (m *memStore) activate(_ context.Context, id, userID string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(id, userID)
	if i < 0 {
		return Item{}, ErrNotFound
	}
	if m.items[i].RetiredAt != nil {
		return Item{}, ErrInvalid
	}
	cat := m.items[i].Category.ID
	for j := range m.items {
		if m.items[j].UserID == userID && m.items[j].Category.ID == cat && m.items[j].IsActive && j != i && m.items[j].RetiredAt == nil {
			m.items[j].IsActive = false
		}
	}
	m.items[i].IsActive = true
	m.items[i].NotifiedAt = nil
	m.items[i].UpdatedAt = m.now()
	return m.hydrate(m.items[i]), nil
}

func (m *memStore) reset(_ context.Context, id, userID string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(id, userID)
	if i < 0 {
		return Item{}, ErrNotFound
	}
	m.items[i].AccumulatedSeconds = 0
	m.items[i].NotifiedAt = nil
	m.items[i].UpdatedAt = m.now()
	return m.hydrate(m.items[i]), nil
}

func (m *memStore) archive(_ context.Context, id, userID string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(id, userID)
	if i < 0 {
		return Item{}, ErrNotFound
	}
	if m.items[i].RetiredAt != nil {
		return Item{}, ErrConflict
	}
	now := m.now()
	m.items[i].RetiredAt = &now
	m.items[i].IsActive = false
	m.items[i].UpdatedAt = now
	return m.hydrate(m.items[i]), nil
}

func (m *memStore) renew(_ context.Context, id, userID string, row itemRow) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.indexLocked(id, userID)
	if i < 0 {
		return Item{}, ErrNotFound
	}
	if m.items[i].RetiredAt != nil {
		return Item{}, ErrConflict
	}
	now := m.now()
	wasActive := m.items[i].IsActive
	lineage := m.items[i].LineageID
	m.items[i].RetiredAt = &now
	m.items[i].IsActive = false
	return m.insertLocked(userID, row, wasActive, lineage), nil
}

func (m *memStore) accrueActive(_ context.Context, userID, sport string, seconds int) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Item
	for i := range m.items {
		if m.items[i].UserID == userID && m.items[i].Sport == sport && m.items[i].IsActive && m.items[i].RetiredAt == nil {
			m.items[i].AccumulatedSeconds += int64(seconds)
			m.items[i].UpdatedAt = m.now()
			out = append(out, m.hydrate(m.items[i]))
		}
	}
	return out, nil
}

func (m *memStore) markNotified(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == id && m.items[i].NotifiedAt == nil {
			now := m.now()
			m.items[i].NotifiedAt = &now
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) listDueReminders(context.Context) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Item
	for _, it := range m.items {
		it = m.hydrate(it)
		if it.IsActive && it.RetiredAt == nil && it.NotifiedAt == nil && crossedThreshold(it, m.now()) {
			out = append(out, it)
		}
	}
	return out, nil
}

// get returns the stored row without ownership checks (test inspection).
func (m *memStore) get(id string) Item {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.ID == id {
			return m.hydrate(it)
		}
	}
	return Item{}
}

func strp(s string) *string { return &s }
func intp(n int) *int       { return &n }

func (m *memStore) activeItemsForSport(_ context.Context, userID, sport string) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Item
	for _, it := range m.items {
		if it.UserID == userID && it.Sport == sport && it.IsActive && it.RetiredAt == nil {
			out = append(out, m.hydrate(it))
		}
	}
	return out, nil
}

func (m *memStore) insertUsage(_ context.Context, u usageRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.usage {
		if e.MatchID == u.MatchID && e.UserID == u.UserID {
			return nil
		}
	}
	m.usage = append(m.usage, u)
	return nil
}

func (m *memStore) deleteUsage(_ context.Context, matchID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.usage[:0]
	for _, e := range m.usage {
		if e.MatchID != matchID {
			kept = append(kept, e)
		}
	}
	m.usage = kept
	return nil
}

func (m *memStore) usageCounts(_ context.Context, userID, sport, comboKey string) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var played, won int
	for _, e := range m.usage {
		if e.UserID == userID && e.Sport == sport && e.ComboKey == comboKey {
			played++
			if e.Won {
				won++
			}
		}
	}
	return played, won, nil
}
