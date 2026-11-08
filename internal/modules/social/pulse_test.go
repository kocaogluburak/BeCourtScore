package social

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestUserSummary_OmitsTimeZone(t *testing.T) {
	raw, err := json.Marshal(UserSummary{ID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "time_zone") {
		t.Fatalf("public profile leaked time zone: %s", raw)
	}
}

func TestSelectPulseRank_MatchesLadderAndDropsZero(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	played := start.Add(48 * time.Hour)
	members := []GroupMember{{UserID: "u1", Name: "Ada"}, {UserID: "u2", Name: "Ken"}}
	group := pulseGroup{ID: "g1", Name: "Salı kortu", Sport: "TENNIS", Members: members}
	win := ladderMatch{
		Sport: "TENNIS", PlayedAt: played, PlayerA: "u1", PlayerB: "u2",
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	}
	rank := selectPulseRank("u1", []pulseGroup{group}, []ladderMatch{win}, nil, start, end)
	standings := buildStandings(members, []ladderMatch{win}, "TENNIS", start, end, nil)
	var ladder Standing
	for _, row := range standings {
		if row.UserID == "u1" {
			ladder = row
		}
	}
	if rank == nil || rank.Rank != ladder.Rank || rank.Points != ladder.Points || rank.Points < 1 || rank.GroupName != "Salı kortu" {
		t.Fatalf("rank=%+v ladder=%+v", rank, ladder)
	}

	if got := selectPulseRank("u1", []pulseGroup{group}, nil, nil, start, end); got != nil {
		t.Fatalf("zero points rank=%+v", got)
	}
	guest := win
	guest.PlayerB = ""
	if got := selectPulseRank("u1", []pulseGroup{group}, []ladderMatch{guest}, nil, start, end); got != nil {
		t.Fatalf("guest match counted: %+v", got)
	}
}

func TestSelectPulseRank_LatestGroupThenSmallerID(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	early := start.Add(24 * time.Hour)
	late := start.Add(72 * time.Hour)
	a := pulseGroup{ID: "g-a", Name: "A", Sport: "TENNIS", Members: []GroupMember{{UserID: "u1", Name: "Ada"}, {UserID: "u2", Name: "Ken"}}}
	b := pulseGroup{ID: "g-b", Name: "B", Sport: "TENNIS", Members: []GroupMember{{UserID: "u1", Name: "Ada"}, {UserID: "u3", Name: "Lin"}}}
	matches := []ladderMatch{
		{Sport: "TENNIS", PlayedAt: early, PlayerA: "u1", PlayerB: "u2", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: late, PlayerA: "u1", PlayerB: "u3", SetsA: 2, SetsB: 1, WinnerSide: "A"},
	}
	got := selectPulseRank("u1", []pulseGroup{a, b}, matches, nil, start, end)
	if got == nil || got.GroupID != "g-b" {
		t.Fatalf("latest=%+v", got)
	}

	same := []ladderMatch{
		{Sport: "TENNIS", PlayedAt: late, PlayerA: "u1", PlayerB: "u2", SetsA: 2, SetsB: 0, WinnerSide: "A"},
		{Sport: "TENNIS", PlayedAt: late, PlayerA: "u1", PlayerB: "u3", SetsA: 2, SetsB: 0, WinnerSide: "A"},
	}
	got = selectPulseRank("u1", []pulseGroup{b, a}, same, nil, start, end)
	if got == nil || got.GroupID != "g-a" {
		t.Fatalf("tie id=%+v", got)
	}

	// Leaving g-b removes it from the membership load. The earlier group remains.
	got = selectPulseRank("u1", []pulseGroup{a}, matches, nil, start, end)
	if got == nil || got.GroupID != "g-a" {
		t.Fatalf("after leave=%+v", got)
	}
	if got = selectPulseRank("u1", nil, matches, nil, start, end); got != nil {
		t.Fatalf("no groups=%+v", got)
	}
}

func TestSelectPulseRank_SocialNightScoresRankNotFriends(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	group := pulseGroup{ID: "g1", Name: "Night", Sport: "PADEL", Members: []GroupMember{
		{UserID: "u1", Name: "Ada"}, {UserID: "u2", Name: "Ken"},
	}}
	night := socialNight{
		TournamentID: "t1", Format: "AMERICANO", Status: "COMPLETED", GroupID: "g1",
		FinishedAt: start.Add(36 * time.Hour),
		UserIDs:    []string{"u1", "u2"},
		Placements: []NightPlacement{
			{UserID: "u1", Rank: 1, Played: true},
			{UserID: "u2", Rank: 2, Played: true},
		},
	}
	rank := selectPulseRank("u1", []pulseGroup{group}, nil, []socialNight{night}, start, end)
	if rank == nil || rank.Points < 1 || rank.GroupID != "g1" {
		t.Fatalf("night rank=%+v", rank)
	}
	if friends := selectPulseFriends("u1", nil); friends != nil {
		t.Fatalf("night leaked into friends: %+v", friends)
	}
}

func TestSelectPulseFriends_OrderCapAndBlanks(t *testing.T) {
	t1 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(24 * time.Hour)
	t4 := t1.Add(72 * time.Hour)
	rows := []pulseFriend{
		{UserID: "f4", Name: "Fourth", PlayedAt: t1},
		{UserID: "f2", Name: "Second", PlayedAt: t4},
		{UserID: "f1", Name: "First", PlayedAt: t4},
		{UserID: "me", Name: "Me", PlayedAt: t4.Add(time.Hour)},
		{UserID: "blank", Name: "  ", PlayedAt: t4.Add(time.Hour)},
		{UserID: "f3", Name: "Third", PlayedAt: t2},
	}
	got := selectPulseFriends("me", rows)
	if got == nil || len(got.Names) != 3 || got.Others != 1 {
		t.Fatalf("got=%+v", got)
	}
	if got.Names[0] != "First" || got.Names[1] != "Second" || got.Names[2] != "Third" {
		t.Fatalf("names=%v", got.Names)
	}
	if again := selectPulseFriends("me", []pulseFriend{{UserID: "f1", Name: "First", PlayedAt: t4}}); again == nil || again.Others != 0 {
		t.Fatalf("single=%+v", again)
	}
}

func TestAssembleAudience_MatchIncludesThreeArms(t *testing.T) {
	got := assembleAudience([]string{"player", ""}, []string{"friend"}, []string{"member", "player"})
	if len(got) != 3 || got[0] != "player" || got[1] != "friend" || got[2] != "member" {
		t.Fatalf("audience=%v", got)
	}
	// An opted-out friend is omitted before the union, so the friend arm is absent.
	without := assembleAudience([]string{"player"}, nil, []string{"member"})
	for _, id := range without {
		if id == "optout" {
			t.Fatal("opt-out friend received the friend arm")
		}
	}
	night := assembleAudience([]string{"player"}, []string{"member"})
	if len(night) != 2 {
		t.Fatalf("night=%v", night)
	}
}

func TestPulseAudienceSQL_FriendArmAndNight(t *testing.T) {
	if !strings.Contains(pulseMatchAudienceSQL, "status = 'accepted'") || !strings.Contains(pulseMatchAudienceSQL, "NOT u.feed_opt_out") {
		t.Fatal("match audience must drop pending and opt-out friends")
	}
	if strings.Contains(pulseNightGroupAudienceSQL, "friendships") || strings.Contains(pulseNightOpenAudienceSQL, "friendships") {
		t.Fatal("a social night must not invent a friend-match event")
	}
}

func TestPulseFriendsSQL_AcceptedFinishedMatchesOnly(t *testing.T) {
	if !strings.Contains(pulseFriendsSQL, "status = 'accepted'") {
		t.Fatal("pending requests must stay out")
	}
	if !strings.Contains(pulseFriendsSQL, "NOT u.feed_opt_out") {
		t.Fatal("opt-out friends must stay out")
	}
	if !strings.Contains(pulseFriendsSQL, "MAX(m.played_at)") || !strings.Contains(pulseFriendsSQL, "GROUP BY") {
		t.Fatal("a second match must not open a second row")
	}
	if strings.Contains(pulseFriendsSQL, "live_matches") || strings.Contains(pulseFriendsSQL, "tournament") {
		t.Fatal("unfinished matches and social nights are not friend news")
	}
}
