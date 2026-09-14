package billing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
)

type fakeStore struct {
	subs     []Sub
	count    int
	countErr error
}

func (f *fakeStore) upsert(_ context.Context, s Sub) error {
	for i, existing := range f.subs {
		if existing.Store == s.Store && existing.OriginalTxnID == s.OriginalTxnID {
			f.subs[i] = s
			return nil
		}
	}
	f.subs = append(f.subs, s)
	return nil
}

func (f *fakeStore) activeForUser(_ context.Context, userID string, now time.Time) (Sub, bool, error) {
	for _, s := range f.subs {
		if s.UserID != userID {
			continue
		}
		if s.Status != "active" && s.Status != "grace" {
			continue
		}
		if s.ExpiresAt.After(now) || (s.GraceUntil != nil && s.GraceUntil.After(now)) {
			return s, true, nil
		}
	}
	return Sub{}, false, nil
}

func (f *fakeStore) byOriginalTxn(_ context.Context, store, txn string) (Sub, bool, error) {
	for _, s := range f.subs {
		if s.Store == store && s.OriginalTxnID == txn {
			return s, true, nil
		}
	}
	return Sub{}, false, nil
}

func (f *fakeStore) countFinishedMatches(context.Context, string) (int, error) {
	return f.count, f.countErr
}

func newBilling(store *fakeStore) *Service {
	return &Service{repo: store, now: func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }}
}

func TestLoad_FreeUnderCap(t *testing.T) {
	svc := newBilling(&fakeStore{count: 3})
	snap, err := svc.Load(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.IsPro || snap.FinishedMatchCount != 3 {
		t.Fatalf("%+v", snap)
	}
	if snap.MaxTournamentParticipants != entitlement.ProMaxParticipants {
		t.Fatalf("under cap should allow large draws, got %d", snap.MaxTournamentParticipants)
	}
}

func TestLoad_OverCapWithoutPro(t *testing.T) {
	svc := newBilling(&fakeStore{count: 10})
	snap, err := svc.Load(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.IsPro || entitlement.WithinFreeQuota(snap) {
		t.Fatalf("should be locked: %+v", snap)
	}
	if snap.MaxTournamentParticipants != entitlement.FreeMaxParticipants {
		t.Fatalf("got max %d", snap.MaxTournamentParticipants)
	}
	if err := svc.AllowCloudSync(context.Background(), "u1"); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
	if err := svc.AllowLargeTournament(context.Background(), "u1", 8); err != nil {
		t.Fatalf("8-draw should stay free: %v", err)
	}
	if err := svc.AllowLargeTournament(context.Background(), "u1", 16); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoad_ActiveSubscription(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc := newBilling(&fakeStore{
		count: 40,
		subs: []Sub{{
			UserID: "u1", Store: "google", ProductID: ProductYearly,
			OriginalTxnID: "txn-1", Status: "active", ExpiresAt: now.Add(24 * time.Hour),
		}},
	})
	snap, err := svc.Load(context.Background(), "u1")
	if err != nil || !snap.IsPro {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
	if err := svc.AllowStats(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ExpiredSubscription(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc := newBilling(&fakeStore{
		count: 12,
		subs: []Sub{{
			UserID: "u1", Store: "apple", ProductID: ProductMonthly,
			OriginalTxnID: "txn-2", Status: "active", ExpiresAt: now.Add(-time.Hour),
		}},
	})
	if err := svc.AllowCloudSync(context.Background(), "u1"); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoad_GracePeriod(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	g := now.Add(48 * time.Hour)
	svc := newBilling(&fakeStore{
		count: 12,
		subs: []Sub{{
			UserID: "u1", Store: "google", ProductID: ProductMonthly,
			OriginalTxnID: "txn-g", Status: "grace", ExpiresAt: now.Add(-time.Hour), GraceUntil: &g,
		}},
	})
	snap, err := svc.Load(context.Background(), "u1")
	if err != nil || !snap.IsPro {
		t.Fatalf("grace should count as Pro: %+v err=%v", snap, err)
	}
}

func TestParseGoogleRTDN(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"subscriptionNotification": map[string]any{
			"purchaseToken":  "tok-1",
			"subscriptionId": "pro_monthly",
		},
	})
	token, product, err := ParseGoogleRTDN(base64.StdEncoding.EncodeToString(payload))
	if err != nil || token != "tok-1" || product != "pro_monthly" {
		t.Fatalf("token=%s product=%s err=%v", token, product, err)
	}
}

func TestAppleJWSFallbackPayload(t *testing.T) {
	exp := time.Now().Add(24 * time.Hour).UnixMilli()
	body, _ := json.Marshal(map[string]any{
		"bundleId": appleBundleID, "productId": ProductMonthly,
		"originalTransactionId": "orig-1", "expiresDate": exp,
	})
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	jws := hdr + "." + base64.RawURLEncoding.EncodeToString(body) + "."
	a := &AppleVerifier{bundleID: appleBundleID}
	v, err := a.VerifyApple(context.Background(), jws)
	if err != nil {
		t.Fatal(err)
	}
	if v.OriginalTxnID != "orig-1" || v.Status != "active" {
		t.Fatalf("%+v", v)
	}
}
