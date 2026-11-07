package score

import (
	"fmt"
	"time"
)

const (
	wrappedMinMatches = 5
	wrappedMinYear    = 2023
)

var wrappedSportOrder = []string{"TENNIS", "PADEL", "SQUASH", "PING_PONG", "PICKLEBALL"}

// WrappedReport is the year-in-review payload for GET /v1/me/wrapped.
// Labels are client-localized; this DTO is numbers and stable enums only.
type WrappedReport struct {
	UserID            string                 `json:"user_id"`
	Year              int                    `json:"year"`
	GeneratedAt       time.Time              `json:"generated_at"`
	HasEnoughData     bool                   `json:"has_enough_data"`
	TotalMatches      int                    `json:"total_matches"`
	Wins              int                    `json:"wins"`
	Losses            int                    `json:"losses"`
	WinRate           float64                `json:"win_rate"`
	SetsFor           int                    `json:"sets_for"`
	SetsAgainst       int                    `json:"sets_against"`
	FavoriteSport     string                 `json:"favorite_sport,omitempty"`
	BySport           []SportStats           `json:"by_sport"`
	BiggestRival      *RivalSummary          `json:"biggest_rival,omitempty"`
	LongestMatch      *WrappedMatchHighlight `json:"longest_match,omitempty"`
	BusiestMonth      *WrappedMonth          `json:"busiest_month,omitempty"`
	MonthlyMatches    []int                  `json:"monthly_matches"`
	CurrentStreak     *WrappedStreak         `json:"current_streak,omitempty"`
	LongestWinStreak  int                    `json:"longest_win_streak"`
	EloPeak           *WrappedElo            `json:"elo_peak,omitempty"`
	EloYearEnd        *WrappedElo            `json:"elo_year_end,omitempty"`
	FirstMatchOfYear  *WrappedFirstMatch     `json:"first_match_of_year,omitempty"`
	SportsPlayedCount int                    `json:"sports_played_count"`
	NewSportsTried    []string               `json:"new_sports_tried"`
}

// WrappedMatchHighlight is the longest battle of the year.
type WrappedMatchHighlight struct {
	MatchID         string    `json:"match_id"`
	Sport           string    `json:"sport"`
	DurationSeconds int       `json:"duration_seconds"`
	PlayedAt        time.Time `json:"played_at"`
	OpponentName    string    `json:"opponent_name"`
}

// WrappedMonth is the calendar month (1–12) with the most matches.
type WrappedMonth struct {
	Month   int `json:"month"`
	Matches int `json:"matches"`
}

// WrappedStreak is the open streak at year end (from the newest match back).
type WrappedStreak struct {
	Type   string `json:"type"`
	Length int    `json:"length"`
}

// WrappedElo is a rating snapshot. PlayedAt is set for peaks only.
type WrappedElo struct {
	Sport    string     `json:"sport"`
	Rating   int        `json:"rating"`
	PlayedAt *time.Time `json:"played_at,omitempty"`
}

// WrappedFirstMatch is the earliest participant match in the year window.
type WrappedFirstMatch struct {
	PlayedAt     time.Time `json:"played_at"`
	OpponentName string    `json:"opponent_name"`
}

// SportFirst is the career-first participant match for one sport.
type SportFirst struct {
	Sport    string
	PlayedAt time.Time
}

func wrappedYearWindow(year int, now time.Time) (from, to time.Time, err error) {
	current := now.UTC().Year()
	if year < wrappedMinYear || year > current {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: year must be between %d and %d", ErrInvalid, wrappedMinYear, current)
	}
	from = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	to = time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
	return from, to, nil
}

func inHalfOpen(t, from, to time.Time) bool {
	return !t.Before(from) && t.Before(to)
}

func opponentDisplayName(m Match, userID string) string {
	opps := opponentsOf(m, userID)
	if len(opps) == 1 {
		if opps[0].name != "" {
			return opps[0].name
		}
	}
	if len(opps) > 1 {
		a, b := opps[0].name, opps[1].name
		switch {
		case a != "" && b != "":
			return a + " / " + b
		case a != "":
			return a
		case b != "":
			return b
		}
	}
	switch sideOf(m, userID) {
	case "A":
		return m.PlayerBName
	case "B":
		return m.PlayerAName
	default:
		return ""
	}
}

func pickEloYearEnd(favorite string, ratings []UserRating) *WrappedElo {
	bySport := map[string]int{}
	for _, row := range ratings {
		if !isSinglesDiscipline(row.Discipline) {
			continue
		}
		bySport[row.Sport] = row.Rating
	}
	if favorite != "" {
		if rating, ok := bySport[favorite]; ok {
			return &WrappedElo{Sport: favorite, Rating: rating}
		}
		return &WrappedElo{Sport: favorite, Rating: initialRating}
	}
	var best *WrappedElo
	for _, sport := range wrappedSportOrder {
		rating, ok := bySport[sport]
		if !ok {
			continue
		}
		if best == nil || rating > best.Rating {
			s, r := sport, rating
			best = &WrappedElo{Sport: s, Rating: r}
		}
	}
	return best
}

func pickEloPeak(rows []WrappedElo) *WrappedElo {
	var best *WrappedElo
	for i := range rows {
		row := rows[i]
		if best == nil || row.Rating > best.Rating {
			copy := row
			best = &copy
		}
	}
	return best
}

// aggregateWrapped builds a year report from already year-scoped matches.
// yearMatches should be newest-first (same as listParticipantMatchesInRange).
// firsts are career-first participant matches per sport (any year).
func aggregateWrapped(
	userID string,
	year int,
	yearMatches []Match,
	firsts []SportFirst,
	ratings []UserRating,
	peak *WrappedElo,
	generatedAt time.Time,
) WrappedReport {
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)

	out := WrappedReport{
		UserID:         userID,
		Year:           year,
		GeneratedAt:    generatedAt.UTC(),
		BySport:        []SportStats{},
		MonthlyMatches: make([]int, 12),
		NewSportsTried: []string{},
	}

	career := aggregateUserStats(userID, "", yearMatches)
	out.TotalMatches = career.Matches
	out.Wins = career.Wins
	out.Losses = career.Losses
	out.SetsFor = career.SetsFor
	out.SetsAgainst = career.SetsAgainst
	out.BySport = career.BySport
	if career.Matches > 0 {
		out.WinRate = float64(career.Wins) / float64(career.Matches)
	}
	out.HasEnoughData = career.Matches >= wrappedMinMatches
	out.SportsPlayedCount = len(career.BySport)

	maxMatches := 0
	for _, sport := range wrappedSportOrder {
		for _, ss := range career.BySport {
			if ss.Sport != sport {
				continue
			}
			if ss.Matches > maxMatches {
				maxMatches = ss.Matches
				out.FavoriteSport = sport
			}
		}
	}
	if len(career.Rivals) > 0 {
		r := career.Rivals[0]
		out.BiggestRival = &r
	}

	var first *Match
	outcomesChrono := []string{} // oldest → newest
	for i := len(yearMatches) - 1; i >= 0; i-- {
		m := yearMatches[i]
		if participantOutcome(m, userID) == "" {
			continue
		}
		if first == nil || m.PlayedAt.Before(first.PlayedAt) {
			copy := m
			first = &copy
		}
		outcomesChrono = append(outcomesChrono, participantOutcome(m, userID))
		month := int(m.PlayedAt.UTC().Month())
		if month >= 1 && month <= 12 {
			out.MonthlyMatches[month-1]++
		}
		if m.DurationSeconds != nil && *m.DurationSeconds > 0 {
			if out.LongestMatch == nil || *m.DurationSeconds > out.LongestMatch.DurationSeconds {
				out.LongestMatch = &WrappedMatchHighlight{
					MatchID:         m.ID,
					Sport:           m.Sport,
					DurationSeconds: *m.DurationSeconds,
					PlayedAt:        m.PlayedAt,
					OpponentName:    opponentDisplayName(m, userID),
				}
			}
		}
	}
	if first != nil {
		out.FirstMatchOfYear = &WrappedFirstMatch{
			PlayedAt:     first.PlayedAt,
			OpponentName: opponentDisplayName(*first, userID),
		}
	}

	busiestMonth, busiestCount := 0, 0
	for i, n := range out.MonthlyMatches {
		if n > busiestCount {
			busiestCount = n
			busiestMonth = i + 1
		}
	}
	if busiestCount > 0 {
		out.BusiestMonth = &WrappedMonth{Month: busiestMonth, Matches: busiestCount}
	}

	run := 0
	for _, outcome := range outcomesChrono {
		if outcome == "W" {
			run++
			if run > out.LongestWinStreak {
				out.LongestWinStreak = run
			}
		} else {
			run = 0
		}
	}
	if len(outcomesChrono) > 0 {
		last := outcomesChrono[len(outcomesChrono)-1]
		length := 0
		for i := len(outcomesChrono) - 1; i >= 0; i-- {
			if outcomesChrono[i] != last {
				break
			}
			length++
		}
		out.CurrentStreak = &WrappedStreak{Type: last, Length: length}
	}

	firstBySport := map[string]time.Time{}
	for _, f := range firsts {
		firstBySport[f.Sport] = f.PlayedAt
	}
	for _, sport := range wrappedSportOrder {
		t, ok := firstBySport[sport]
		if !ok {
			continue
		}
		if inHalfOpen(t.UTC(), from, to) {
			out.NewSportsTried = append(out.NewSportsTried, sport)
		}
	}

	out.EloYearEnd = pickEloYearEnd(out.FavoriteSport, ratings)
	out.EloPeak = peak
	return out
}
