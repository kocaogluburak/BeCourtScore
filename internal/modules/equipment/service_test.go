package equipment

import (
	"context"
	"errors"
	"sync"
	"testing"

	"courtscore/internal/platform/entitlement"
)

func TestCreate_Validation(t *testing.T) {
	svc := &Service{repo: newMemStore()}
	ctx := context.Background()
	_, err := svc.Create(ctx, "u1", CreateInput{Type: "RACKET", Name: "X", LifespanHours: 25})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad type err=%v", err)
	}
	_, err = svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "  ", LifespanHours: 25})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank name err=%v", err)
	}
	_, err = svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "Luxilon", LifespanHours: 0})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero lifespan err=%v", err)
	}
	got, err := svc.Create(ctx, "u1", CreateInput{Type: "string", Name: " Luxilon ", LifespanHours: 25})
	if err != nil || got.Type != TypeString || got.Name != "Luxilon" || !got.IsActive {
		t.Fatalf("happy=%+v err=%v", got, err)
	}
}

func TestCreate_SecondOfTypeStaysInactive(t *testing.T) {
	svc := &Service{repo: newMemStore()}
	ctx := context.Background()
	first, _ := svc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Old", LifespanHours: 60})
	second, err := svc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "New", LifespanHours: 60})
	if err != nil || second.IsActive || !first.IsActive {
		t.Fatalf("first=%+v second=%+v err=%v", first, second, err)
	}
}

func TestOwnershipIsolation(t *testing.T) {
	svc := &Service{repo: newMemStore()}
	ctx := context.Background()
	a, _ := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "Mine", LifespanHours: 25})
	_, err := svc.Get(ctx, "u2", a.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign get err=%v", err)
	}
	if err := svc.Delete(ctx, "u2", a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign delete err=%v", err)
	}
}

func TestActivate_SwapsSingleActive(t *testing.T) {
	svc := &Service{repo: newMemStore()}
	ctx := context.Background()
	old, _ := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "Old", LifespanHours: 25})
	neu, _ := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "New", LifespanHours: 25})
	got, err := svc.Activate(ctx, "u1", neu.ID)
	if err != nil || !got.IsActive {
		t.Fatalf("activate=%+v err=%v", got, err)
	}
	prev, _ := svc.Get(ctx, "u1", old.ID)
	if prev.IsActive {
		t.Fatal("old still active")
	}
	cur, _ := svc.Get(ctx, "u1", neu.ID)
	if !cur.IsActive {
		t.Fatal("new not active")
	}
}

func TestFreeCreate_AllowsTwoBlocksThirdAndReEnablesAfterDelete(t *testing.T) {
	svc := &Service{repo: newMemStore(), gate: denyEquipGate{}}
	ctx := context.Background()
	first, err := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "A", LifespanHours: 25})
	if err != nil {
		t.Fatalf("first=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "B", LifespanHours: 60}); err != nil {
		t.Fatalf("second=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "C", LifespanHours: 25}); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("third=%v", err)
	}
	if err := svc.Delete(ctx, "u1", first.ID); err != nil {
		t.Fatalf("delete=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "C", LifespanHours: 25}); err != nil {
		t.Fatalf("after delete create=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "D", LifespanHours: 25}); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("fourth=%v", err)
	}
}

func TestFreeCreate_ConcurrentSerializesLimitAndActive(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store, gate: denyEquipGate{}}
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "S" + itoa(idx), LifespanHours: 25})
		}(i)
	}
	wg.Wait()

	var okCount, proErrCount int
	for _, err := range errs {
		if err == nil {
			okCount++
		} else if errors.Is(err, entitlement.ErrProRequired) {
			proErrCount++
		} else {
			t.Fatalf("unexpected err=%v", err)
		}
	}
	if okCount != 2 || proErrCount != 3 {
		t.Fatalf("ok=%d proErr=%d", okCount, proErrCount)
	}
	items, total, err := svc.List(ctx, "u1", 20, 0)
	if err != nil || total != 2 {
		t.Fatalf("list total=%d err=%v", total, err)
	}
	active := 0
	for _, it := range items {
		if it.IsActive {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("want exactly 1 active string, got %d", active)
	}
}

func TestProLapse_GrandfathersExistingItemsBlocksNewUntilBelowTwo(t *testing.T) {
	store := newMemStore()
	proSvc := &Service{repo: store, gate: proEquipGate{}}
	ctx := context.Background()
	e1, _ := proSvc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "S1", LifespanHours: 25})
	e2, _ := proSvc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "S2", LifespanHours: 25})
	e3, _ := proSvc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Sh1", LifespanHours: 60})

	// User lapses to free
	lapsedSvc := &Service{repo: store, gate: denyEquipGate{}}
	items, total, err := lapsedSvc.List(ctx, "u1", 20, 0)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("lapsed list=%d err=%v", total, err)
	}
	newName := "S2-edited"
	if _, err := lapsedSvc.Update(ctx, "u1", e2.ID, UpdateInput{Name: &newName}); err != nil {
		t.Fatalf("lapsed update=%v", err)
	}
	if _, err := lapsedSvc.Activate(ctx, "u1", e2.ID); err != nil {
		t.Fatalf("lapsed activate=%v", err)
	}
	lapsedSvc.AccrueMatchDuration(ctx, "u1", 3600)
	gotE2, err := lapsedSvc.Get(ctx, "u1", e2.ID)
	if err != nil || gotE2.AccumulatedHours != 1.0 {
		t.Fatalf("lapsed accrue=%+v err=%v", gotE2, err)
	}
	if _, err := lapsedSvc.Reset(ctx, "u1", e2.ID); err != nil {
		t.Fatalf("lapsed reset=%v", err)
	}
	if _, err := lapsedSvc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Sh2", LifespanHours: 60}); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("lapsed create at 3=%v", err)
	}
	// Delete 1 -> still 2 -> create still blocked
	_ = lapsedSvc.Delete(ctx, "u1", e1.ID)
	if _, err := lapsedSvc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Sh2", LifespanHours: 60}); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("lapsed create at 2=%v", err)
	}
	// Delete another -> count 1 -> create allowed
	_ = lapsedSvc.Delete(ctx, "u1", e3.ID)
	if _, err := lapsedSvc.Create(ctx, "u1", CreateInput{Type: TypeShoes, Name: "Sh2", LifespanHours: 60}); err != nil {
		t.Fatalf("lapsed create at 1=%v", err)
	}
}

func TestFreeUser_ListAndAccrueWithoutPro(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store, gate: denyEquipGate{}}
	ctx := context.Background()
	it, _ := store.insert(ctx, "u1", CreateInput{Type: TypeString, Name: "Luxilon", LifespanHours: 25}, true)
	if _, _, err := svc.List(ctx, "u1", 20, 0); err != nil {
		t.Fatalf("list=%v", err)
	}
	svc.AccrueMatchDuration(ctx, "u1", 3600)
	got, _ := store.getByID(ctx, it.ID, "u1")
	if got.AccumulatedSeconds != 3600 {
		t.Fatalf("accrue sec=%d", got.AccumulatedSeconds)
	}
}

func TestProUser_UnlimitedCreate(t *testing.T) {
	svc := &Service{repo: newMemStore(), gate: proEquipGate{}}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		name := "Gear" + itoa(i)
		if _, err := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: name, LifespanHours: 25}); err != nil {
			t.Fatalf("create %d=%v", i, err)
		}
	}
}

type proEquipGate struct{}

func (proEquipGate) AllowCloudSync(context.Context, string) error     { return nil }
func (proEquipGate) AllowStats(context.Context, string) error         { return nil }
func (proEquipGate) AllowHeadToHead(context.Context, string) error    { return nil }
func (proEquipGate) AllowAdvancedStats(context.Context, string) error { return nil }
func (proEquipGate) AllowPublicLive(context.Context, string) error    { return nil }
func (proEquipGate) AllowLargeTournament(context.Context, string, int) error {
	return nil
}
func (proEquipGate) AllowLargeLeague(context.Context, string, int) error { return nil }
func (proEquipGate) AllowSeasonArchive(context.Context, string) error    { return nil }
func (proEquipGate) AllowEquipment(context.Context, string) error        { return nil }
func (proEquipGate) MaxParticipants(context.Context, string) (int, error) {
	return 128, nil
}
func (proEquipGate) IsPro(context.Context, string) (bool, error) { return true, nil }

func TestUsageFrom_RemainingAndPercent(t *testing.T) {
	u := UsageFrom(Item{ID: "1", Type: TypeString, Name: "X", LifespanHours: 25, AccumulatedSeconds: 66240, IsActive: true})
	if u.AccumulatedHours != 18.4 || u.RemainingHours != 6.6 || u.UsagePercent != 74 {
		t.Fatalf("usage=%+v", u)
	}
}
