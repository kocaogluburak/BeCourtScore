package social

import (
	"crypto/rand"
	"strings"
	"time"
)

// isoWeek is the ISO week containing now, Monday 00:00 UTC inclusive
// through the next Monday exclusive.
func isoWeek(now time.Time) (start, end time.Time) {
	t := now.UTC()
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(wd - 1))
	return start, start.AddDate(0, 0, 7)
}

// ladderMatch is the slice of a finished match the weekly ladder needs.
type ladderMatch struct {
	Sport      string
	PlayedAt   time.Time
	PlayerA    string
	PlayerA2   string
	PlayerB    string
	PlayerB2   string
	SetsA      int
	SetsB      int
	WinnerSide string
}

// buildStandings ranks current members by wins, then set difference,
// for matches of sport played inside [start, end). A match counts only
// when each side has at least one registered player and every registered
// player on both sides is a current member. Guests and non-members drop
// the match. Each registered player on a side receives that side's win
// or loss and the same set difference.
func buildStandings(members []GroupMember, matches []ladderMatch, sport string, start, end time.Time) []Standing {
	memberSet := make(map[string]GroupMember, len(members))
	rows := make([]Standing, 0, len(members))
	index := make(map[string]int, len(members))
	for _, m := range members {
		memberSet[m.UserID] = m
		index[m.UserID] = len(rows)
		rows = append(rows, Standing{
			UserID: m.UserID,
			Name:   m.Name,
		})
	}

	for _, match := range matches {
		if match.Sport != sport || match.PlayedAt.Before(start) || !match.PlayedAt.Before(end) {
			continue
		}
		sideA := sideIDs(match.PlayerA, match.PlayerA2)
		sideB := sideIDs(match.PlayerB, match.PlayerB2)
		if len(sideA) == 0 || len(sideB) == 0 || !allMembers(sideA, memberSet) || !allMembers(sideB, memberSet) {
			continue
		}
		if sharesID(sideA, sideB) {
			continue
		}
		applySide(rows, index, sideA, match.WinnerSide == "A", match.SetsA, match.SetsB)
		applySide(rows, index, sideB, match.WinnerSide == "B", match.SetsB, match.SetsA)
	}

	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if standingLess(rows[j], rows[i]) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

func standingLess(a, b Standing) bool {
	if a.Wins != b.Wins {
		return a.Wins > b.Wins
	}
	if a.SetDiff != b.SetDiff {
		return a.SetDiff > b.SetDiff
	}
	return a.UserID < b.UserID
}

func applySide(rows []Standing, index map[string]int, ids []string, won bool, setsFor, setsAgainst int) {
	for _, id := range ids {
		i, ok := index[id]
		if !ok {
			continue
		}
		rows[i].Matches++
		if won {
			rows[i].Wins++
		} else {
			rows[i].Losses++
		}
		rows[i].SetDiff += setsFor - setsAgainst
	}
}

func sideIDs(ids ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func allMembers(ids []string, members map[string]GroupMember) bool {
	for _, id := range ids {
		if _, ok := members[id]; !ok {
			return false
		}
	}
	return true
}

func sharesID(a, b []string) bool {
	for _, id := range a {
		for _, other := range b {
			if id == other {
				return true
			}
		}
	}
	return false
}

const groupCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomGroupCode(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		for i := range buf {
			buf[i] = groupCodeAlphabet[i%len(groupCodeAlphabet)]
		}
		return string(buf)
	}
	for i := range buf {
		buf[i] = groupCodeAlphabet[int(buf[i])%len(groupCodeAlphabet)]
	}
	return string(buf)
}

func validGroupSport(sport string) bool {
	switch sport {
	case "TENNIS", "PADEL", "SQUASH", "PING_PONG":
		return true
	default:
		return false
	}
}

func memberDisplayName(nickname, name, surname *string) string {
	var parts []string
	if name != nil && strings.TrimSpace(*name) != "" {
		parts = append(parts, strings.TrimSpace(*name))
	}
	if surname != nil && strings.TrimSpace(*surname) != "" {
		parts = append(parts, strings.TrimSpace(*surname))
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if nickname != nil && strings.TrimSpace(*nickname) != "" {
		return strings.TrimSpace(*nickname)
	}
	return ""
}
