package equipment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
)

func newSvc() (*Service, *memStore) {
	st := newMemStore()
	return &Service{repo: st}, st
}

func stringIn(h int) CreateInput {
	return CreateInput{Sport: "TENNIS", CategoryID: catTString, BrandID: strp(brandLux), ModelID: strp(modelAlu), LifespanHours: intp(h)}
}

func shoesIn(name string) CreateInput {
	return CreateInput{Sport: "TENNIS", CategoryID: catTShoes, CustomBrandName: strp(name), LifespanHours: intp(60)}
}

func TestCreate_Validation(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	long := make([]byte, 81)
	for i := range long {
		long[i] = 'x'
	}
	manyAttrs := map[string]string{}
	for i := 0; i < 11; i++ {
		manyAttrs["k"+itoa(i)] = "v"
	}
	cases := map[string]CreateInput{
		"unknown sport":           {Sport: "GOLF", CategoryID: catTString, BrandID: strp(brandLux), LifespanHours: intp(15)},
		"missing category":        {Sport: "TENNIS", BrandID: strp(brandLux), LifespanHours: intp(15)},
		"unknown category":        {Sport: "TENNIS", CategoryID: "nope", BrandID: strp(brandLux), LifespanHours: intp(15)},
		"category other sport":    {Sport: "PADEL", CategoryID: catTString, BrandID: strp(brandLux), LifespanHours: intp(15)},
		"inactive category":       {Sport: "TENNIS", CategoryID: "cat-off", BrandID: strp(brandLux), LifespanHours: intp(15)},
		"no brand":                {Sport: "TENNIS", CategoryID: catTShoes, LifespanHours: intp(60)},
		"blank custom brand":      {Sport: "TENNIS", CategoryID: catTShoes, CustomBrandName: strp("   "), LifespanHours: intp(60)},
		"both brands":             {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), CustomBrandName: strp("X"), LifespanHours: intp(60)},
		"unknown brand":           {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp("b-none"), LifespanHours: intp(60)},
		"unknown model":           {Sport: "TENNIS", CategoryID: catTString, BrandID: strp(brandLux), ModelID: strp("m-none"), LifespanHours: intp(15)},
		"model wrong brand":       {Sport: "TENNIS", CategoryID: catTString, BrandID: strp(brandBab), ModelID: strp(modelAlu), LifespanHours: intp(15)},
		"model wrong category":    {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), ModelID: strp(modelAlu), LifespanHours: intp(60)},
		"model with custom brand": {Sport: "TENNIS", CategoryID: catTString, CustomBrandName: strp("Home"), ModelID: strp(modelAlu), LifespanHours: intp(15)},
		"HOURS needs hours":       {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux)},
		"HOURS zero hours":        {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), LifespanHours: intp(0)},
		"negative hours":          {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), LifespanHours: intp(-3)},
		"HOURS_OR_DAYS neither":   {Sport: "TENNIS", CategoryID: catTString, BrandID: strp(brandLux)},
		"bad purchase date":       {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), LifespanHours: intp(60), PurchaseDate: strp("10/01/2026")},
		"nickname too long":       {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), LifespanHours: intp(60), Nickname: strp(string(long))},
		"custom model too long":   {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), LifespanHours: intp(60), CustomModelName: strp(string(long))},
		"too many attributes":     {Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandLux), LifespanHours: intp(60), Attributes: manyAttrs},
	}
	for name, in := range cases {
		if _, err := svc.Create(ctx, "u1", in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err=%v want ErrInvalid", name, err)
		}
	}
}

func TestCreate_HappyPathShapesView(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	in := stringIn(15)
	in.LifespanDays = intp(90)
	in.PurchaseDate = strp("2026-01-10")
	in.Attributes = map[string]string{"tension_kg": " 24/23 "}
	got, err := svc.Create(ctx, "u1", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sport != "TENNIS" || got.Category.Code != "STRING" || !got.Category.IsCritical || !got.Category.SupportsRenew ||
		got.Category.ThresholdType != ThresholdHoursOrDays || got.Category.ParentCategoryID == nil {
		t.Fatalf("category=%+v", got.Category)
	}
	if got.Brand == nil || got.Brand.Name != "Luxilon" || got.Brand.AccentColorHex != "#212121" || got.Model == nil || got.Model.Name != "ALU Power" {
		t.Fatalf("brand/model=%+v %+v", got.Brand, got.Model)
	}
	if got.DisplayName != "Luxilon ALU Power" || got.Name != got.DisplayName {
		t.Fatalf("display=%q name=%q", got.DisplayName, got.Name)
	}
	if got.Type != TypeString {
		t.Fatalf("legacy type=%q", got.Type)
	}
	if *got.LifespanHours != 15 || *got.LifespanDays != 90 || !got.IsActive || got.RetiredAt != nil || got.RenewalsThisYear != 0 {
		t.Fatalf("view=%+v", got)
	}
	if got.Attributes["tension_kg"] != "24/23" || got.PurchaseDate == nil || *got.PurchaseDate != "2026-01-10" {
		t.Fatalf("attrs/date=%+v", got)
	}
}

func TestCreate_LifespanRulesByThreshold(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()

	// NONE: lifespans are forced to nil, whatever the client sends.
	racket, err := svc.Create(ctx, "u1", CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab), ModelID: strp(modelPure), LifespanHours: intp(50), LifespanDays: intp(9)})
	if err != nil || racket.LifespanHours != nil || racket.LifespanDays != nil || racket.UsagePercent != 0 {
		t.Fatalf("NONE: %+v err=%v", racket, err)
	}
	// HOURS: days are ignored.
	shoes, err := svc.Create(ctx, "u2", CreateInput{Sport: "TENNIS", CategoryID: catTShoes, BrandID: strp(brandBab), LifespanHours: intp(60), LifespanDays: intp(9)})
	if err != nil || *shoes.LifespanHours != 60 || shoes.LifespanDays != nil {
		t.Fatalf("HOURS: %+v err=%v", shoes, err)
	}
	// HOURS_OR_DAYS: days alone is enough.
	str, err := svc.Create(ctx, "u3", CreateInput{Sport: "TENNIS", CategoryID: catTString, BrandID: strp(brandLux), LifespanDays: intp(90)})
	if err != nil || str.LifespanHours != nil || *str.LifespanDays != 90 {
		t.Fatalf("HOURS_OR_DAYS days only: %+v err=%v", str, err)
	}
}

func TestCreate_NonTennisHasNoLegacyType(t *testing.T) {
	svc, _ := newSvc()
	got, err := svc.Create(context.Background(), "u1", CreateInput{Sport: "padel", CategoryID: catPala, CustomBrandName: strp("Bullpadel"), CustomModelName: strp("Vertex 04"), LifespanHours: intp(90)})
	if err != nil || got.Type != "" || got.DisplayName != "Bullpadel Vertex 04" || got.Sport != "PADEL" {
		t.Fatalf("padel=%+v err=%v", got, err)
	}
}

func TestCreate_SecondOfCategoryStaysInactive(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	first, _ := svc.Create(ctx, "u1", shoesIn("Old"))
	second, err := svc.Create(ctx, "u1", shoesIn("New"))
	if err != nil || second.IsActive || !first.IsActive {
		t.Fatalf("first=%+v second=%+v err=%v", first, second, err)
	}
	// Another category starts its own active slot.
	other, err := svc.Create(ctx, "u1", stringIn(15))
	if err != nil || !other.IsActive {
		t.Fatalf("other=%+v err=%v", other, err)
	}
}

func TestCreate_ParentItemValidation(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	racket, _ := svc.Create(ctx, "u1", CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab), LifespanHours: intp(1)})
	shoes, _ := svc.Create(ctx, "u1", shoesIn("Asics"))
	foreignRacket, _ := svc.Create(ctx, "u2", CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab)})

	in := stringIn(15)
	in.ParentItemID = &racket.ID
	got, err := svc.Create(ctx, "u1", in)
	if err != nil || got.ParentItemID == nil || *got.ParentItemID != racket.ID {
		t.Fatalf("linked=%+v err=%v", got, err)
	}
	for name, parent := range map[string]string{"wrong category": shoes.ID, "foreign": foreignRacket.ID, "missing": "nope"} {
		in := stringIn(15)
		in.ParentItemID = strp(parent)
		if _, err := svc.Create(ctx, "u1", in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	// A category without a parent category cannot take a parent item.
	bad := shoesIn("X")
	bad.ParentItemID = &racket.ID
	if _, err := svc.Create(ctx, "u1", bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("no parent category: err=%v", err)
	}
}

func TestOwnershipIsolation(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	a, _ := svc.Create(ctx, "u1", stringIn(25))
	if _, err := svc.Get(ctx, "u2", a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign get err=%v", err)
	}
	if err := svc.Delete(ctx, "u2", a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign delete err=%v", err)
	}
	if _, err := svc.Archive(ctx, "u2", a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign archive err=%v", err)
	}
	if _, err := svc.Renew(ctx, "u2", a.ID, RenewInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign renew err=%v", err)
	}
}

func TestActivate_SwapsSingleActivePerCategory(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	old, _ := svc.Create(ctx, "u1", shoesIn("Old"))
	neu, _ := svc.Create(ctx, "u1", shoesIn("New"))
	str, _ := svc.Create(ctx, "u1", stringIn(25))

	got, err := svc.Activate(ctx, "u1", neu.ID)
	if err != nil || !got.IsActive {
		t.Fatalf("activate=%+v err=%v", got, err)
	}
	if prev, _ := svc.Get(ctx, "u1", old.ID); prev.IsActive {
		t.Fatal("old shoes still active")
	}
	if cur, _ := svc.Get(ctx, "u1", neu.ID); !cur.IsActive {
		t.Fatal("new shoes not active")
	}
	if s, _ := svc.Get(ctx, "u1", str.ID); !s.IsActive {
		t.Fatal("activating shoes must not touch the string slot")
	}
}

func TestActivate_ArchivedIsRejected(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	it, _ := svc.Create(ctx, "u1", shoesIn("Old"))
	if _, err := svc.Archive(ctx, "u1", it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Activate(ctx, "u1", it.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("activate archived err=%v", err)
	}
}

func TestFreeCreate_AllowsTwoBlocksThirdAndReEnablesAfterDelete(t *testing.T) {
	svc, _ := newSvc()
	svc.gate = denyEquipGate{}
	ctx := context.Background()
	first, err := svc.Create(ctx, "u1", stringIn(25))
	if err != nil {
		t.Fatalf("first=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("B")); err != nil {
		t.Fatalf("second=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("C")); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("third=%v", err)
	}
	if err := svc.Delete(ctx, "u1", first.ID); err != nil {
		t.Fatalf("delete=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("C")); err != nil {
		t.Fatalf("after delete create=%v", err)
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("D")); !errors.Is(err, ErrProRequiredAlias) {
		t.Fatalf("fourth=%v", err)
	}
}

// ErrProRequiredAlias keeps the assertion above readable.
var ErrProRequiredAlias = entitlement.ErrProRequired

func TestFreeLimit_ArchivedRowsDoNotCount(t *testing.T) {
	svc, _ := newSvc()
	svc.gate = denyEquipGate{}
	ctx := context.Background()
	a, _ := svc.Create(ctx, "u1", stringIn(25))
	if _, err := svc.Create(ctx, "u1", shoesIn("B")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("C")); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("limit not enforced: %v", err)
	}
	if _, err := svc.Archive(ctx, "u1", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("C")); err != nil {
		t.Fatalf("archived row must free a slot: %v", err)
	}
}

func TestFreeLimit_RenewIsNetZero(t *testing.T) {
	svc, _ := newSvc()
	svc.gate = denyEquipGate{}
	ctx := context.Background()
	str, _ := svc.Create(ctx, "u1", stringIn(25))
	if _, err := svc.Create(ctx, "u1", shoesIn("B")); err != nil {
		t.Fatal(err)
	}
	cur := str
	for i := 0; i < 3; i++ {
		next, err := svc.Renew(ctx, "u1", cur.ID, RenewInput{})
		if err != nil {
			t.Fatalf("renew %d at the free limit: %v", i, err)
		}
		cur = next
	}
	if _, err := svc.Create(ctx, "u1", shoesIn("C")); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("limit must still hold after renewals: %v", err)
	}
	items, total, _ := svc.List(ctx, "u1", ListFilter{}, 20, 0)
	if total != 2 || len(items) != 2 {
		t.Fatalf("live rows=%d", total)
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
			_, errs[idx] = svc.Create(ctx, "u1", CreateInput{Sport: "TENNIS", CategoryID: catTShoes, CustomBrandName: strp("S" + itoa(idx)), LifespanHours: intp(25)})
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
	items, total, err := svc.List(ctx, "u1", ListFilter{}, 20, 0)
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
		t.Fatalf("want exactly 1 active in the category, got %d", active)
	}
}

func TestProLapse_GrandfathersExistingItemsBlocksNewUntilBelowTwo(t *testing.T) {
	store := newMemStore()
	proSvc := &Service{repo: store, gate: proEquipGate{}}
	ctx := context.Background()
	e1, _ := proSvc.Create(ctx, "u1", stringIn(25))
	e2, _ := proSvc.Create(ctx, "u1", stringIn(25))
	e3, _ := proSvc.Create(ctx, "u1", shoesIn("Sh1"))

	lapsedSvc := &Service{repo: store, gate: denyEquipGate{}}
	items, total, err := lapsedSvc.List(ctx, "u1", ListFilter{}, 20, 0)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("lapsed list=%d err=%v", total, err)
	}
	nick := "S2-edited"
	if _, err := lapsedSvc.Update(ctx, "u1", e2.ID, UpdateInput{Nickname: &nick}); err != nil {
		t.Fatalf("lapsed update=%v", err)
	}
	if _, err := lapsedSvc.Activate(ctx, "u1", e2.ID); err != nil {
		t.Fatalf("lapsed activate=%v", err)
	}
	lapsedSvc.AccrueMatchDuration(ctx, "u1", "TENNIS", 3600)
	gotE2, err := lapsedSvc.Get(ctx, "u1", e2.ID)
	if err != nil || gotE2.AccumulatedHours != 1.0 {
		t.Fatalf("lapsed accrue=%+v err=%v", gotE2, err)
	}
	if _, err := lapsedSvc.Reset(ctx, "u1", e2.ID); err != nil {
		t.Fatalf("lapsed reset=%v", err)
	}
	if _, err := lapsedSvc.Create(ctx, "u1", shoesIn("Sh2")); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("lapsed create at 3=%v", err)
	}
	_ = lapsedSvc.Delete(ctx, "u1", e1.ID)
	if _, err := lapsedSvc.Create(ctx, "u1", shoesIn("Sh2")); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("lapsed create at 2=%v", err)
	}
	_ = lapsedSvc.Delete(ctx, "u1", e3.ID)
	if _, err := lapsedSvc.Create(ctx, "u1", shoesIn("Sh2")); err != nil {
		t.Fatalf("lapsed create at 1=%v", err)
	}
}

func TestFreeUser_ListAndAccrueWithoutPro(t *testing.T) {
	store := newMemStore()
	svc := &Service{repo: store, gate: denyEquipGate{}}
	ctx := context.Background()
	it := store.seed("u1", catTString, 25, true)
	if _, _, err := svc.List(ctx, "u1", ListFilter{}, 20, 0); err != nil {
		t.Fatalf("list=%v", err)
	}
	svc.AccrueMatchDuration(ctx, "u1", "TENNIS", 3600)
	if got := store.get(it.ID); got.AccumulatedSeconds != 3600 {
		t.Fatalf("accrue sec=%d", got.AccumulatedSeconds)
	}
}

func TestProUser_UnlimitedCreate(t *testing.T) {
	svc, _ := newSvc()
	svc.gate = proEquipGate{}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, "u1", shoesIn("Gear"+itoa(i))); err != nil {
			t.Fatalf("create %d=%v", i, err)
		}
	}
}

func TestArchive_RetiresWithoutReplacementAndFreesSlot(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	a, _ := svc.Create(ctx, "u1", shoesIn("A"))
	got, err := svc.Archive(ctx, "u1", a.ID)
	if err != nil || got.RetiredAt == nil || got.IsActive {
		t.Fatalf("archive=%+v err=%v", got, err)
	}
	if len(st.items) != 1 {
		t.Fatalf("archive must not open a new row, rows=%d", len(st.items))
	}
	if _, err := svc.Archive(ctx, "u1", a.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("second archive err=%v", err)
	}
	// The category slot is free again: the next one becomes active.
	next, _ := svc.Create(ctx, "u1", shoesIn("B"))
	if !next.IsActive {
		t.Fatal("new item in a freed category must be active")
	}
}

func TestRenew_ArchivesOldAndOpensNewGenerationInSameLineage(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	racket, _ := svc.Create(ctx, "u1", CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab), ModelID: strp(modelPure)})
	in := stringIn(15)
	in.LifespanDays = intp(90)
	in.ParentItemID = &racket.ID
	in.Attributes = map[string]string{"tension_kg": "24/23"}
	in.PurchaseDate = strp("2025-01-01")
	in.Nickname = strp("Match string")
	old, err := svc.Create(ctx, "u1", in)
	if err != nil {
		t.Fatal(err)
	}
	svc.AccrueMatchDuration(ctx, "u1", "TENNIS", 7200)

	neu, err := svc.Renew(ctx, "u1", old.ID, RenewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if neu.ID == old.ID || !neu.IsActive || neu.RetiredAt != nil {
		t.Fatalf("new=%+v", neu)
	}
	// Empty renew copies brand/model/attributes/lifespans/parent from the old row.
	if neu.Brand == nil || neu.Brand.ID != brandLux || neu.Model == nil || neu.Model.ID != modelAlu ||
		neu.Attributes["tension_kg"] != "24/23" || *neu.LifespanHours != 15 || *neu.LifespanDays != 90 ||
		neu.ParentItemID == nil || *neu.ParentItemID != racket.ID {
		t.Fatalf("copy=%+v", neu)
	}
	// ... but starts fresh: no usage, no purchase date, no nickname, no notification.
	if neu.AccumulatedHours != 0 || neu.PurchaseDate != nil || neu.Nickname != nil || neu.NotifiedAt != nil {
		t.Fatalf("fresh=%+v", neu)
	}
	oldRow := st.get(old.ID)
	if oldRow.RetiredAt == nil || oldRow.IsActive || oldRow.AccumulatedSeconds != 7200 {
		t.Fatalf("old=%+v", oldRow)
	}
	if oldRow.LineageID != st.get(neu.ID).LineageID {
		t.Fatal("lineage_id must stay constant across generations")
	}
	if neu.RenewalsThisYear != 1 {
		t.Fatalf("renewals_this_year=%d want 1", neu.RenewalsThisYear)
	}
	third, err := svc.Renew(ctx, "u1", neu.ID, RenewInput{})
	if err != nil || third.RenewalsThisYear != 2 {
		t.Fatalf("third=%+v err=%v", third, err)
	}
}

func TestRenew_NewValuesOverrideAndBrandResetsModel(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	old, _ := svc.Create(ctx, "u1", stringIn(15))

	// Attributes only: brand/model kept.
	a, err := svc.Renew(ctx, "u1", old.ID, RenewInput{Attributes: map[string]string{"tension_kg": "25/24"}})
	if err != nil || a.Attributes["tension_kg"] != "25/24" || a.Brand.ID != brandLux || a.Model == nil {
		t.Fatalf("attrs only=%+v err=%v", a, err)
	}
	// New custom brand: the old catalog model is not carried over.
	b, err := svc.Renew(ctx, "u1", a.ID, RenewInput{CustomBrandName: strp("Gut Co")})
	if err != nil || b.Brand != nil || b.CustomBrandName == nil || *b.CustomBrandName != "Gut Co" || b.Model != nil || b.DisplayName != "Gut Co" {
		t.Fatalf("custom brand=%+v err=%v", b, err)
	}
	// Both brand and model given: validated against the catalog.
	if _, err := svc.Renew(ctx, "u1", b.ID, RenewInput{BrandID: strp(brandBab), ModelID: strp(modelAlu)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched model err=%v", err)
	}
	c, err := svc.Renew(ctx, "u1", b.ID, RenewInput{BrandID: strp(brandLux), ModelID: strp(modelAlu)})
	if err != nil || c.Model == nil || c.Model.ID != modelAlu {
		t.Fatalf("catalog pick=%+v err=%v", c, err)
	}
}

func TestRenew_Guards(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	shoes, _ := svc.Create(ctx, "u1", shoesIn("A"))
	if _, err := svc.Renew(ctx, "u1", shoes.ID, RenewInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("renew on a non-renewable category err=%v", err)
	}
	str, _ := svc.Create(ctx, "u1", stringIn(15))
	if _, err := svc.Archive(ctx, "u1", str.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Renew(ctx, "u1", str.ID, RenewInput{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("renew archived err=%v", err)
	}
	if _, err := svc.Renew(ctx, "u1", "nope", RenewInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("renew missing err=%v", err)
	}
}

func TestRenew_SpareKeepsInactiveSlot(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	active, _ := svc.Create(ctx, "u1", stringIn(15))
	spare, _ := svc.Create(ctx, "u1", stringIn(15))
	renewed, err := svc.Renew(ctx, "u1", spare.ID, RenewInput{})
	if err != nil || renewed.IsActive {
		t.Fatalf("a renewed spare must stay inactive: %+v err=%v", renewed, err)
	}
	if cur, _ := svc.Get(ctx, "u1", active.ID); !cur.IsActive {
		t.Fatal("active string was disturbed")
	}
}

func TestList_ViewAndFilters(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	s1, _ := svc.Create(ctx, "u1", stringIn(15))
	sh, _ := svc.Create(ctx, "u1", shoesIn("A"))
	pala, _ := svc.Create(ctx, "u1", CreateInput{Sport: "PADEL", CategoryID: catPala, CustomBrandName: strp("Nox"), LifespanHours: intp(90)})
	renewed, _ := svc.Renew(ctx, "u1", s1.ID, RenewInput{})
	_, _ = svc.Create(ctx, "u2", shoesIn("other user"))

	active, total, err := svc.List(ctx, "u1", ListFilter{}, 20, 0)
	if err != nil || total != 3 || len(active) != 3 {
		t.Fatalf("default view=active total=%d err=%v", total, err)
	}
	for _, it := range active {
		if it.RetiredAt != nil || it.ID == s1.ID {
			t.Fatalf("active view leaked a retired row: %+v", it)
		}
	}
	arch, total, _ := svc.List(ctx, "u1", ListFilter{View: "ARCHIVE"}, 20, 0)
	if total != 1 || len(arch) != 1 || arch[0].ID != s1.ID || arch[0].RetiredAt == nil {
		t.Fatalf("archive=%+v", arch)
	}
	bySport, total, _ := svc.List(ctx, "u1", ListFilter{Sport: "padel"}, 20, 0)
	if total != 1 || bySport[0].ID != pala.ID {
		t.Fatalf("sport filter=%+v", bySport)
	}
	byCat, total, _ := svc.List(ctx, "u1", ListFilter{CategoryID: catTShoes}, 20, 0)
	if total != 1 || byCat[0].ID != sh.ID {
		t.Fatalf("category filter=%+v", byCat)
	}
	_ = renewed
	paged, total, _ := svc.List(ctx, "u1", ListFilter{}, 2, 2)
	if total != 3 || len(paged) != 1 {
		t.Fatalf("paging total=%d len=%d", total, len(paged))
	}
	if _, _, err := svc.List(ctx, "u1", ListFilter{View: "all"}, 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad view err=%v", err)
	}
	if _, _, err := svc.List(ctx, "u1", ListFilter{Sport: "GOLF"}, 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad sport err=%v", err)
	}
}

func TestUpdate_NicknameAttributesParentAndPurchase(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	racket, _ := svc.Create(ctx, "u1", CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab)})
	str, _ := svc.Create(ctx, "u1", stringIn(15))

	nick := "  Practice  "
	got, err := svc.Update(ctx, "u1", str.ID, UpdateInput{
		Nickname: &nick, Attributes: map[string]string{"tension_kg": "23"},
		SetParent: true, ParentItemID: &racket.ID, SetPurchaseDate: true, PurchaseDate: strp("2026-02-02"),
	})
	if err != nil || got.DisplayName != "Practice" || got.Attributes["tension_kg"] != "23" ||
		got.ParentItemID == nil || *got.ParentItemID != racket.ID || *got.PurchaseDate != "2026-02-02" {
		t.Fatalf("update=%+v err=%v", got, err)
	}
	blank := "  "
	got, _ = svc.Update(ctx, "u1", str.ID, UpdateInput{Nickname: &blank, SetParent: true, ParentItemID: strp("")})
	if got.Nickname != nil || got.DisplayName != "Luxilon ALU Power" || got.ParentItemID != nil {
		t.Fatalf("clear=%+v", got)
	}
	if _, err := svc.Update(ctx, "u1", str.ID, UpdateInput{SetPurchaseDate: true, PurchaseDate: strp("bad")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad date err=%v", err)
	}
	if _, err := svc.Update(ctx, "u1", str.ID, UpdateInput{SetParent: true, ParentItemID: &str.ID}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self parent err=%v", err)
	}
	if _, err := svc.Update(ctx, "u2", str.ID, UpdateInput{Nickname: &nick}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign update err=%v", err)
	}
}

func TestUpdate_BrandAndModelEditing(t *testing.T) {
	svc, _ := newSvc()
	ctx := context.Background()
	str, _ := svc.Create(ctx, "u1", stringIn(15))

	// Brand swap without a model drops the catalog model.
	got, err := svc.Update(ctx, "u1", str.ID, UpdateInput{SetBrand: true, CustomBrandName: strp("Home Gut")})
	if err != nil || got.Brand != nil || got.Model != nil || *got.CustomBrandName != "Home Gut" {
		t.Fatalf("brand swap=%+v err=%v", got, err)
	}
	// Both brand fields set -> XOR violation.
	if _, err := svc.Update(ctx, "u1", str.ID, UpdateInput{SetBrand: true, BrandID: strp(brandLux), CustomBrandName: strp("x")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("xor err=%v", err)
	}
	got, err = svc.Update(ctx, "u1", str.ID, UpdateInput{SetBrand: true, BrandID: strp(brandLux), SetModel: true, ModelID: strp(modelAlu)})
	if err != nil || got.Model == nil || got.Model.ID != modelAlu || got.CustomBrandName != nil {
		t.Fatalf("catalog pick=%+v err=%v", got, err)
	}
	got, err = svc.Update(ctx, "u1", str.ID, UpdateInput{SetModel: true, CustomModelName: strp("Special")})
	if err != nil || got.Model != nil || *got.CustomModelName != "Special" || got.Brand == nil {
		t.Fatalf("model edit=%+v err=%v", got, err)
	}
}

func TestUpdate_LifespanRulesAndNotifiedReset(t *testing.T) {
	svc, st := newSvc()
	ctx := context.Background()
	shoes, _ := svc.Create(ctx, "u1", shoesIn("A"))
	row := st.get(shoes.ID)
	now := time.Now()
	st.mu.Lock()
	st.items[0].NotifiedAt = &now
	st.mu.Unlock()
	_ = row

	if _, err := svc.Update(ctx, "u1", shoes.ID, UpdateInput{LifespanHours: intp(-1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative err=%v", err)
	}
	if got, _ := svc.Update(ctx, "u1", shoes.ID, UpdateInput{LifespanHours: intp(30)}); got.NotifiedAt == nil {
		t.Fatal("lowering the lifespan must not clear notified_at")
	}
	got, err := svc.Update(ctx, "u1", shoes.ID, UpdateInput{LifespanHours: intp(90), LifespanDays: intp(10)})
	if err != nil || got.NotifiedAt != nil || *got.LifespanHours != 90 || got.LifespanDays != nil {
		t.Fatalf("raise=%+v err=%v", got, err)
	}
	// NONE categories stay lifespan-less.
	racket, _ := svc.Create(ctx, "u1", CreateInput{Sport: "TENNIS", CategoryID: catTRacket, BrandID: strp(brandBab)})
	got, err = svc.Update(ctx, "u1", racket.ID, UpdateInput{LifespanHours: intp(40)})
	if err != nil || got.LifespanHours != nil {
		t.Fatalf("NONE update=%+v err=%v", got, err)
	}
}

func TestDisplayName_Chain(t *testing.T) {
	n := func(s string) *string { return &s }
	cases := []struct {
		name string
		it   Item
		want string
	}{
		{"nickname wins", Item{Nickname: n("Mine"), BrandName: n("Luxilon"), ModelName: n("ALU")}, "Mine"},
		{"brand+model", Item{BrandName: n("Luxilon"), ModelName: n("ALU Power")}, "Luxilon ALU Power"},
		{"custom brand + custom model", Item{CustomBrandName: n("Home"), CustomModelName: n("Gut 1")}, "Home Gut 1"},
		{"brand only trimmed", Item{BrandName: n("Luxilon")}, "Luxilon"},
		{"legacy name fallback", Item{LegacyName: n("Luxilon ALU 2024")}, "Luxilon ALU 2024"},
		{"dash fallback", Item{}, "—"},
	}
	for _, c := range cases {
		if got := displayName(c.it); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestUsageFrom_RemainingPercentAndLegacyType(t *testing.T) {
	h := 25
	u := UsageFrom(Item{ID: "1", Sport: "TENNIS", Category: Category{Code: "STRING"}, LifespanHours: &h, AccumulatedSeconds: 66240, IsActive: true})
	if u.AccumulatedHours != 18.4 || u.RemainingHours != 6.6 || u.UsagePercent != 74 || u.Type != TypeString {
		t.Fatalf("usage=%+v", u)
	}
	none := UsageFrom(Item{ID: "2", Sport: "TENNIS", Category: Category{Code: "RACKET"}, AccumulatedSeconds: 7200})
	if none.LifespanHours != nil || none.RemainingHours != 0 || none.UsagePercent != 0 || none.Type != "" || none.AccumulatedHours != 2 {
		t.Fatalf("no lifespan=%+v", none)
	}
	if UsageFrom(Item{Sport: "PADEL", Category: Category{Code: "SHOES"}}).Type != "" {
		t.Fatal("non-tennis rows must omit type")
	}
	if UsageFrom(Item{Sport: "TENNIS", Category: Category{Code: "SHOES"}}).Type != TypeShoes {
		t.Fatal("tennis shoes must expose legacy type")
	}
	if UsageFrom(Item{}).Attributes == nil {
		t.Fatal("attributes must serialize as an object")
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
