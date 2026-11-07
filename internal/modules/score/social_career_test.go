package score

import (
	"testing"
	"time"
)

func TestNightRanksAndSummary_BestPlace(t *testing.T) {
	night := []nightPlayer{
		{UserID: "a", Points: 20, Won: 2, Played: 2},
		{UserID: "b", Points: 10, Won: 1, Played: 2},
		{UserID: "c", Points: 0, Played: 0},
	}
	ranks := nightRanks(night)
	if ranks["a"] != 1 || ranks["b"] != 2 {
		t.Fatalf("ranks=%v", ranks)
	}
	if _, ok := ranks["c"]; ok {
		t.Fatal("sit-out should not place")
	}
	stats := summarizeUserNights("b", [][]nightPlayer{night, {
		{UserID: "b", Points: 30, Won: 3, Played: 3},
		{UserID: "a", Points: 10, Won: 1, Played: 3},
	}})
	if stats.Nights != 2 || stats.BestRank != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestCountSocialH2H_IgnoresTiesAndSetScores(t *testing.T) {
	rows := []socialFixture{
		{SideA: [2]string{"me", "p"}, SideB: [2]string{"opp", "q"}, APts: 16, BPts: 12},
		{SideA: [2]string{"opp", "p"}, SideB: [2]string{"me", "q"}, APts: 16, BPts: 10},
		{SideA: [2]string{"me", "p"}, SideB: [2]string{"opp", "q"}, APts: 16, BPts: 16},
		{SideA: [2]string{"me", "opp"}, SideB: [2]string{"me", "y"}, APts: 16, BPts: 8},
	}
	got := countSocialH2H("me", rows)
	if got.Rounds != 2 || got.Wins != 1 || got.Losses != 1 {
		t.Fatalf("h2h=%+v", got)
	}
}

func TestReplayDoublesHistory_From1500AndSkipsGuest(t *testing.T) {
	played := time.Date(2026, 4, 1, 18, 0, 0, 0, time.UTC)
	events := []doublesEvent{{
		Sport: "PADEL", PlayedAt: played, MatchID: "m1",
		SideA: [2]string{"a", "b"}, SideB: [2]string{"c", "d"}, AWon: true,
	}, {
		Sport: "PADEL", PlayedAt: played, MatchID: "guest",
		SideA: [2]string{"a", ""}, SideB: [2]string{"c", "d"}, AWon: true,
	}}
	history, finals := replayDoublesHistory(events)
	if len(history) != 4 {
		t.Fatalf("history=%d", len(history))
	}
	if history[0].Rating != 1516 || history[0].Discipline != DisciplineDoubles {
		t.Fatalf("first=%+v", history[0])
	}
	if len(finals) != 4 {
		t.Fatalf("finals=%d", len(finals))
	}
}
