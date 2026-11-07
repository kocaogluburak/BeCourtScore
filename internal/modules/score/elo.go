package score

import (
	"math"
	"strings"
)

const initialRating = 1500

const (
	DisciplineSingles = "SINGLES"
	DisciplineDoubles = "DOUBLES"
)

// UserRating is one player's stored Elo for a sport and discipline.
type UserRating struct {
	Sport          string
	Discipline     string
	Rating         int
	MatchesCounted int
}

func isSinglesDiscipline(discipline string) bool {
	return discipline == "" || discipline == DisciplineSingles
}

func expectedScore(ratingA, ratingB float64) float64 {
	return 1 / (1 + math.Pow(10, (ratingB-ratingA)/400))
}

// kFactor stays wide until the player has 20 rated matches in that sport
// and discipline.
func kFactor(matchesCounted int) int {
	if matchesCounted < 20 {
		return 32
	}
	return 16
}

// nextRatings applies one singles result. aWon is from player A's perspective.
// K is chosen from each player's count before this match.
func nextRatings(ratingA, ratingB, countedA, countedB int, aWon bool) (int, int) {
	ea := expectedScore(float64(ratingA), float64(ratingB))
	eb := 1 - ea
	scoreA, scoreB := 0.0, 0.0
	if aWon {
		scoreA = 1
	} else {
		scoreB = 1
	}
	nextA := int(math.Round(float64(ratingA) + float64(kFactor(countedA))*(scoreA-ea)))
	nextB := int(math.Round(float64(ratingB) + float64(kFactor(countedB))*(scoreB-eb)))
	return nextA, nextB
}

// nextDoublesRatings applies one 2v2 result. Team strength is the average of
// the two players' doubles ratings. Each player moves from their own rating
// with their own K. aWon is from side A's perspective.
func nextDoublesRatings(sideA, sideB [2]lockedRating, aWon bool) (nextA, nextB [2]int) {
	teamA := (float64(sideA[0].rating) + float64(sideA[1].rating)) / 2
	teamB := (float64(sideB[0].rating) + float64(sideB[1].rating)) / 2
	ea := expectedScore(teamA, teamB)
	eb := 1 - ea
	scoreA, scoreB := 0.0, 1.0
	if aWon {
		scoreA, scoreB = 1, 0
	}
	for i := 0; i < 2; i++ {
		nextA[i] = int(math.Round(float64(sideA[i].rating) + float64(kFactor(sideA[i].counted))*(scoreA-ea)))
		nextB[i] = int(math.Round(float64(sideB[i].rating) + float64(kFactor(sideB[i].counted))*(scoreB-eb)))
	}
	return nextA, nextB
}

// singlesEloPair is the winner and loser when both sides are exactly one registered user.
// Guests, doubles, and matches without a winner side are skipped.
func singlesEloPair(m Match) (winnerID, loserID string, ok bool) {
	if m.PlayerAUserID == nil || m.PlayerBUserID == nil {
		return "", "", false
	}
	a := strings.TrimSpace(*m.PlayerAUserID)
	b := strings.TrimSpace(*m.PlayerBUserID)
	if a == "" || b == "" || a == b {
		return "", "", false
	}
	if hasPartner(m.PlayerA2Name, m.PlayerA2UserID) || hasPartner(m.PlayerB2Name, m.PlayerB2UserID) {
		return "", "", false
	}
	switch m.WinnerSide {
	case "A":
		return a, b, true
	case "B":
		return b, a, true
	default:
		return "", "", false
	}
}

// doublesEloSides is the two registered pairs when all four slots are distinct
// users. A guest name, a missing id, or a repeated id skips doubles Elo.
func doublesEloSides(m Match) (sideA, sideB [2]string, aWon bool, ok bool) {
	a1 := strings.TrimSpace(derefID(m.PlayerAUserID))
	a2 := strings.TrimSpace(derefID(m.PlayerA2UserID))
	b1 := strings.TrimSpace(derefID(m.PlayerBUserID))
	b2 := strings.TrimSpace(derefID(m.PlayerB2UserID))
	if !distinctIDs(a1, a2, b1, b2) {
		return sideA, sideB, false, false
	}
	switch m.WinnerSide {
	case "A":
		aWon = true
	case "B":
		aWon = false
	default:
		return sideA, sideB, false, false
	}
	return [2]string{a1, a2}, [2]string{b1, b2}, aWon, true
}

func distinctIDs(ids ...string) bool {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if id == "" {
			return false
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func hasPartner(name, id *string) bool {
	if id != nil && strings.TrimSpace(*id) != "" {
		return true
	}
	if name != nil && strings.TrimSpace(*name) != "" {
		return true
	}
	return false
}

func attachRatings(stats UserStats, rows []UserRating) UserStats {
	singles := map[string]int{}
	doubles := map[string]int{}
	for _, row := range rows {
		if isSinglesDiscipline(row.Discipline) {
			singles[row.Sport] = row.Rating
		} else if row.Discipline == DisciplineDoubles {
			doubles[row.Sport] = row.Rating
		}
	}
	for i := range stats.BySport {
		sport := stats.BySport[i].Sport
		if rating, ok := singles[sport]; ok {
			stats.BySport[i].Rating = rating
		} else {
			stats.BySport[i].Rating = initialRating
		}
		if rating, ok := doubles[sport]; ok {
			stats.BySport[i].DoublesRating = rating
		} else {
			stats.BySport[i].DoublesRating = initialRating
		}
	}
	if stats.Sport != nil {
		sport := *stats.Sport
		if rating, ok := singles[sport]; ok {
			stats.Rating = rating
		} else {
			stats.Rating = initialRating
		}
		if rating, ok := doubles[sport]; ok {
			stats.DoublesRating = rating
		} else {
			stats.DoublesRating = initialRating
		}
	}
	return stats
}
