package equipment

import (
	"context"
	"sync"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
)

func TestHoursFromSeconds_RoundsOneDecimal(t *testing.T) {
	if got := hoursFromSeconds(66240); got != 18.4 {
		t.Fatalf("18.4h got=%v", got)
	}
	if got := hoursFromSeconds(0); got != 0 {
		t.Fatalf("0 got=%v", got)
	}
	if got := hoursFromSeconds(3600); got != 1 {
		t.Fatalf("1h got=%v", got)
	}
}

func TestUsagePercent_Clamps(t *testing.T) {
	if got := usagePercent(0, 25); got != 0 {
		t.Fatalf("0%% got=%d", got)
	}
	if got := usagePercent(90000, 25); got != 100 {
		t.Fatalf("100%% got=%d", got)
	}
	if got := usagePercent(180000, 25); got != 100 {
		t.Fatalf("over clamp got=%d", got)
	}
	if got := usagePercent(45000, 25); got != 50 {
		t.Fatalf("50%% got=%d", got)
	}
	if got := usagePercent(3600, 0); got != 0 {
		t.Fatalf("div0 got=%d", got)
	}
}

func TestAccrue_ActiveOnlyBothTypes(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store}
	ctx := context.Background()
	stringActive, _ := store.insert(ctx, "u1", CreateInput{Type: TypeString, Name: "Luxilon", LifespanHours: 25}, true)
	_, _ = store.insert(ctx, "u1", CreateInput{Type: TypeString, Name: "Spare", LifespanHours: 25}, false)
	shoes, _ := store.insert(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Asics", LifespanHours: 60}, true)

	svc.AccrueMatchDuration(ctx, "u1", 3600)

	gotS, _ := store.getByID(ctx, stringActive.ID, "u1")
	gotSpare, _ := store.getByID(ctx, store.items[1].ID, "u1")
	gotShoes, _ := store.getByID(ctx, shoes.ID, "u1")
	if gotS.AccumulatedSeconds != 3600 {
		t.Fatalf("active string sec=%d", gotS.AccumulatedSeconds)
	}
	if gotSpare.AccumulatedSeconds != 0 {
		t.Fatalf("inactive accrued=%d", gotSpare.AccumulatedSeconds)
	}
	if gotShoes.AccumulatedSeconds != 3600 {
		t.Fatalf("shoes sec=%d", gotShoes.AccumulatedSeconds)
	}
}

func TestAccrue_IgnoresNilZeroNegative(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store}
	ctx := context.Background()
	it, _ := store.insert(ctx, "u1", CreateInput{Type: TypeString, Name: "Luxilon", LifespanHours: 25}, true)

	svc.AccrueMatchDuration(ctx, "u1", 0)
	svc.AccrueMatchDuration(ctx, "u1", -10)
	got, _ := store.getByID(ctx, it.ID, "u1")
	if got.AccumulatedSeconds != 0 {
		t.Fatalf("zero/negative should no-op, sec=%d", got.AccumulatedSeconds)
	}

	svc.AccrueMatchDuration(ctx, "u1", 3600)
	got, _ = store.getByID(ctx, it.ID, "u1")
	if got.AccumulatedSeconds != 3600 {
		t.Fatalf("positive accrue sec=%d", got.AccumulatedSeconds)
	}

	svc.AccrueMatchDuration(ctx, "u1", 0)
	got, _ = store.getByID(ctx, it.ID, "u1")
	if got.AccumulatedSeconds != 3600 {
		t.Fatalf("zero after accrue sec=%d", got.AccumulatedSeconds)
	}
}

func TestThreshold_FiresOnceAndIdempotent(t *testing.T) {
	store := newMemStore()
	push := &recPush{}
	svc := &Service{repo: store, push: push}
	ctx := context.Background()
	it, _ := store.insert(ctx, "u1", CreateInput{Type: TypeString, Name: "Luxilon", LifespanHours: 1}, true)

	svc.AccrueMatchDuration(ctx, "u1", 1800)
	if push.calls != 0 {
		t.Fatalf("under threshold fired=%d", push.calls)
	}

	svc.AccrueMatchDuration(ctx, "u1", 1800)
	if push.calls != 1 || push.typ != "equipment.reminder" || push.eqType != TypeString {
		t.Fatalf("first cross: %+v", push)
	}
	got, _ := store.getByID(ctx, it.ID, "u1")
	if got.NotifiedAt == nil {
		t.Fatal("notified_at not stamped")
	}

	svc.AccrueMatchDuration(ctx, "u1", 1800)
	if push.calls != 1 {
		t.Fatalf("re-fired: %d", push.calls)
	}
}

func TestThreshold_RefiresAfterResetAndLifespanRaise(t *testing.T) {
	store := newMemStore()
	push := &recPush{}
	svc := &Service{repo: store, push: push}
	ctx := context.Background()
	it, _ := store.insert(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Asics", LifespanHours: 1}, true)

	svc.AccrueMatchDuration(ctx, "u1", 3600)
	if push.calls != 1 || push.eqType != TypeShoes {
		t.Fatalf("shoes payload: %+v", push)
	}

	if _, err := svc.Reset(ctx, "u1", it.ID); err != nil {
		t.Fatal(err)
	}
	svc.AccrueMatchDuration(ctx, "u1", 3600)
	if push.calls != 2 {
		t.Fatalf("reset should allow re-fire: %d", push.calls)
	}

	raised := 3
	if _, err := svc.Update(ctx, "u1", it.ID, UpdateInput{LifespanHours: &raised}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.getByID(ctx, it.ID, "u1")
	if got.NotifiedAt != nil {
		t.Fatal("raised lifespan should clear notified_at")
	}
	svc.AccrueMatchDuration(ctx, "u1", 3600*2)
	if push.calls != 3 {
		t.Fatalf("raise should allow re-fire: %d", push.calls)
	}
}

func TestSweepReminders_BackstopIdempotent(t *testing.T) {
	store := newMemStore()
	push := &recPush{}
	svc := &Service{repo: store, push: push}
	ctx := context.Background()
	it, _ := store.insert(ctx, "u1", CreateInput{Type: TypeString, Name: "Luxilon", LifespanHours: 1}, true)
	it.AccumulatedSeconds = 3600
	store.items[0] = it

	if err := svc.SweepReminders(ctx); err != nil {
		t.Fatal(err)
	}
	if push.calls != 1 {
		t.Fatalf("sweep calls=%d", push.calls)
	}
	if err := svc.SweepReminders(ctx); err != nil {
		t.Fatal(err)
	}
	if push.calls != 1 {
		t.Fatalf("sweep re-fired: %d", push.calls)
	}
}

type denyEquipGate struct{}

func (denyEquipGate) AllowCloudSync(context.Context, string) error            { return entitlement.ErrProRequired }
func (denyEquipGate) AllowStats(context.Context, string) error                { return entitlement.ErrProRequired }
func (denyEquipGate) AllowHeadToHead(context.Context, string) error           { return entitlement.ErrProRequired }
func (denyEquipGate) AllowAdvancedStats(context.Context, string) error        { return entitlement.ErrProRequired }
func (denyEquipGate) AllowPublicLive(context.Context, string) error           { return entitlement.ErrProRequired }
func (denyEquipGate) AllowLargeTournament(context.Context, string, int) error { return entitlement.ErrProRequired }
func (denyEquipGate) AllowLargeLeague(context.Context, string, int) error     { return entitlement.ErrProRequired }
func (denyEquipGate) AllowSeasonArchive(context.Context, string) error        { return entitlement.ErrProRequired }
func (denyEquipGate) AllowEquipment(context.Context, string) error            { return entitlement.ErrProRequired }
func (denyEquipGate) MaxParticipants(context.Context, string) (int, error)    { return 8, nil }
func (denyEquipGate) IsPro(context.Context, string) (bool, error)             { return false, nil }

type recPush struct {
	calls  int
	typ    string
	eqType string
	hours  string
}

func (p *recPush) SendToUser(_ context.Context, _, _, _ string, data map[string]string) error {
	p.calls++
	if data != nil {
		p.typ = data["type"]
		p.eqType = data["equipment_type"]
		p.hours = data["hours"]
	}
	return nil
}

type memStore struct {
	mu    sync.Mutex
	items []Item
	seq   int
}

func newMemStore() *memStore { return &memStore{} }

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

func (m *memStore) insertCreate(_ context.Context, userID string, in CreateInput, isPro bool) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	var hasActive bool
	for _, it := range m.items {
		if it.UserID == userID {
			count++
			if it.Type == in.Type && it.IsActive {
				hasActive = true
			}
		}
	}
	if !isPro && count >= freeEquipmentLimit {
		return Item{}, entitlement.ErrProRequired
	}
	return m.insertLocked(userID, in, !hasActive), nil
}

func (m *memStore) insert(_ context.Context, userID string, in CreateInput, isActive bool) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.insertLocked(userID, in, isActive), nil
}

func (m *memStore) insertLocked(userID string, in CreateInput, isActive bool) Item {
	now := time.Now()
	it := Item{
		ID: m.nextID(), UserID: userID, Type: in.Type, Name: in.Name,
		PurchaseDate: in.PurchaseDate, LifespanHours: in.LifespanHours,
		IsActive: isActive, CreatedAt: now, UpdatedAt: now,
	}
	m.items = append(m.items, it)
	return it
}

func (m *memStore) listForUser(_ context.Context, userID string, limit, offset int) ([]Item, int64, error) {
	var all []Item
	for _, it := range m.items {
		if it.UserID == userID {
			all = append(all, it)
		}
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
	for _, it := range m.items {
		if it.ID == id && it.UserID == userID {
			return it, nil
		}
	}
	return Item{}, ErrNotFound
}

func (m *memStore) update(_ context.Context, id, userID string, in UpdateInput, clearNotified bool) (Item, error) {
	for i, it := range m.items {
		if it.ID != id || it.UserID != userID {
			continue
		}
		if in.Name != nil {
			it.Name = *in.Name
		}
		if in.LifespanHours != nil {
			it.LifespanHours = *in.LifespanHours
		}
		if in.SetPurchaseDate {
			it.PurchaseDate = in.PurchaseDate
		}
		if clearNotified {
			it.NotifiedAt = nil
		}
		it.UpdatedAt = time.Now()
		m.items[i] = it
		return it, nil
	}
	return Item{}, ErrNotFound
}

func (m *memStore) delete(_ context.Context, id, userID string) error {
	for i, it := range m.items {
		if it.ID == id && it.UserID == userID {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *memStore) hasActive(_ context.Context, userID, typ string) (bool, error) {
	for _, it := range m.items {
		if it.UserID == userID && it.Type == typ && it.IsActive {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) activate(_ context.Context, id, userID string) (Item, error) {
	var target *Item
	var typ string
	for i := range m.items {
		if m.items[i].ID == id && m.items[i].UserID == userID {
			target = &m.items[i]
			typ = m.items[i].Type
			break
		}
	}
	if target == nil {
		return Item{}, ErrNotFound
	}
	for i := range m.items {
		if m.items[i].UserID == userID && m.items[i].Type == typ && m.items[i].IsActive && m.items[i].ID != id {
			m.items[i].IsActive = false
		}
	}
	target.IsActive = true
	target.NotifiedAt = nil
	target.UpdatedAt = time.Now()
	return *target, nil
}

func (m *memStore) reset(_ context.Context, id, userID string) (Item, error) {
	for i, it := range m.items {
		if it.ID == id && it.UserID == userID {
			it.AccumulatedSeconds = 0
			it.NotifiedAt = nil
			it.UpdatedAt = time.Now()
			m.items[i] = it
			return it, nil
		}
	}
	return Item{}, ErrNotFound
}

func (m *memStore) accrueActive(_ context.Context, userID string, seconds int) ([]Item, error) {
	var out []Item
	for i := range m.items {
		if m.items[i].UserID == userID && m.items[i].IsActive {
			m.items[i].AccumulatedSeconds += int64(seconds)
			m.items[i].UpdatedAt = time.Now()
			out = append(out, m.items[i])
		}
	}
	return out, nil
}

func (m *memStore) markNotified(_ context.Context, id string) (bool, error) {
	for i := range m.items {
		if m.items[i].ID == id && m.items[i].NotifiedAt == nil {
			now := time.Now()
			m.items[i].NotifiedAt = &now
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) listDueReminders(context.Context) ([]Item, error) {
	var out []Item
	for _, it := range m.items {
		if it.IsActive && it.NotifiedAt == nil && crossedThreshold(it.AccumulatedSeconds, it.LifespanHours) {
			out = append(out, it)
		}
	}
	return out, nil
}
