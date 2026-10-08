package tournament

import "testing"

func TestAmericano_Rotation(t *testing.T) {
	cases := []struct {
		n      int
		courts int
	}{
		{4, 1},
		{5, 1},
		{8, 2},
	}
	for _, tc := range cases {
		players := ids(tc.n)
		plans := buildAmericano(players, tc.courts)
		if len(plans) == 0 {
			t.Fatalf("n=%d: empty schedule", tc.n)
		}
		byRound := map[int][]MatchPlan{}
		for _, p := range plans {
			if p.A == "" || p.A2 == "" || p.B == "" || p.B2 == "" {
				t.Fatalf("n=%d: fixture missing a player %+v", tc.n, p)
			}
			byRound[p.Round] = append(byRound[p.Round], p)
		}
		if len(byRound) != socialRoundCount(tc.n) {
			t.Fatalf("n=%d: rounds=%d want %d", tc.n, len(byRound), socialRoundCount(tc.n))
		}
		var prev map[string]string
		for r := 1; r <= len(byRound); r++ {
			seen := map[string]bool{}
			partners := map[string]string{}
			for _, p := range byRound[r] {
				for _, id := range []string{p.A, p.A2, p.B, p.B2} {
					if seen[id] {
						t.Fatalf("n=%d round %d: %s plays twice", tc.n, r, id)
					}
					seen[id] = true
				}
				partners[p.A] = p.A2
				partners[p.A2] = p.A
				partners[p.B] = p.B2
				partners[p.B2] = p.B
			}
			wantPlaying := tc.courts * 4
			if wantPlaying > tc.n {
				wantPlaying = (tc.n / 4) * 4
			}
			if len(seen) != wantPlaying {
				t.Fatalf("n=%d round %d: playing=%d want %d", tc.n, r, len(seen), wantPlaying)
			}
			if tc.n%2 == 1 && len(seen) != tc.n-1 {
				t.Fatalf("n=%d round %d: bye count wrong, playing %d", tc.n, r, len(seen))
			}
			if prev != nil {
				for id, partner := range partners {
					if prev[id] == partner {
						t.Fatalf("n=%d round %d: %s stays with %s", tc.n, r, id, partner)
					}
				}
			}
			prev = partners
		}
	}
}

func TestAmericano_FivePlayerByeRotates(t *testing.T) {
	plans := buildAmericano(ids(5), 1)
	sat := map[string]int{}
	for _, id := range ids(5) {
		sat[id] = 0
	}
	byRound := map[int]map[string]bool{}
	for _, p := range plans {
		if byRound[p.Round] == nil {
			byRound[p.Round] = map[string]bool{}
		}
		for _, id := range []string{p.A, p.A2, p.B, p.B2} {
			byRound[p.Round][id] = true
		}
	}
	for r := 1; r <= 5; r++ {
		for _, id := range ids(5) {
			if !byRound[r][id] {
				sat[id]++
			}
		}
	}
	for id, n := range sat {
		if n != 1 {
			t.Fatalf("%s sat %d times, want 1", id, n)
		}
	}
}

func TestPointStandings_PointsThenRoundWins(t *testing.T) {
	parts := []Participant{
		{ID: "p1", DisplayName: "A"},
		{ID: "p2", DisplayName: "B"},
		{ID: "p3", DisplayName: "C"},
		{ID: "p4", DisplayName: "D"},
	}
	matches := []TournamentMatch{
		socialMatch("p1", "p2", "p3", "p4", "16-12"),
		socialMatch("p1", "p3", "p2", "p4", "10-16"),
	}
	got := computePointStandings(parts, matches)
	// p2 is on the winning side both times: 16+16. p4 scores 12 then 16.
	if got[0].ParticipantID != "p2" || got[0].Points != 32 || got[0].Won != 2 {
		t.Fatalf("top = %+v, want p2 32/2", got[0])
	}
	if got[1].ParticipantID != "p4" || got[1].Points != 28 || got[1].Won != 1 {
		t.Fatalf("second = %+v, want p4 28/1", got[1])
	}
	if got[2].ParticipantID != "p1" || got[2].Points != 26 {
		t.Fatalf("third = %+v", got[2])
	}
}

func TestPointStandings_RoundWinsBreakPointTie(t *testing.T) {
	parts := []Participant{
		{ID: "p1", DisplayName: "A"},
		{ID: "p2", DisplayName: "B"},
	}
	matches := []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "10-10"),
		socialMatch("p1", "x", "p2", "y", "12-8"),
	}
	// p1 points 22 wins 1; p2 points 18 wins 0. Not a tie.
	// Force equal points, different wins: 10-10 and then we need equal totals with one win.
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "16-8"),
		socialMatch("p2", "x", "p1", "y", "8-16"),
	}
	// p1: 16+16=32, wins 2. p2: 8+8=16.
	// Equal points:
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "12-12"),
		socialMatch("p1", "x", "p2", "y", "10-10"),
	}
	got := computePointStandings(parts, matches)
	if got[0].Points != got[1].Points || got[0].Won != 0 {
		t.Fatalf("expected a scoreless tie, got %+v %+v", got[0], got[1])
	}
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "16-16"),
		socialMatch("p1", "x", "p2", "y", "8-8"),
		// third match gives p2 the only round win but fewer... wait both get same if 8-8.
		socialMatch("p2", "x", "p1", "y", "20-4"),
	}
	// p1: 16+8+4=28 wins 0. p2: 16+8+20=44.
	// Want equal points, p2 more wins:
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "10-14"),
		socialMatch("p2", "x", "p1", "y", "10-14"),
	}
	// p1: 10+14=24, wins 1 (second match). p2: 14+10=24, wins 1 (first). Still tied.
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "10-14"),
		socialMatch("p1", "x", "p2", "y", "14-10"),
	}
	// p1: 10+14=24 wins 1. p2: 14+10=24 wins 1. Still tied.
	// p1 wins both but points equal only if each match is equal points which can't both be wins.
	// Points equal AND wins differ means one player won a low-scoring match and lost a high one.
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "8-16"),  // p2 wins, p1=8 p2=16
		socialMatch("p1", "x", "p2", "y", "16-8"),  // p1 wins, p1=24 p2=24
		socialMatch("p1", "x", "p2", "y", "12-12"), // tie, p1=36 p2=36
		socialMatch("p1", "x", "p2", "y", "4-4"),   // still tied wins
		socialMatch("p2", "x", "p1", "y", "1-0"),   // p2 wins, p2=37 p1=36
	}
	// Simpler: two matches, p1 wins 20-10, p2 wins 15-5.
	// p1 points 20+5=25 wins 1. p2 points 10+15=25 wins 1. Still 1-1.
	// Three matches: p1 wins 20-10, p2 wins 15-5, tie 0-0.
	// still 1-1.
	// p1 wins one blowout and loses two close ones?
	// Match1 p1 wins 30-0 (p1=30 p2=0, p1 wins)
	// Match2 p2 wins 15-0 (p1=30 p2=15)
	// Match3 p2 wins 15-0 (p1=30 p2=30, p2 has 2 wins, p1 has 1)
	matches = []TournamentMatch{
		socialMatch("p1", "x", "p2", "y", "30-0"),
		socialMatch("p2", "x", "p1", "y", "15-0"),
		socialMatch("p2", "x", "p1", "y", "15-0"),
	}
	got = computePointStandings(parts, matches)
	if got[0].ParticipantID != "p2" || got[0].Points != 30 || got[0].Won != 2 {
		t.Fatalf("want p2 on round wins, got %+v", got[0])
	}
	if got[1].Points != 30 || got[1].Won != 1 {
		t.Fatalf("p1 = %+v", got[1])
	}
}

func TestMexicano_PairsOneFourVsTwoThree(t *testing.T) {
	plans := buildSocialRound(ids(8), 2, 1)
	if len(plans) != 2 {
		t.Fatalf("courts=%d", len(plans))
	}
	if plans[0].A != "p1" || plans[0].A2 != "p4" || plans[0].B != "p2" || plans[0].B2 != "p3" {
		t.Fatalf("court 0 = %+v", plans[0])
	}
	if plans[1].A != "p5" || plans[1].B2 != "p7" {
		t.Fatalf("court 1 = %+v", plans[1])
	}
}

func TestKing_OneCourtStays(t *testing.T) {
	matches := []TournamentMatch{socialMatch("p1", "p4", "p2", "p3", "16-10")}
	matches[0].Round = 1
	matches[0].PositionInRound = 0
	standings := computePointStandings([]Participant{
		{ID: "p1", DisplayName: "A"},
		{ID: "p2", DisplayName: "B"},
		{ID: "p3", DisplayName: "C"},
		{ID: "p4", DisplayName: "D"},
	}, matches)
	next := buildKingRound(matches, 1, standings, 1)
	if len(next) != 1 {
		t.Fatalf("next=%d", len(next))
	}
	idsOnCourt := map[string]bool{next[0].A: true, next[0].A2: true, next[0].B: true, next[0].B2: true}
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		if !idsOnCourt[id] {
			t.Fatalf("missing %s in %+v", id, next[0])
		}
	}
}

func TestKing_WinnersMoveUp(t *testing.T) {
	top := socialMatch("p1", "p2", "p3", "p4", "16-8")
	top.Round, top.PositionInRound = 1, 0
	bottom := socialMatch("p5", "p6", "p7", "p8", "20-4")
	bottom.Round, bottom.PositionInRound = 1, 1
	matches := []TournamentMatch{top, bottom}
	parts := make([]Participant, 8)
	for i, id := range ids(8) {
		parts[i] = Participant{ID: id, DisplayName: id}
	}
	standings := computePointStandings(parts, matches)
	next := buildKingRound(matches, 1, standings, 2)
	if len(next) != 2 {
		t.Fatalf("next=%d", len(next))
	}
	topIDs := map[string]bool{next[0].A: true, next[0].A2: true, next[0].B: true, next[0].B2: true}
	for _, id := range []string{"p1", "p2", "p5", "p6"} {
		if !topIDs[id] {
			t.Fatalf("top court missing %s: %+v", id, next[0])
		}
	}
	bottomIDs := map[string]bool{next[1].A: true, next[1].A2: true, next[1].B: true, next[1].B2: true}
	for _, id := range []string{"p3", "p4", "p7", "p8"} {
		if !bottomIDs[id] {
			t.Fatalf("bottom court missing %s: %+v", id, next[1])
		}
	}
}

func TestKing_ByeReplacesBottom(t *testing.T) {
	m := socialMatch("p1", "p2", "p3", "p4", "16-10")
	m.Round, m.PositionInRound = 1, 0
	parts := []Participant{
		{ID: "p1", DisplayName: "A"},
		{ID: "p2", DisplayName: "B"},
		{ID: "p3", DisplayName: "C"},
		{ID: "p4", DisplayName: "D"},
		{ID: "p5", DisplayName: "E"},
	}
	matches := []TournamentMatch{m}
	standings := computePointStandings(parts, matches)
	next := buildKingRound(matches, 1, standings, 1)
	idsOnCourt := map[string]bool{next[0].A: true, next[0].A2: true, next[0].B: true, next[0].B2: true}
	if !idsOnCourt["p5"] {
		t.Fatalf("sitter should enter, got %+v", next[0])
	}
	if idsOnCourt["p4"] {
		t.Fatalf("lowest scorer should sit, got %+v", next[0])
	}
}

func socialMatch(a, a2, b, b2, score string) TournamentMatch {
	return TournamentMatch{
		ParticipantAID:  &a,
		ParticipantA2ID: &a2,
		ParticipantBID:  &b,
		ParticipantB2ID: &b2,
		ScoreSummary:    &score,
		Status:          "COMPLETED",
	}
}
