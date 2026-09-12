package score

import "time"

const formLimit = 5
const rivalsLimit = 5

// UserStats is the career / form summary for a registered participant.
type UserStats struct {
	UserID      string         `json:"user_id"`
	Sport       *string        `json:"sport"`
	Matches     int            `json:"matches"`
	Wins        int            `json:"wins"`
	Losses      int            `json:"losses"`
	SetsFor     int            `json:"sets_for"`
	SetsAgainst int            `json:"sets_against"`
	SetDiff     int            `json:"set_diff"`
	Form        []string       `json:"form"`
	Rivals      []RivalSummary `json:"rivals"`
	BySport     []SportStats   `json:"by_sport"`
}

// SportStats is per-sport career totals (no form/rivals).
type SportStats struct {
	Sport   string `json:"sport"`
	Matches int    `json:"matches"`
	Wins    int    `json:"wins"`
	Losses  int    `json:"losses"`
	SetDiff int    `json:"set_diff"`
}

// RivalSummary is one registered opponent ranked by match count.
type RivalSummary struct {
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Matches int    `json:"matches"`
	Wins    int    `json:"wins"`
	Losses  int    `json:"losses"`
}

// HeadToHead is viewer-vs-opponent career summary (viewer perspective).
type HeadToHead struct {
	OpponentUserID string    `json:"opponent_user_id"`
	Sport          *string   `json:"sport"`
	Matches        int       `json:"matches"`
	Wins           int       `json:"wins"`
	Losses         int       `json:"losses"`
	SetsFor        int       `json:"sets_for"`
	SetsAgainst    int       `json:"sets_against"`
	SetDiff        int       `json:"set_diff"`
	Form           []string  `json:"form"`
	LastPlayedAt   *time.Time `json:"last_played_at"`
}

// participantOutcome returns "W" or "L" when userID is a registered player on the match.
// Empty string means the user is not a participant (e.g. recorder-only).
func participantOutcome(m Match, userID string) string {
	side := ""
	if m.PlayerAUserID != nil && *m.PlayerAUserID == userID {
		side = "A"
	} else if m.PlayerBUserID != nil && *m.PlayerBUserID == userID {
		side = "B"
	}
	if side == "" {
		return ""
	}
	if m.WinnerSide == side {
		return "W"
	}
	return "L"
}

func setsForAgainst(m Match, userID string) (forSets, against int, ok bool) {
	if m.PlayerAUserID != nil && *m.PlayerAUserID == userID {
		return m.SetsA, m.SetsB, true
	}
	if m.PlayerBUserID != nil && *m.PlayerBUserID == userID {
		return m.SetsB, m.SetsA, true
	}
	return 0, 0, false
}

func opponentOf(m Match, userID string) (oppID, oppName string, ok bool) {
	if m.PlayerAUserID != nil && *m.PlayerAUserID == userID {
		if m.PlayerBUserID == nil || *m.PlayerBUserID == "" {
			return "", "", false
		}
		return *m.PlayerBUserID, m.PlayerBName, true
	}
	if m.PlayerBUserID != nil && *m.PlayerBUserID == userID {
		if m.PlayerAUserID == nil || *m.PlayerAUserID == "" {
			return "", "", false
		}
		return *m.PlayerAUserID, m.PlayerAName, true
	}
	return "", "", false
}

// aggregateUserStats builds career stats from participant matches (newest first preferred for form).
// Matches where userID is not a registered player are ignored.
func aggregateUserStats(userID string, sportFilter string, matches []Match) UserStats {
	var sportPtr *string
	if sportFilter != "" {
		s := sportFilter
		sportPtr = &s
	}
	out := UserStats{
		UserID:  userID,
		Sport:   sportPtr,
		Form:    []string{},
		Rivals:  []RivalSummary{},
		BySport: []SportStats{},
	}

	bySport := map[string]*SportStats{}
	rivals := map[string]*RivalSummary{}

	for _, m := range matches {
		outcome := participantOutcome(m, userID)
		if outcome == "" {
			continue
		}
		sf, sa, ok := setsForAgainst(m, userID)
		if !ok {
			continue
		}

		includeInTotals := sportFilter == "" || m.Sport == sportFilter
		if includeInTotals {
			out.Matches++
			if outcome == "W" {
				out.Wins++
			} else {
				out.Losses++
			}
			out.SetsFor += sf
			out.SetsAgainst += sa
			if len(out.Form) < formLimit {
				out.Form = append(out.Form, outcome)
			}
			if oppID, oppName, hasOpp := opponentOf(m, userID); hasOpp {
				r, exists := rivals[oppID]
				if !exists {
					r = &RivalSummary{UserID: oppID, Name: oppName}
					rivals[oppID] = r
				}
				r.Matches++
				if outcome == "W" {
					r.Wins++
				} else {
					r.Losses++
				}
				// Prefer non-empty display name if later rows refine it.
				if oppName != "" {
					r.Name = oppName
				}
			}
		}

		ss, exists := bySport[m.Sport]
		if !exists {
			ss = &SportStats{Sport: m.Sport}
			bySport[m.Sport] = ss
		}
		ss.Matches++
		if outcome == "W" {
			ss.Wins++
		} else {
			ss.Losses++
		}
		ss.SetDiff += sf - sa
	}

	out.SetDiff = out.SetsFor - out.SetsAgainst

	// Stable sport order for clients.
	for _, sport := range []string{"TENNIS", "PADEL", "SQUASH", "PING_PONG"} {
		if ss, ok := bySport[sport]; ok && ss.Matches > 0 {
			out.BySport = append(out.BySport, *ss)
		}
	}

	// Top rivals by matches desc, then wins desc, then user_id.
	type rivalPair struct {
		id string
		r  RivalSummary
	}
	list := make([]rivalPair, 0, len(rivals))
	for id, r := range rivals {
		list = append(list, rivalPair{id: id, r: *r})
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			a, b := list[i].r, list[j].r
			swap := false
			if b.Matches > a.Matches {
				swap = true
			} else if b.Matches == a.Matches {
				if b.Wins > a.Wins {
					swap = true
				} else if b.Wins == a.Wins && list[j].id < list[i].id {
					swap = true
				}
			}
			if swap {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	n := rivalsLimit
	if len(list) < n {
		n = len(list)
	}
	for i := 0; i < n; i++ {
		out.Rivals = append(out.Rivals, list[i].r)
	}
	return out
}

// aggregateHeadToHead builds viewer-vs-opponent stats from H2H matches (newest first for form).
func aggregateHeadToHead(viewerID, opponentID, sportFilter string, matches []Match) HeadToHead {
	var sportPtr *string
	if sportFilter != "" {
		s := sportFilter
		sportPtr = &s
	}
	out := HeadToHead{
		OpponentUserID: opponentID,
		Sport:          sportPtr,
		Form:           []string{},
	}
	for _, m := range matches {
		if sportFilter != "" && m.Sport != sportFilter {
			continue
		}
		outcome := participantOutcome(m, viewerID)
		if outcome == "" {
			continue
		}
		sf, sa, ok := setsForAgainst(m, viewerID)
		if !ok {
			continue
		}
		out.Matches++
		if outcome == "W" {
			out.Wins++
		} else {
			out.Losses++
		}
		out.SetsFor += sf
		out.SetsAgainst += sa
		if len(out.Form) < formLimit {
			out.Form = append(out.Form, outcome)
		}
		t := m.PlayedAt
		if out.LastPlayedAt == nil || t.After(*out.LastPlayedAt) {
			out.LastPlayedAt = &t
		}
	}
	out.SetDiff = out.SetsFor - out.SetsAgainst
	return out
}
