package tournament

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"courtscore/internal/platform/sse"
)

func isSocial(format string) bool {
	switch format {
	case "AMERICANO", "MEXICANO", "KING_OF_THE_COURT":
		return true
	default:
		return false
	}
}

// socialRoundCount is n-1 when everyone can play, and n when one player sits
// each round so the bye rotates.
func socialRoundCount(n int) int {
	if n < 4 {
		return 0
	}
	if n%2 == 1 {
		return n
	}
	return n - 1
}

// usableCourts never asks for more courts than the field can fill with fours.
func usableCourts(n, requested int) int {
	if requested < 1 {
		requested = 1
	}
	maxCourts := n / 4
	if maxCourts < 1 {
		return 1
	}
	if requested > maxCourts {
		return maxCourts
	}
	return requested
}

// buildAmericano writes the full social night up front. Each round takes a
// rotating window of courts*4 players and pairs them as (0,1) vs (2,3).
// Partners change every round; an odd field leaves one rotating bye.
func buildAmericano(players []string, courts int) []MatchPlan {
	n := len(players)
	courts = usableCourts(n, courts)
	seats := courts * 4
	if seats > n {
		seats = (n / 4) * 4
	}
	if seats < 4 {
		return nil
	}
	var plans []MatchPlan
	for r := 0; r < socialRoundCount(n); r++ {
		window := make([]string, seats)
		for i := 0; i < seats; i++ {
			window[i] = players[(r+i)%n]
		}
		for c := 0; c < seats/4; c++ {
			base := c * 4
			plans = append(plans, MatchPlan{
				Round:     r + 1,
				Position:  c,
				A:         window[base],
				A2:        window[base+1],
				B:         window[base+2],
				B2:        window[base+3],
				Status:    "READY",
				NextRound: -1,
				NextPos:   -1,
			})
		}
	}
	return plans
}

// buildSocialRound pairs a ranked list into courts of four: 1+4 vs 2+3.
// Players past courts*4 sit the round out.
func buildSocialRound(ranked []string, courts, round int) []MatchPlan {
	n := len(ranked)
	courts = usableCourts(n, courts)
	var plans []MatchPlan
	for c := 0; c < courts; c++ {
		base := c * 4
		if base+3 >= n {
			break
		}
		g := ranked[base : base+4]
		plans = append(plans, pairCourt(g, round, c))
	}
	return plans
}

func pairCourt(rankedFour []string, round, position int) MatchPlan {
	return MatchPlan{
		Round:     round,
		Position:  position,
		A:         rankedFour[0],
		A2:        rankedFour[3],
		B:         rankedFour[1],
		B2:        rankedFour[2],
		Status:    "READY",
		NextRound: -1,
		NextPos:   -1,
	}
}

// buildKingRound moves winning pairs up one court and losing pairs down.
// The top court keeps its winners; the bottom court keeps its losers.
// One court does not move anyone. A tied score stays on the same court.
// A player who sat out replaces the lowest-ranked player on the bottom court.
func buildKingRound(matches []TournamentMatch, finishedRound int, standings []Standing, courts int) []MatchPlan {
	var played []TournamentMatch
	for _, m := range matches {
		if m.Round == finishedRound && m.Status == "COMPLETED" {
			played = append(played, m)
		}
	}
	sort.Slice(played, func(i, j int) bool { return played[i].PositionInRound < played[j].PositionInRound })
	if len(played) == 0 {
		return nil
	}
	n := len(played)
	next := make([][]string, n)
	for i, m := range played {
		teamA := teamIDs(m.ParticipantAID, m.ParticipantA2ID)
		teamB := teamIDs(m.ParticipantBID, m.ParticipantB2ID)
		aPts, bPts, ok := 0, 0, false
		if m.ScoreSummary != nil {
			aPts, bPts, ok = parseTeamPoints(*m.ScoreSummary)
		}
		if !ok || aPts == bPts {
			next[i] = append(next[i], teamA...)
			next[i] = append(next[i], teamB...)
			continue
		}
		winners, losers := teamA, teamB
		if bPts > aPts {
			winners, losers = teamB, teamA
		}
		up := i
		if i > 0 {
			up = i - 1
		}
		down := i
		if i < n-1 {
			down = i + 1
		}
		next[up] = append(next[up], winners...)
		next[down] = append(next[down], losers...)
	}
	rank := map[string]int{}
	all := make([]string, 0, len(standings))
	for i, s := range standings {
		rank[s.ParticipantID] = i
		all = append(all, s.ParticipantID)
	}
	rotateSitters(next, all, rank)
	var plans []MatchPlan
	for c, players := range next {
		if len(players) < 4 {
			continue
		}
		sort.Slice(players, func(i, j int) bool { return rank[players[i]] < rank[players[j]] })
		plans = append(plans, pairCourt(players[:4], finishedRound+1, c))
	}
	if courts > 0 && len(plans) > courts {
		plans = plans[:courts]
	}
	return plans
}

// rotateSitters brings a sitting player onto the bottom court, sending the
// lowest-ranked player there to the bench.
func rotateSitters(courts [][]string, all []string, rank map[string]int) {
	if len(courts) == 0 {
		return
	}
	playing := map[string]bool{}
	for _, court := range courts {
		for _, id := range court {
			playing[id] = true
		}
	}
	var sitting []string
	for _, id := range all {
		if !playing[id] {
			sitting = append(sitting, id)
		}
	}
	if len(sitting) == 0 {
		return
	}
	sort.Slice(sitting, func(i, j int) bool { return rank[sitting[i]] < rank[sitting[j]] })
	bottom := courts[len(courts)-1]
	if len(bottom) == 0 {
		return
	}
	swaps := len(sitting)
	if swaps > len(bottom) {
		swaps = len(bottom)
	}
	order := append([]string(nil), bottom...)
	sort.Slice(order, func(i, j int) bool { return rank[order[i]] > rank[order[j]] })
	replace := map[string]string{}
	for i := 0; i < swaps; i++ {
		replace[order[i]] = sitting[i]
	}
	for i, id := range bottom {
		if in, ok := replace[id]; ok {
			bottom[i] = in
		}
	}
	courts[len(courts)-1] = bottom
}

func teamIDs(a, b *string) []string {
	var out []string
	if a != nil && *a != "" {
		out = append(out, *a)
	}
	if b != nil && *b != "" {
		out = append(out, *b)
	}
	return out
}

// parseTeamPoints reads a social score "16-12" as team A points and team B points.
func parseTeamPoints(score string) (int, int, bool) {
	score = strings.TrimSpace(score)
	parts := strings.Fields(score)
	if len(parts) != 1 {
		return 0, 0, false
	}
	sides := strings.Split(parts[0], "-")
	if len(sides) != 2 {
		return 0, 0, false
	}
	a, errA := strconv.Atoi(sides[0])
	b, errB := strconv.Atoi(sides[1])
	if errA != nil || errB != nil || a < 0 || b < 0 {
		return 0, 0, false
	}
	return a, b, true
}

// computePointStandings ranks players by points scored, then rounds won.
func computePointStandings(participants []Participant, matches []TournamentMatch) []Standing {
	byID := map[string]*Standing{}
	order := make([]string, 0, len(participants))
	for _, p := range participants {
		byID[p.ID] = &Standing{ParticipantID: p.ID, DisplayName: p.DisplayName}
		order = append(order, p.ID)
	}
	for _, m := range matches {
		if m.Status != "COMPLETED" || m.ScoreSummary == nil {
			continue
		}
		aPts, bPts, ok := parseTeamPoints(*m.ScoreSummary)
		if !ok {
			continue
		}
		creditTeam(byID, teamIDs(m.ParticipantAID, m.ParticipantA2ID), aPts, aPts > bPts, bPts > aPts)
		creditTeam(byID, teamIDs(m.ParticipantBID, m.ParticipantB2ID), bPts, bPts > aPts, aPts > bPts)
	}
	out := make([]Standing, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Points != out[j].Points {
			return out[i].Points > out[j].Points
		}
		if out[i].Won != out[j].Won {
			return out[i].Won > out[j].Won
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}

func creditTeam(byID map[string]*Standing, ids []string, points int, won, lost bool) {
	for _, id := range ids {
		s, ok := byID[id]
		if !ok {
			continue
		}
		s.Points += points
		s.Played++
		if won {
			s.Won++
		}
		if lost {
			s.Lost++
		}
	}
}

func standingIDs(standings []Standing) []string {
	out := make([]string, len(standings))
	for i, s := range standings {
		out[i] = s.ParticipantID
	}
	return out
}

func socialWinner(d matchDetail, score string) (string, bool) {
	aPts, bPts, ok := parseTeamPoints(score)
	if !ok {
		return "", false
	}
	if aPts > bPts {
		if d.ParticipantAID == nil {
			return "", false
		}
		return *d.ParticipantAID, true
	}
	if bPts > aPts {
		if d.ParticipantBID == nil {
			return "", false
		}
		return *d.ParticipantBID, true
	}
	return "", true
}

func roundSettled(matches []TournamentMatch, round int) bool {
	seen := false
	for _, m := range matches {
		if m.Round != round {
			continue
		}
		seen = true
		if m.Status != "COMPLETED" && m.Status != "BYE" {
			return false
		}
	}
	return seen
}

func hasRound(matches []TournamentMatch, round int) bool {
	for _, m := range matches {
		if m.Round == round {
			return true
		}
	}
	return false
}

func (s *Service) advanceSocial(ctx context.Context, d matchDetail) error {
	matches, err := s.store.bracketMatches(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	round := 0
	for _, m := range matches {
		if m.ID == d.ID {
			round = m.Round
			break
		}
	}
	if round == 0 || !roundSettled(matches, round) {
		return nil
	}
	t, err := s.store.getByID(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	parts, err := s.store.confirmedParticipants(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	target := socialRoundCount(len(parts))
	if (d.Format == "MEXICANO" || d.Format == "KING_OF_THE_COURT") && round < target && !hasRound(matches, round+1) {
		standings := computePointStandings(parts, matches)
		var next []MatchPlan
		if d.Format == "MEXICANO" {
			next = buildSocialRound(standingIDs(standings), t.CourtCount, round+1)
		} else {
			next = buildKingRound(matches, round, standings, t.CourtCount)
		}
		if len(next) > 0 {
			if err := s.store.appendFixtures(ctx, d.TournamentID, next); err != nil {
				return err
			}
		}
	}
	remaining, err := s.store.remainingMatches(ctx, d.TournamentID)
	if err != nil {
		return err
	}
	if remaining > 0 {
		return nil
	}
	standings := computePointStandings(parts, matches)
	if len(standings) == 0 {
		return nil
	}
	champ := standings[0].ParticipantID
	s.publishAll(ctx, d.TournamentID, sse.Event{
		Type: "tournament.completed",
		Data: map[string]any{"tournament_id": d.TournamentID, "champion_id": champ},
	})
	return s.store.setChampion(ctx, d.TournamentID, champ)
}
