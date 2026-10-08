package score

import (
	"math"
	"strings"
)

const initialRating = 1500

// UserRating is one player's stored Elo for a single sport.
type UserRating struct {
	Sport          string
	Rating         int
	MatchesCounted int
}

func expectedScore(ratingA, ratingB float64) float64 {
	return 1 / (1 + math.Pow(10, (ratingB-ratingA)/400))
}

// kFactor stays wide until the player has 20 rated matches in that sport.
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
	bySport := map[string]int{}
	for _, row := range rows {
		bySport[row.Sport] = row.Rating
	}
	for i := range stats.BySport {
		if rating, ok := bySport[stats.BySport[i].Sport]; ok {
			stats.BySport[i].Rating = rating
		} else {
			stats.BySport[i].Rating = initialRating
		}
	}
	if stats.Sport != nil {
		if rating, ok := bySport[*stats.Sport]; ok {
			stats.Rating = rating
		} else {
			stats.Rating = initialRating
		}
	}
	return stats
}
