package equipment

import (
	"context"
	"testing"

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

func TestAccrue_ActiveOnlyAcrossCategories(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store}
	ctx := context.Background()
	str := store.seed("u1", catTString, 25, true)
	spare := store.seed("u1", catTString, 25, false)
	shoes := store.seed("u1", catTShoes, 60, true)
	retired := store.seed("u1", catTOver, 6, false)
	other := store.seed("u2", catTShoes, 60, true)
	if _, err := store.archive(ctx, retired.ID, "u1"); err != nil {
		t.Fatal(err)
	}

	svc.AccrueMatchDuration(ctx, "u1", 3600)

	if store.get(str.ID).AccumulatedSeconds != 3600 || store.get(shoes.ID).AccumulatedSeconds != 3600 {
		t.Fatal("active rows of every category must accrue")
	}
	if store.get(spare.ID).AccumulatedSeconds != 0 || store.get(retired.ID).AccumulatedSeconds != 0 || store.get(other.ID).AccumulatedSeconds != 0 {
		t.Fatal("inactive / retired / foreign rows must not accrue")
	}
}

func TestAccrue_IgnoresNilZeroNegative(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store}
	ctx := context.Background()
	it := store.seed("u1", catTString, 25, true)

	svc.AccrueMatchDuration(ctx, "u1", 0)
	svc.AccrueMatchDuration(ctx, "u1", -10)
	svc.AccrueMatchDuration(ctx, "", 100)
	if got := store.get(it.ID); got.AccumulatedSeconds != 0 {
		t.Fatalf("zero/negative should no-op, sec=%d", got.AccumulatedSeconds)
	}

	svc.AccrueMatchDuration(ctx, "u1", 3600)
	if got := store.get(it.ID); got.AccumulatedSeconds != 3600 {
		t.Fatalf("positive accrue sec=%d", got.AccumulatedSeconds)
	}

	svc.AccrueMatchDuration(ctx, "u1", 0)
	if got := store.get(it.ID); got.AccumulatedSeconds != 3600 {
		t.Fatalf("zero after accrue sec=%d", got.AccumulatedSeconds)
	}
}

func TestThreshold_FiresOnceAndIdempotent(t *testing.T) {
	store := newMemStore()
	push := &recPush{}
	svc := &Service{repo: store, push: push}
	ctx := context.Background()
	it := store.seed("u1", catTString, 1, true)

	svc.AccrueMatchDuration(ctx, "u1", 1800)
	if push.calls != 0 {
		t.Fatalf("under threshold fired=%d", push.calls)
	}

	svc.AccrueMatchDuration(ctx, "u1", 1800)
	if push.calls != 1 || push.typ != "equipment.reminder" || push.eqType != TypeString || push.sport != "TENNIS" || push.hours != "1" {
		t.Fatalf("first cross: %+v", push)
	}
	if store.get(it.ID).NotifiedAt == nil {
		t.Fatal("notified_at not stamped")
	}

	svc.AccrueMatchDuration(ctx, "u1", 1800)
	if push.calls != 1 {
		t.Fatalf("re-fired: %d", push.calls)
	}
}

func TestThreshold_NoneCategoryNeverReminds(t *testing.T) {
	store := newMemStore()
	push := &recPush{}
	svc := &Service{repo: store, push: push}
	ctx := context.Background()
	racket := store.seed("u1", catTRacket, 0, true)

	svc.AccrueMatchDuration(ctx, "u1", 3600*500)
	if push.calls != 0 || store.get(racket.ID).AccumulatedSeconds != 3600*500 {
		t.Fatalf("racket must accrue silently: calls=%d", push.calls)
	}
	if err := svc.SweepReminders(ctx); err != nil || push.calls != 0 {
		t.Fatalf("sweep must skip NONE: calls=%d err=%v", push.calls, err)
	}
}

func TestThreshold_RefiresAfterResetAndLifespanRaise(t *testing.T) {
	store := newMemStore()
	push := &recPush{}
	svc := &Service{repo: store, push: push}
	ctx := context.Background()
	it := store.seed("u1", catTShoes, 1, true)

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
	if store.get(it.ID).NotifiedAt != nil {
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
	it := store.seed("u1", catTString, 1, true)
	store.mu.Lock()
	store.items[0].AccumulatedSeconds = 3600
	store.mu.Unlock()
	_ = it

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

func TestReminderCopy_PerCategory(t *testing.T) {
	for _, code := range []string{"STRING", "SHOES", "RUBBER_FH", "RUBBER_BH", "OVERGRIP", "PALA", "PADDLE"} {
		title, body := reminderCopy(code, "15")
		if title == "" || body == "" {
			t.Errorf("%s: empty copy", code)
		}
	}
}

type denyEquipGate struct{}

func (denyEquipGate) AllowCloudSync(context.Context, string) error { return entitlement.ErrProRequired }
func (denyEquipGate) AllowStats(context.Context, string) error     { return entitlement.ErrProRequired }
func (denyEquipGate) AllowHeadToHead(context.Context, string) error {
	return entitlement.ErrProRequired
}
func (denyEquipGate) AllowAdvancedStats(context.Context, string) error {
	return entitlement.ErrProRequired
}
func (denyEquipGate) AllowPublicLive(context.Context, string) error {
	return entitlement.ErrProRequired
}
func (denyEquipGate) AllowLargeTournament(context.Context, string, int) error {
	return entitlement.ErrProRequired
}
func (denyEquipGate) AllowLargeLeague(context.Context, string, int) error {
	return entitlement.ErrProRequired
}
func (denyEquipGate) AllowSeasonArchive(context.Context, string) error {
	return entitlement.ErrProRequired
}
func (denyEquipGate) AllowEquipment(context.Context, string) error         { return entitlement.ErrProRequired }
func (denyEquipGate) MaxParticipants(context.Context, string) (int, error) { return 8, nil }
func (denyEquipGate) IsPro(context.Context, string) (bool, error)          { return false, nil }

type recPush struct {
	calls  int
	typ    string
	eqType string
	sport  string
	hours  string
}

func (p *recPush) SendToUser(_ context.Context, _, _, _ string, data map[string]string) error {
	p.calls++
	if data != nil {
		p.typ = data["type"]
		p.eqType = data["equipment_type"]
		p.sport = data["sport"]
		p.hours = data["hours"]
	}
	return nil
}
