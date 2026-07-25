package tournament

import (
	"fmt"
	"testing"
)

func ids(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("p%d", i+1)
	}
	return out
}

func countStatus(plans []MatchPlan, status string) int {
	n := 0
	for _, p := range plans {
		if p.Status == status {
			n++
		}
	}
	return n
}

func maxRound(plans []MatchPlan) int {
	m := 0
	for _, p := range plans {
		if p.Round > m {
			m = p.Round
		}
	}
	return m
}

// ── Single elimination ───────────────────────────────────────────────────────

func TestBuildSingleElim_Power2NoByes(t *testing.T) {
	plans := buildSingleElim(ids(8))

	if got := len(plans); got != 7 { // size-1
		t.Fatalf("match count: got %d, want 7", got)
	}
	if got := maxRound(plans); got != 3 {
		t.Fatalf("rounds: got %d, want 3", got)
	}
	if got := countStatus(plans, "BYE"); got != 0 {
		t.Errorf("byes: got %d, want 0", got)
	}
	// Round 1: 4 READY matches.
	r1ready := 0
	for _, p := range plans {
		if p.Round == 1 && p.Status == "READY" {
			r1ready++
		}
	}
	if r1ready != 4 {
		t.Errorf("round-1 ready matches: got %d, want 4", r1ready)
	}
}

func TestBuildSingleElim_AsListedPairing(t *testing.T) {
	plans := buildSingleElim([]string{"p1", "p2", "p3", "p4"})

	byPos := map[int]MatchPlan{}
	for _, p := range plans {
		if p.Round == 1 {
			byPos[p.Position] = p
		}
	}
	if byPos[0].A != "p1" || byPos[0].B != "p2" {
		t.Errorf("match 0: got %s vs %s, want p1 vs p2", byPos[0].A, byPos[0].B)
	}
	if byPos[1].A != "p3" || byPos[1].B != "p4" {
		t.Errorf("match 1: got %s vs %s, want p3 vs p4", byPos[1].A, byPos[1].B)
	}
}

func TestBuildSingleElim_ByesAdvance(t *testing.T) {
	plans := buildSingleElim(ids(6)) // size 8, 2 byes

	if got := len(plans); got != 7 {
		t.Fatalf("match count: got %d, want 7", got)
	}
	if got := countStatus(plans, "BYE"); got != 2 {
		t.Errorf("byes: got %d, want 2", got)
	}
	// Every BYE match must have a winner set (auto-advanced).
	for _, p := range plans {
		if p.Status == "BYE" && p.Winner == "" {
			t.Errorf("BYE match at r%d p%d has no winner", p.Round, p.Position)
		}
	}
	// At least one round-2 slot should be pre-filled by an advancing BYE winner.
	prefilled := false
	for _, p := range plans {
		if p.Round == 2 && (p.A != "" || p.B != "") {
			prefilled = true
		}
	}
	if !prefilled {
		t.Errorf("expected a BYE winner pre-filled into round 2")
	}
}

func TestBuildSingleElim_NoEmptyMatches(t *testing.T) {
	// n=5 previously risked a BYE-vs-BYE (empty) match; ensure none exist.
	for _, n := range []int{2, 3, 5, 6, 7, 9, 15} {
		plans := buildSingleElim(ids(n))
		for _, p := range plans {
			if p.Round == 1 && p.A == "" && p.B == "" {
				t.Errorf("n=%d: empty round-1 match at pos %d", n, p.Position)
			}
		}
		if got, want := len(plans), nextPow2(n)-1; got != want {
			t.Errorf("n=%d: match count got %d, want %d", n, got, want)
		}
	}
}

func TestBuildSingleElim_TwoPlayers(t *testing.T) {
	plans := buildSingleElim([]string{"p1", "p2"})
	if len(plans) != 1 {
		t.Fatalf("match count: got %d, want 1", len(plans))
	}
	final := plans[0]
	if final.NextRound != -1 {
		t.Errorf("final should have no next match, got round %d", final.NextRound)
	}
	if final.Status != "READY" {
		t.Errorf("final status: got %s, want READY", final.Status)
	}
}

func TestBuildSingleElim_NextLinks(t *testing.T) {
	plans := buildSingleElim(ids(8))
	for _, p := range plans {
		if p.Round < maxRound(plans) {
			if p.NextRound != p.Round+1 || p.NextPos != p.Position/2 {
				t.Errorf("r%d p%d: next got (%d,%d), want (%d,%d)",
					p.Round, p.Position, p.NextRound, p.NextPos, p.Round+1, p.Position/2)
			}
		} else if p.NextRound != -1 {
			t.Errorf("final round match should have no next, got %d", p.NextRound)
		}
	}
}

// ── Round robin ──────────────────────────────────────────────────────────────

func pairKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

func TestBuildRoundRobin_Even(t *testing.T) {
	players := ids(4)
	plans := buildRoundRobin(players)

	if got, want := len(plans), 4*3/2; got != want {
		t.Fatalf("match count: got %d, want %d", got, want)
	}
	if got := maxRound(plans); got != 3 {
		t.Errorf("rounds: got %d, want 3", got)
	}
	assertRoundRobinValid(t, players, plans)
}

func TestBuildRoundRobin_Odd(t *testing.T) {
	players := ids(5)
	plans := buildRoundRobin(players)

	if got, want := len(plans), 5*4/2; got != want {
		t.Fatalf("match count: got %d, want %d", got, want)
	}
	assertRoundRobinValid(t, players, plans)
}

func assertRoundRobinValid(t *testing.T, players []string, plans []MatchPlan) {
	t.Helper()
	seen := map[string]bool{}
	for _, p := range plans {
		if p.A == "" || p.B == "" {
			t.Errorf("round-robin match with empty slot at r%d", p.Round)
		}
		if p.A == p.B {
			t.Errorf("self match: %s", p.A)
		}
		key := pairKey(p.A, p.B)
		if seen[key] {
			t.Errorf("duplicate pairing: %s", key)
		}
		seen[key] = true
	}
	// Every pair must appear exactly once.
	want := len(players) * (len(players) - 1) / 2
	if len(seen) != want {
		t.Errorf("unique pairs: got %d, want %d", len(seen), want)
	}
	// No participant plays twice in the same round.
	perRound := map[int]map[string]bool{}
	for _, p := range plans {
		if perRound[p.Round] == nil {
			perRound[p.Round] = map[string]bool{}
		}
		for _, id := range []string{p.A, p.B} {
			if perRound[p.Round][id] {
				t.Errorf("participant %s plays twice in round %d", id, p.Round)
			}
			perRound[p.Round][id] = true
		}
	}
}
