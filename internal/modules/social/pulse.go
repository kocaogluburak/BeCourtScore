package social

import (
	"context"
	"sort"
	"strings"
	"time"
)

// PulseRank is the one ladder row the weekly card shows.
type PulseRank struct {
	GroupID   string
	GroupName string
	Rank      int
	Points    int
}

// PulseFriends is the card's friend line: up to three names, then a remainder.
type PulseFriends struct {
	Names  []string
	Others int
}

// pulseGroup is one group the player still belongs to, with its current members.
type pulseGroup struct {
	ID      string
	Name    string
	Sport   string
	Members []GroupMember
}

// pulseFriend is one accepted friend who finished a match this week.
type pulseFriend struct {
	UserID   string
	Name     string
	PlayedAt time.Time
}

// pulseStore loads the fixed query pack for the weekly card. *repo implements it.
type pulseStore interface {
	loadPulseGroups(ctx context.Context, userID string) ([]pulseGroup, error)
	loadPulseMatches(ctx context.Context, memberIDs, sports []string, start, end time.Time) ([]ladderMatch, error)
	loadPulseNights(ctx context.Context, groupIDs []string, start, end time.Time) ([]socialNight, error)
	loadPulseFriends(ctx context.Context, userID string, start, end time.Time) ([]pulseFriend, error)
	matchAudience(ctx context.Context, participantIDs []string) ([]string, error)
	nightAudience(ctx context.Context, playerIDs []string, groupID string) ([]string, error)
}

// MatchAudience is the one set that should refresh after a finished match:
// registered participants, accepted friends who have not opted out, and
// current members of groups that include a participant.
func (s *Service) MatchAudience(ctx context.Context, participantIDs []string) ([]string, error) {
	if s.pulse == nil {
		return nil, nil
	}
	return s.pulse.matchAudience(ctx, participantIDs)
}

// NightAudience refreshes the players of a finished social night and the
// group members whose ladder can move. Friends are not included.
func (s *Service) NightAudience(ctx context.Context, playerIDs []string, groupID string) ([]string, error) {
	if s.pulse == nil {
		return nil, nil
	}
	return s.pulse.nightAudience(ctx, playerIDs, groupID)
}

// assembleAudience unions ids, dropping blanks and duplicates.
func assembleAudience(groups ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range groups {
		for _, id := range group {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// WeeklyRank picks the player's ladder row for the open ISO week.
// Points below 1 yield nil. Several scoring groups keep the latest scoring event.
func (s *Service) WeeklyRank(ctx context.Context, userID string, start, end time.Time) (*PulseRank, error) {
	if s.pulse == nil {
		return nil, nil
	}
	groups, err := s.pulse.loadPulseGroups(ctx, userID)
	if err != nil || len(groups) == 0 {
		return nil, err
	}
	memberIDs, sports, groupIDs := pulseQueryKeys(groups)
	matches, err := s.pulse.loadPulseMatches(ctx, memberIDs, sports, start, end)
	if err != nil {
		return nil, err
	}
	nights, err := s.pulse.loadPulseNights(ctx, groupIDs, start, end)
	if err != nil {
		return nil, err
	}
	return selectPulseRank(userID, groups, matches, nights, start, end), nil
}

// WeeklyFriends lists accepted friends who finished a match this week.
// Social nights are not a source. An empty list is nil.
func (s *Service) WeeklyFriends(ctx context.Context, userID string, start, end time.Time) (*PulseFriends, error) {
	if s.pulse == nil {
		return nil, nil
	}
	rows, err := s.pulse.loadPulseFriends(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}
	return selectPulseFriends(userID, rows), nil
}

func pulseQueryKeys(groups []pulseGroup) (memberIDs, sports, groupIDs []string) {
	seenMember := map[string]bool{}
	seenSport := map[string]bool{}
	for _, g := range groups {
		groupIDs = append(groupIDs, g.ID)
		if g.Sport != "" && !seenSport[g.Sport] {
			seenSport[g.Sport] = true
			sports = append(sports, g.Sport)
		}
		for _, m := range g.Members {
			if m.UserID != "" && !seenMember[m.UserID] {
				seenMember[m.UserID] = true
				memberIDs = append(memberIDs, m.UserID)
			}
		}
	}
	return memberIDs, sports, groupIDs
}

type pulseCandidate struct {
	id, name     string
	rank, points int
	last         time.Time
}

// selectPulseRank uses buildStandings and nightsToPoints. A group with no
// points for the player is dropped. Ties on the last scoring instant keep
// the smaller group id.
func selectPulseRank(userID string, groups []pulseGroup, matches []ladderMatch, nights []socialNight, start, end time.Time) *PulseRank {
	var best *pulseCandidate
	for _, g := range groups {
		roster := make([]seasonMember, len(g.Members))
		memberSet := make(map[string]GroupMember, len(g.Members))
		for i, m := range g.Members {
			roster[i] = seasonMember{UserID: m.UserID, Name: m.Name}
			memberSet[m.UserID] = m
		}
		standings := buildStandings(g.Members, matches, g.Sport, start, end, nightsToPoints(nights, g.ID, roster, start, end))
		var row *Standing
		for i := range standings {
			if standings[i].UserID == userID {
				row = &standings[i]
				break
			}
		}
		if row == nil || row.Points < 1 {
			continue
		}
		c := pulseCandidate{
			id: g.ID, name: g.Name, rank: row.Rank, points: row.Points,
			last: lastPulseEvent(userID, g.ID, memberSet, roster, matches, nights, g.Sport, start, end),
		}
		if best == nil || c.last.After(best.last) || (c.last.Equal(best.last) && c.id < best.id) {
			cp := c
			best = &cp
		}
	}
	if best == nil {
		return nil
	}
	return &PulseRank{GroupID: best.id, GroupName: best.name, Rank: best.rank, Points: best.points}
}

func lastPulseEvent(userID, groupID string, memberSet map[string]GroupMember, roster []seasonMember, matches []ladderMatch, nights []socialNight, sport string, start, end time.Time) time.Time {
	var last time.Time
	for _, match := range matches {
		sideA, sideB, ok := countableSides(match, memberSet, sport, start, end)
		if !ok || (!idIn(sideA, userID) && !idIn(sideB, userID)) {
			continue
		}
		if match.PlayedAt.After(last) {
			last = match.PlayedAt
		}
	}
	for _, n := range nights {
		pts := nightsToPoints([]socialNight{n}, groupID, roster, start, end)
		if pts[userID] < 1 {
			continue
		}
		if n.FinishedAt.After(last) {
			last = n.FinishedAt
		}
	}
	return last
}

func idIn(ids []string, id string) bool {
	for _, other := range ids {
		if other == id {
			return true
		}
	}
	return false
}

// selectPulseFriends orders by latest finished match, drops blank names and
// the viewer, and keeps the first three names plus the remainder count.
func selectPulseFriends(viewerID string, rows []pulseFriend) *PulseFriends {
	kept := make([]pulseFriend, 0, len(rows))
	for _, row := range rows {
		if row.UserID == "" || row.UserID == viewerID {
			continue
		}
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}
		row.Name = name
		kept = append(kept, row)
	}
	if len(kept) == 0 {
		return nil
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].PlayedAt.Equal(kept[j].PlayedAt) {
			return kept[i].UserID < kept[j].UserID
		}
		return kept[i].PlayedAt.After(kept[j].PlayedAt)
	})
	n := 3
	if len(kept) < n {
		n = len(kept)
	}
	names := make([]string, n)
	for i := 0; i < n; i++ {
		names[i] = kept[i].Name
	}
	others := len(kept) - n
	return &PulseFriends{Names: names, Others: others}
}
