package achievements

import (
	"testing"
	"time"
)

func match(id string, won bool, at time.Time, sets ...SetLine) CountedMatch {
	return CountedMatch{ID: id, Sport: "TENNIS", PlayedAt: at, Won: won, SetScores: sets}
}

func TestEarnedBadges_MatchCountBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	nine := make([]CountedMatch, 9)
	for i := range nine {
		nine[i] = match("m"+itoa(i), false, now.AddDate(0, 0, -i))
	}
	got := EarnedBadges(EvalInput{Matches: nine})
	if !contains(got, BadgeFirstMatch) || contains(got, BadgeMatches10) {
		t.Fatalf("9 matches: %v", got)
	}
	ten := append(nine, match("m9", false, now.AddDate(0, 0, -9)))
	got = EarnedBadges(EvalInput{Matches: ten})
	if !contains(got, BadgeMatches10) {
		t.Fatalf("10 matches missing matches_10: %v", got)
	}
}

func TestEarnedBadges_WinStreakBoundary(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	four := []CountedMatch{
		match("1", true, now.AddDate(0, 0, -4)),
		match("2", true, now.AddDate(0, 0, -3)),
		match("3", true, now.AddDate(0, 0, -2)),
		match("4", true, now.AddDate(0, 0, -1)),
	}
	if contains(EarnedBadges(EvalInput{Matches: four}), BadgeWinStreak5) {
		t.Fatal("4 wins should not unlock win_streak_5")
	}
	five := append(four, match("5", true, now))
	if !contains(EarnedBadges(EvalInput{Matches: five}), BadgeWinStreak5) {
		t.Fatal("5 wins should unlock")
	}
	reset := append(five, match("6", false, now.Add(time.Hour)))
	if !contains(EarnedBadges(EvalInput{Matches: reset}), BadgeWinStreak5) {
		t.Fatal("historical 5-streak still counts after a later loss")
	}
}

func TestEarnedBadges_EloBoundary(t *testing.T) {
	if contains(EarnedBadges(EvalInput{Matches: []CountedMatch{match("1", true, time.Now())}, MaxRating: 1599}), BadgeElo1600) {
		t.Fatal("1599 should not unlock")
	}
	if !contains(EarnedBadges(EvalInput{Matches: []CountedMatch{match("1", true, time.Now())}, MaxRating: 1600}), BadgeElo1600) {
		t.Fatal("1600 should unlock")
	}
}

func TestEarnedBadges_TiebreakBoundary(t *testing.T) {
	now := time.Now()
	tb := SetLine{For: 7, Against: 6}
	nine := make([]CountedMatch, 9)
	for i := range nine {
		nine[i] = match("t"+itoa(i), true, now, tb)
	}
	if contains(EarnedBadges(EvalInput{Matches: nine}), BadgeTiebreakMaster) {
		t.Fatal("9 TB sets should not unlock")
	}
	ten := append(nine, match("t9", true, now, tb))
	if !contains(EarnedBadges(EvalInput{Matches: ten}), BadgeTiebreakMaster) {
		t.Fatal("10 TB sets should unlock")
	}
	lostTB := []CountedMatch{match("x", true, now, SetLine{For: 6, Against: 7})}
	if countTiebreakWins(lostTB) != 0 {
		t.Fatal("lost 6-7 is not a won tiebreak set")
	}
}

func TestEarnedBadges_RecorderOnlyIgnored(t *testing.T) {
	// Empty slice = no counted participant matches (recorder-only rows never enter CountedMatch).
	got := EarnedBadges(EvalInput{})
	if len(got) != 0 {
		t.Fatalf("empty input unlocked %v", got)
	}
}

func TestEarnedBadges_StreakAndGoalAndTournament(t *testing.T) {
	got := EarnedBadges(EvalInput{LongestWeeks: 4, ChampionCount: 1, WeeklyTarget: 2})
	if !contains(got, BadgeWeeksStreak4) || contains(got, BadgeWeeksStreak12) {
		t.Fatalf("week streak: %v", got)
	}
	if !contains(got, BadgeTournamentWin1) {
		t.Fatal("missing tournament_win_1")
	}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	var matches []CountedMatch
	for w := 0; w < 4; w++ {
		week := now.AddDate(0, 0, w*7)
		matches = append(matches, match("a"+itoa(w), true, week), match("b"+itoa(w), true, week.Add(time.Hour)))
	}
	got = EarnedBadges(EvalInput{Matches: matches, WeeklyTarget: 2})
	if !contains(got, BadgeGoalCrusher) {
		t.Fatalf("goal crusher: %v", got)
	}
	got = EarnedBadges(EvalInput{Matches: matches, WeeklyTarget: 3})
	if contains(got, BadgeGoalCrusher) {
		t.Fatal("target 3 should not count 2-match weeks")
	}
}

func contains(codes []string, want string) bool {
	for _, c := range codes {
		if c == want {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	if i < 0 {
		return "n"
	}
	const digits = "0123456789"
	if i < 10 {
		return string(digits[i])
	}
	return itoa(i/10) + string(digits[i%10])
}
