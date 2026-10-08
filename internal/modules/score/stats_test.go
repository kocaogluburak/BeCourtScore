package score

import (
	"testing"
	"time"
)

func TestParticipantOutcome_RecorderOnlyIgnored(t *testing.T) {
	m := Match{
		PlayerAName: "Ada", PlayerBName: "Grace",
		WinnerSide: "A", CreatedBy: "recorder",
	}
	if got := participantOutcome(m, "recorder"); got != "" {
		t.Fatalf("recorder-only outcome=%q", got)
	}
}

func TestAggregateUserStats_FormOrderAndRivals(t *testing.T) {
	me, opp1, opp2 := "u1", "u2", "u3"
	now := time.Now()
	matches := []Match{
		{Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp1,
			PlayerAName: "Me", PlayerBName: "Ali", SetsA: 2, SetsB: 1, WinnerSide: "A",
			PlayedAt: now},
		{Sport: "TENNIS", PlayerAUserID: &opp1, PlayerBUserID: &me,
			PlayerAName: "Ali", PlayerBName: "Me", SetsA: 2, SetsB: 0, WinnerSide: "A",
			PlayedAt: now.Add(-time.Hour)},
		{Sport: "PADEL", PlayerAUserID: &me, PlayerBUserID: &opp2,
			PlayerAName: "Me", PlayerBName: "Mert", SetsA: 2, SetsB: 0, WinnerSide: "A",
			PlayedAt: now.Add(-2 * time.Hour)},
		// Recorder-only — must not count
		{Sport: "TENNIS", PlayerAName: "X", PlayerBName: "Y", WinnerSide: "A",
			CreatedBy: me, SetsA: 2, SetsB: 0, PlayedAt: now.Add(-3 * time.Hour)},
		// Guest opponent — counts for W/L but not rivals
		{Sport: "TENNIS", PlayerAUserID: &me, PlayerAName: "Me", PlayerBName: "Guest",
			SetsA: 2, SetsB: 1, WinnerSide: "A", PlayedAt: now.Add(-4 * time.Hour)},
	}

	all := aggregateUserStats(me, "", matches)
	if all.Matches != 4 || all.Wins != 3 || all.Losses != 1 {
		t.Fatalf("totals: matches=%d w=%d l=%d", all.Matches, all.Wins, all.Losses)
	}
	if all.SetDiff != (2-1)+(0-2)+(2-0)+(2-1) {
		t.Fatalf("set_diff=%d", all.SetDiff)
	}
	wantForm := []string{"W", "L", "W", "W"}
	if len(all.Form) != 4 {
		t.Fatalf("form=%v", all.Form)
	}
	for i := range wantForm {
		if all.Form[i] != wantForm[i] {
			t.Fatalf("form[%d]=%s want %s full=%v", i, all.Form[i], wantForm[i], all.Form)
		}
	}
	if len(all.Rivals) != 2 {
		t.Fatalf("rivals=%+v", all.Rivals)
	}
	if all.Rivals[0].UserID != opp1 || all.Rivals[0].Matches != 2 {
		t.Fatalf("top rival=%+v", all.Rivals[0])
	}
	if len(all.BySport) != 2 {
		t.Fatalf("by_sport=%+v", all.BySport)
	}

	tennis := aggregateUserStats(me, "TENNIS", matches)
	if tennis.Matches != 3 || tennis.Wins != 2 || tennis.Losses != 1 {
		t.Fatalf("tennis totals: %+v", tennis)
	}
	if tennis.Sport == nil || *tennis.Sport != "TENNIS" {
		t.Fatalf("sport ptr=%v", tennis.Sport)
	}
	// by_sport still includes all sports
	if len(tennis.BySport) != 2 {
		t.Fatalf("by_sport with filter=%+v", tennis.BySport)
	}
}

func TestAggregateHeadToHead_ViewerPerspective(t *testing.T) {
	me, opp := "u1", "u2"
	now := time.Now()
	matches := []Match{
		{Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp,
			SetsA: 2, SetsB: 1, WinnerSide: "A", PlayedAt: now},
		{Sport: "TENNIS", PlayerAUserID: &opp, PlayerBUserID: &me,
			SetsA: 2, SetsB: 0, WinnerSide: "A", PlayedAt: now.Add(-time.Hour)},
		{Sport: "PADEL", PlayerAUserID: &me, PlayerBUserID: &opp,
			SetsA: 2, SetsB: 0, WinnerSide: "A", PlayedAt: now.Add(-2 * time.Hour)},
	}

	h2h := aggregateHeadToHead(me, opp, "", matches)
	if h2h.Matches != 3 || h2h.Wins != 2 || h2h.Losses != 1 {
		t.Fatalf("h2h=%+v", h2h)
	}
	if h2h.Form[0] != "W" || h2h.Form[1] != "L" || h2h.Form[2] != "W" {
		t.Fatalf("form=%v", h2h.Form)
	}
	if h2h.LastPlayedAt == nil || !h2h.LastPlayedAt.Equal(now) {
		t.Fatalf("last_played=%v", h2h.LastPlayedAt)
	}

	tennis := aggregateHeadToHead(me, opp, "TENNIS", matches)
	if tennis.Matches != 2 || tennis.Wins != 1 {
		t.Fatalf("tennis h2h=%+v", tennis)
	}

	empty := aggregateHeadToHead(me, "nobody", "", nil)
	if empty.Matches != 0 || empty.Form == nil || len(empty.Form) != 0 {
		t.Fatalf("empty=%+v", empty)
	}
}

func TestAggregateUserStats_DoublesPartnerCounts(t *testing.T) {
	me, partner, opp, opp2 := "u1", "u2", "u3", "u4"
	partnerName, opp2Name := "Partner", "Opp2"
	matches := []Match{{
		Sport: "PADEL", PlayerAName: "Me", PlayerBName: "Opp",
		PlayerAUserID: &me, PlayerA2UserID: &partner, PlayerA2Name: &partnerName,
		PlayerBUserID: &opp, PlayerB2UserID: &opp2, PlayerB2Name: &opp2Name,
		SetsA: 2, SetsB: 1, WinnerSide: "A",
	}}
	got := aggregateUserStats(partner, "", matches)
	if got.Matches != 1 || got.Wins != 1 || got.Losses != 0 {
		t.Fatalf("partner stats=%+v", got)
	}
	if len(got.Rivals) != 2 {
		t.Fatalf("rivals=%+v", got.Rivals)
	}
	for _, r := range got.Rivals {
		if r.UserID == me {
			t.Fatalf("same-side partner counted as rival: %+v", got.Rivals)
		}
	}
}
