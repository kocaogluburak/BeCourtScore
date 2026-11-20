package entitlement

import "testing"

func TestFreeMaxLeagueMembers(t *testing.T) {
	if FreeMaxLeagueMembers != 8 {
		t.Fatalf("FreeMaxLeagueMembers=%d", FreeMaxLeagueMembers)
	}
}

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

func TestWithinFreeH2HQuota(t *testing.T) {
	if !WithinFreeH2HQuota(Snapshot{FinishedMatchCount: 0}) {
		t.Fatal("0 matches should allow H2H")
	}
	if !WithinFreeH2HQuota(Snapshot{FinishedMatchCount: 10}) {
		t.Fatal("10th free cloud match should still allow H2H")
	}
	if WithinFreeH2HQuota(Snapshot{FinishedMatchCount: 11}) {
		t.Fatal("11 matches without Pro should lock H2H")
	}
	if !WithinFreeH2HQuota(Snapshot{IsPro: true, FinishedMatchCount: 99}) {
		t.Fatal("Pro should always allow H2H")
	}
}
