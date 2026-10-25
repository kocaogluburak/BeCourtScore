package score

import (
	"testing"
	"time"
)

func TestReplaySinglesHistory_From1500AndSkipsDoubles(t *testing.T) {
	a, b := "u1", "u2"
	played := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	partner := "Bea"
	matches := []Match{
		{ID: "m1", Sport: "TENNIS", PlayerAUserID: &a, PlayerBUserID: &b, WinnerSide: "A", PlayedAt: played},
		{ID: "m-d", Sport: "TENNIS", PlayerAUserID: &a, PlayerBUserID: &b, PlayerA2Name: &partner, WinnerSide: "A", PlayedAt: played.Add(time.Hour)},
		{ID: "m2", Sport: "TENNIS", PlayerAUserID: &a, PlayerBUserID: &b, WinnerSide: "B", PlayedAt: played.Add(2 * time.Hour)},
	}
	rows := replaySinglesHistory(matches)
	if len(rows) != 4 {
		t.Fatalf("rows=%d want 4 (doubles skipped)", len(rows))
	}
	if rows[0].UserID != "u1" || rows[0].Rating != 1516 || rows[0].Counted != 1 {
		t.Fatalf("first winner=%+v", rows[0])
	}
	if rows[1].UserID != "u2" || rows[1].Rating != 1484 {
		t.Fatalf("first loser=%+v", rows[1])
	}
	if rows[2].UserID != "u2" || rows[2].MatchID != "m2" {
		t.Fatalf("second winner=%+v", rows[2])
	}
	if rows[2].Rating == 1516 {
		t.Fatal("second result should use updated ratings, not a fresh 1500")
	}
}

func TestReplaySinglesHistory_IdempotentSameInput(t *testing.T) {
	a, b := "u1", "u2"
	played := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	matches := []Match{
		{ID: "m1", Sport: "PADEL", PlayerAUserID: &a, PlayerBUserID: &b, WinnerSide: "A", PlayedAt: played},
	}
	first := replaySinglesHistory(matches)
	second := replaySinglesHistory(matches)
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("len %d %d", len(first), len(second))
	}
	if first[0] != second[0] || first[1] != second[1] {
		t.Fatalf("replay drifted: %+v vs %+v", first, second)
	}
}
