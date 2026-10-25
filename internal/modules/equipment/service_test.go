package equipment

import (
	"context"
	"errors"
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

func TestProGate_DeniesEveryMethod(t *testing.T) {
	svc := &Service{repo: newMemStore(), gate: denyEquipGate{}}
	ctx := context.Background()
	if _, err := svc.Create(ctx, "u1", CreateInput{Type: TypeString, Name: "X", LifespanHours: 25}); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("create=%v", err)
	}
	if _, _, err := svc.List(ctx, "u1", 20, 0); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("list=%v", err)
	}
	if _, err := svc.Get(ctx, "u1", "x"); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("get=%v", err)
	}
	if _, err := svc.Update(ctx, "u1", "x", UpdateInput{}); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("update=%v", err)
	}
	if err := svc.Delete(ctx, "u1", "x"); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("delete=%v", err)
	}
	if _, err := svc.Activate(ctx, "u1", "x"); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("activate=%v", err)
	}
	if _, err := svc.Reset(ctx, "u1", "x"); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("reset=%v", err)
	}
}

func TestUsageFrom_RemainingAndPercent(t *testing.T) {
	u := UsageFrom(Item{ID: "1", Type: TypeString, Name: "X", LifespanHours: 25, AccumulatedSeconds: 66240, IsActive: true})
	if u.AccumulatedHours != 18.4 || u.RemainingHours != 6.6 || u.UsagePercent != 74 {
		t.Fatalf("usage=%+v", u)
	}
}
