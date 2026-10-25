package achievements

import (
	"sort"
	"time"
)

// SetLine is one set from the subject's perspective (For / Against).
type SetLine struct {
	For     int
	Against int
}

// CountedMatch is a finished match where the subject is a registered participant.
type CountedMatch struct {
	ID        string
	Sport     string
	PlayedAt  time.Time
	Won       bool
	SetScores []SetLine
}

// EvalInput is the snapshot evaluators read. Pure — no I/O.
type EvalInput struct {
	Matches        []CountedMatch
	MaxRating      int
	ChampionCount  int
	CurrentWeeks   int
	LongestWeeks   int
	WeeklyTarget   int
}

// EarnedBadges returns every catalog code whose unlock rule is met.
func EarnedBadges(in EvalInput) []string {
	n := len(in.Matches)
	wins := countWins(in.Matches)
	winStreak := maxWinStreak(in.Matches)
	tbWins := countTiebreakWins(in.Matches)
	goalWeeks := weeksGoalHit(in.Matches, in.WeeklyTarget)
	longest := in.LongestWeeks
	if in.CurrentWeeks > longest {
		longest = in.CurrentWeeks
	}

	var out []string
	if n >= 1 {
		out = append(out, BadgeFirstMatch)
	}
	if wins >= 1 {
		out = append(out, BadgeFirstWin)
	}
	if n >= 10 {
		out = append(out, BadgeMatches10)
	}
	if n >= 50 {
		out = append(out, BadgeMatches50)
	}
	if n >= 100 {
		out = append(out, BadgeMatches100)
	}
	if winStreak >= 5 {
		out = append(out, BadgeWinStreak5)
	}
	if in.MaxRating >= 1600 {
		out = append(out, BadgeElo1600)
	}
	if in.ChampionCount >= 1 {
		out = append(out, BadgeTournamentWin1)
	}
	if tbWins >= 10 {
		out = append(out, BadgeTiebreakMaster)
	}
	if longest >= 4 {
		out = append(out, BadgeWeeksStreak4)
	}
	if longest >= 12 {
		out = append(out, BadgeWeeksStreak12)
	}
	if goalWeeks >= 4 {
		out = append(out, BadgeGoalCrusher)
	}
	return out
}

func countWins(matches []CountedMatch) int {
	n := 0
	for _, m := range matches {
		if m.Won {
			n++
		}
	}
	return n
}

// maxWinStreak is the longest run of chronological wins (any sport). A loss resets.
func maxWinStreak(matches []CountedMatch) int {
	ordered := append([]CountedMatch(nil), matches...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].PlayedAt.Equal(ordered[j].PlayedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].PlayedAt.Before(ordered[j].PlayedAt)
	})
	best, cur := 0, 0
	for _, m := range ordered {
		if m.Won {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

// isTiebreakSet is a 7–6 set or a deciding-set tiebreak (winner ≥ 10, loser ≥ 8).
func isTiebreakSet(forGames, against int) bool {
	hi, lo := forGames, against
	if lo > hi {
		hi, lo = lo, hi
	}
	if hi == 7 && lo == 6 {
		return true
	}
	if hi >= 10 && lo >= 8 {
		return true
	}
	return false
}

func countTiebreakWins(matches []CountedMatch) int {
	n := 0
	for _, m := range matches {
		for _, s := range m.SetScores {
			if s.For > s.Against && isTiebreakSet(s.For, s.Against) {
				n++
			}
		}
	}
	return n
}

// weeksGoalHit counts distinct ISO weeks (UTC) where the subject played at
// least `target` counted matches. target ≤ 0 means no goal.
func weeksGoalHit(matches []CountedMatch, target int) int {
	if target <= 0 {
		return 0
	}
	counts := map[time.Time]int{}
	for _, m := range matches {
		counts[ISOWeekStart(m.PlayedAt)]++
	}
	n := 0
	for _, c := range counts {
		if c >= target {
			n++
		}
	}
	return n
}

func matchesInWeek(matches []CountedMatch, weekStart time.Time) int {
	end := weekStart.AddDate(0, 0, 7)
	n := 0
	for _, m := range matches {
		t := m.PlayedAt.UTC()
		if !t.Before(weekStart) && t.Before(end) {
			n++
		}
	}
	return n
}
