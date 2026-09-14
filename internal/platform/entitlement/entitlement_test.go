package entitlement

import "testing"

func TestWithinFreeQuota(t *testing.T) {
	if !WithinFreeQuota(Snapshot{FinishedMatchCount: 0}) {
		t.Fatal("0 matches should be free")
	}
	if !WithinFreeQuota(Snapshot{FinishedMatchCount: 9}) {
		t.Fatal("9 matches should be free")
	}
	if WithinFreeQuota(Snapshot{FinishedMatchCount: 10}) {
		t.Fatal("10 matches without Pro should lock")
	}
	if !WithinFreeQuota(Snapshot{IsPro: true, FinishedMatchCount: 99}) {
		t.Fatal("Pro should always be within quota")
	}
}
