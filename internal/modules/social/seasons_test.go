package social

import (
	"testing"
	"time"
)

func TestBuildSeasonStandings_WinLossAbsentAndWindow(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 56)
	members := []seasonMember{
		{UserID: "a", Name: "Ada", DivisionTier: 1},
		{UserID: "b", Name: "Bea", DivisionTier: 1},
		{UserID: "c", Name: "Cid", DivisionTier: 1},
	}
	matches := []ladderMatch{
		{Sport: "TENNIS", PlayedAt: start, PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: start.Add(-time.Second), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: end, PlayerA: "b", PlayerB: "a", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "PADEL", PlayedAt: start.Add(time.Hour), PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: start.Add(2 * time.Hour), PlayerA: "a", PlayerB: "stranger", SetsA: 2, SetsB: 0, WinnerSide: "A"},
	}
	rows := buildSeasonStandings(members, matches, "TENNIS", start, end, nil)
	by := map[string]SeasonStanding{}
	for _, r := range rows {
		by[r.UserID] = r
	}
	if by["a"].Points != 3 || by["a"].Wins != 1 || by["a"].SetDiff != 2 {
		t.Fatalf("winner %+v", by["a"])
	}
	if by["b"].Points != 1 || by["b"].Losses != 1 || by["b"].SetDiff != -2 {
		t.Fatalf("loser %+v", by["b"])
	}
	if by["c"].Points != 0 || by["c"].Matches != 0 {
		t.Fatalf("absent %+v", by["c"])
	}
	if rows[0].UserID != "a" || rows[0].Rank != 1 {
		t.Fatalf("rank %+v", rows)
	}
}

func TestBuildSeasonStandings_DoublesAndCrossDivisionIgnored(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 56)
	members := []seasonMember{
		{UserID: "a", Name: "A", DivisionTier: 1},
		{UserID: "b", Name: "B", DivisionTier: 1},
		{UserID: "c", Name: "C", DivisionTier: 2},
		{UserID: "d", Name: "D", DivisionTier: 2},
	}
	matches := []ladderMatch{
		{Sport: "PADEL", PlayedAt: start, PlayerA: "a", PlayerA2: "b", PlayerB: "c", PlayerB2: "d", SetsA: 2, SetsB: 1, WinnerSide: "A"},
		{Sport: "PADEL", PlayedAt: start.Add(time.Hour), PlayerA: "c", PlayerB: "d", SetsA: 2, SetsB: 0, WinnerSide: "A"},
	}
	rows := buildSeasonStandings(members, matches, "PADEL", start, end, nil)
	by := map[string]SeasonStanding{}
	for _, r := range rows {
		by[r.UserID] = r
	}
	if by["a"].Matches != 0 || by["b"].Matches != 0 {
		t.Fatalf("cross-division should be ignored: %+v", by)
	}
	if by["c"].Points != 3 || by["d"].Points != 1 {
		t.Fatalf("div2 %+v %+v", by["c"], by["d"])
	}
}

func TestBuildSeasonStandings_HeadToHeadBreaksTie(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 56)
	members := []seasonMember{
		{UserID: "a", Name: "A", DivisionTier: 1},
		{UserID: "b", Name: "B", DivisionTier: 1},
	}
	matches := []ladderMatch{
		{Sport: "TENNIS", PlayedAt: start, PlayerA: "a", PlayerB: "b", SetsA: 2, SetsB: 1, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: start.Add(time.Hour), PlayerA: "b", PlayerB: "a", SetsA: 2, SetsB: 1, WinnerSide: "A"},
	}
	rows := buildSeasonStandings(members, matches, "TENNIS", start, end, nil)
	if rows[0].Points != rows[1].Points || rows[0].Wins != rows[1].Wins {
		t.Fatalf("expected points/wins tie %+v", rows)
	}
	if rows[0].UserID != "a" {
		t.Fatalf("h2h should keep a first after equal record + user_id; got %+v", rows)
	}
}

func TestSocialNightPoints_PlacementMapping(t *testing.T) {
	got := socialNightPoints([]NightPlacement{
		{UserID: "a", Rank: 1, Played: true},
		{UserID: "b", Rank: 2, Played: true},
		{UserID: "c", Rank: 3, Played: true},
		{UserID: "d", Rank: 4, Played: true},
		{UserID: "e", Rank: 0, Played: false},
	})
	if got["a"] != 3 || got["b"] != 2 || got["c"] != 1 || got["d"] != 1 {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["e"]; ok {
		t.Fatalf("absent should not score: %+v", got)
	}
}

func TestSplitPromotionRelegation_Top2Bottom2AndChampion(t *testing.T) {
	d1 := []SeasonStanding{}
	for i, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		d1 = append(d1, SeasonStanding{UserID: id, Rank: i + 1, DivisionTier: 1})
	}
	d2 := []SeasonStanding{}
	for i, id := range []string{"i", "j", "k", "l", "m", "n", "o", "p"} {
		d2 = append(d2, SeasonStanding{UserID: id, Rank: i + 1, DivisionTier: 2})
	}
	got := splitPromotionRelegation([][]SeasonStanding{d1, d2})
	if got[0][0].Outcome != outcomeChampion || got[0][0].UserID != "a" {
		t.Fatalf("champion %+v", got[0][0])
	}
	if got[0][6].Outcome != outcomeRelegated || got[0][7].Outcome != outcomeRelegated {
		t.Fatalf("relegate d1 %+v %+v", got[0][6], got[0][7])
	}
	if got[1][0].Outcome != outcomePromoted || got[1][1].Outcome != outcomePromoted {
		t.Fatalf("promote d2 %+v %+v", got[1][0], got[1][1])
	}
	if got[0][1].Outcome != outcomeStayed || got[1][2].Outcome != outcomeStayed {
		t.Fatalf("stayed %+v %+v", got[0][1], got[1][2])
	}
}

func TestSplitPromotionRelegation_SmallDivisionUsesOne(t *testing.T) {
	d1 := []SeasonStanding{
		{UserID: "a", Rank: 1, DivisionTier: 1},
		{UserID: "b", Rank: 2, DivisionTier: 1},
		{UserID: "c", Rank: 3, DivisionTier: 1},
	}
	d2 := []SeasonStanding{
		{UserID: "d", Rank: 1, DivisionTier: 2},
		{UserID: "e", Rank: 2, DivisionTier: 2},
		{UserID: "f", Rank: 3, DivisionTier: 2},
	}
	got := splitPromotionRelegation([][]SeasonStanding{d1, d2})
	if got[0][0].Outcome != outcomeChampion {
		t.Fatalf("champ %+v", got[0][0])
	}
	if got[0][2].Outcome != outcomeRelegated {
		t.Fatalf("relegate one %+v", got[0][2])
	}
	if got[1][0].Outcome != outcomePromoted {
		t.Fatalf("promote one %+v", got[1][0])
	}
	if got[1][1].Outcome != outcomeStayed || got[0][1].Outcome != outcomeStayed {
		t.Fatalf("middle stayed")
	}
}

func TestNextSeasonDivisions_NewMembersLowestTier(t *testing.T) {
	prev := []SeasonStanding{
		{UserID: "a", Name: "A", DivisionTier: 1, Outcome: outcomeChampion},
		{UserID: "b", Name: "B", DivisionTier: 1, Outcome: outcomeRelegated},
		{UserID: "c", Name: "C", DivisionTier: 2, Outcome: outcomePromoted},
		{UserID: "d", Name: "D", DivisionTier: 2, Outcome: outcomeStayed},
	}
	next := nextSeasonDivisions(prev, []seasonMember{{UserID: "e", Name: "E"}})
	by := map[string]int{}
	for _, m := range next {
		by[m.UserID] = m.DivisionTier
	}
	if by["a"] != 1 || by["c"] != 1 {
		t.Fatalf("top %+v", by)
	}
	if by["b"] != 2 || by["d"] != 2 {
		t.Fatalf("second %+v", by)
	}
	if by["e"] != 2 {
		t.Fatalf("newcomer should be lowest: %+v", by)
	}
}

func TestSeedFirstDivisions_SingleUnlessRequested(t *testing.T) {
	members := []GroupMember{{UserID: "a"}, {UserID: "b"}, {UserID: "c"}}
	one := seedFirstDivisions(members, 1)
	for _, m := range one {
		if m.DivisionTier != 1 {
			t.Fatalf("%+v", m)
		}
	}
	two := seedFirstDivisions(makeMembers(10), 2)
	tiers := map[int]int{}
	for _, m := range two {
		tiers[m.DivisionTier]++
	}
	if tiers[1] == 0 || tiers[2] == 0 {
		t.Fatalf("expected two tiers %+v", tiers)
	}
}

func makeMembers(n int) []GroupMember {
	out := make([]GroupMember, n)
	for i := 0; i < n; i++ {
		out[i] = GroupMember{UserID: string(rune('a' + i))}
	}
	return out
}

func TestSeasonWindow_Weeks(t *testing.T) {
	start := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	gotStart, end := seasonWindow(start, 8)
	if !gotStart.Equal(start.UTC()) {
		t.Fatalf("start %s", gotStart)
	}
	if !end.Equal(gotStart.AddDate(0, 0, 56)) {
		t.Fatalf("end %s", end)
	}
}
