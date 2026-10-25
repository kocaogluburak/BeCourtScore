package social

import (
	"fmt"
	"sort"
	"time"
)

const (
	seasonStatusActive    = "ACTIVE"
	seasonStatusCompleted = "COMPLETED"

	outcomeChampion  = "CHAMPION"
	outcomePromoted  = "PROMOTED"
	outcomeRelegated = "RELEGATED"
	outcomeStayed    = "STAYED"

	seasonPointsWin  = 3
	seasonPointsLoss = 1

	targetDivisionSize = 8
	minDivisionForTwo  = 6
	promoRelegateTwo   = 2
	promoRelegateOne   = 1

	minSeasonWeeks     = 6
	maxSeasonWeeks     = 8
	defaultSeasonWeeks = 8
)

// Season is one 6–8 week league window for a group.
type Season struct {
	ID                string    `json:"id"`
	GroupID           string    `json:"group_id"`
	SeasonNumber      int       `json:"season_number"`
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
	DivisionCount     int       `json:"division_count"`
	CountSocialNights bool      `json:"count_social_nights"`
	ChampionUserID    *string   `json:"champion_user_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// SeasonSummary is the list/group-detail projection.
type SeasonSummary struct {
	ID             string    `json:"id"`
	SeasonNumber   int       `json:"season_number"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	DivisionCount  int       `json:"division_count"`
	ChampionUserID *string   `json:"champion_user_id,omitempty"`
}

// SeasonStanding is one player's row inside a division.
type SeasonStanding struct {
	Rank         int    `json:"rank"`
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	Points       int    `json:"points"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	Matches      int    `json:"matches"`
	SetDiff      int    `json:"set_diff"`
	DivisionTier int    `json:"division_tier"`
	Outcome      string `json:"outcome,omitempty"`
}

// Division is one promotion tier with its live or frozen standings.
type Division struct {
	Tier      int              `json:"tier"`
	Name      string           `json:"name"`
	Standings []SeasonStanding `json:"standings"`
}

// SeasonDetail is the full season payload (live or frozen).
type SeasonDetail struct {
	SeasonSummary
	CountSocialNights bool       `json:"count_social_nights"`
	Divisions         []Division `json:"divisions"`
	IsFrozen          bool       `json:"is_frozen"`
}

// SeasonConfig is the owner payload for starting a season.
type SeasonConfig struct {
	Weeks             int
	DivisionCount     int
	CountSocialNights bool
}

// NightPlacement is one social-night player's finishing place.
type NightPlacement struct {
	UserID string
	Rank   int
	Played bool
}

// seasonMember is a player assigned to a division for a season.
type seasonMember struct {
	UserID       string
	Name         string
	DivisionTier int
	JoinedAt     time.Time
}

// socialNight is a completed Americano/Mexicano/KotC attributed to a group.
type socialNight struct {
	TournamentID string
	Format       string
	Status       string
	GroupID      string
	FinishedAt   time.Time
	UserIDs      []string
	Placements   []NightPlacement
}

func seasonSummaryOf(s Season) SeasonSummary {
	return SeasonSummary{
		ID:             s.ID,
		SeasonNumber:   s.SeasonNumber,
		Name:           s.Name,
		Status:         s.Status,
		StartsAt:       s.StartsAt,
		EndsAt:         s.EndsAt,
		DivisionCount:  s.DivisionCount,
		ChampionUserID: s.ChampionUserID,
	}
}

func defaultSeasonName(n int) string {
	return fmt.Sprintf("Season %d", n)
}

func seasonWindow(startsAt time.Time, weeks int) (start, end time.Time) {
	start = startsAt.UTC()
	return start, start.AddDate(0, 0, weeks*7)
}

func normalizeSeasonConfig(cfg SeasonConfig) SeasonConfig {
	if cfg.Weeks == 0 {
		cfg.Weeks = defaultSeasonWeeks
	}
	if cfg.DivisionCount < 1 {
		cfg.DivisionCount = 1
	}
	return cfg
}

func validSeasonWeeks(weeks int) bool {
	return weeks >= minSeasonWeeks && weeks <= maxSeasonWeeks
}

// seedFirstDivisions assigns current members into requested divisions.
// Tier 1 is filled first in join order, ~8 players per division.
func seedFirstDivisions(members []GroupMember, divisionCount int) []seasonMember {
	if divisionCount < 1 {
		divisionCount = 1
	}
	out := make([]seasonMember, 0, len(members))
	if len(members) == 0 {
		return out
	}
	per := (len(members) + divisionCount - 1) / divisionCount
	if per < 1 {
		per = 1
	}
	if per > targetDivisionSize && divisionCount == 1 {
		per = len(members)
	}
	for i, m := range members {
		tier := i/per + 1
		if tier > divisionCount {
			tier = divisionCount
		}
		out = append(out, seasonMember{
			UserID:       m.UserID,
			Name:         m.Name,
			DivisionTier: tier,
			JoinedAt:     m.JoinedAt,
		})
	}
	return out
}

// buildSeasonStandings ranks each division by league points (win 3 / loss 1),
// then set difference, wins, head-to-head, and user id. Cross-division
// friendlies and guests are ignored. nightPoints (from social nights) add
// to points only.
func buildSeasonStandings(members []seasonMember, matches []ladderMatch, sport string, start, end time.Time, nightPoints map[string]int) []SeasonStanding {
	byDiv := map[int][]seasonMember{}
	memberDiv := map[string]int{}
	for _, m := range members {
		byDiv[m.DivisionTier] = append(byDiv[m.DivisionTier], m)
		memberDiv[m.UserID] = m.DivisionTier
	}
	tiers := make([]int, 0, len(byDiv))
	for t := range byDiv {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)

	var out []SeasonStanding
	for _, tier := range tiers {
		out = append(out, rankDivision(tier, byDiv[tier], matches, sport, start, end, memberDiv, nightPoints)...)
	}
	return out
}

func rankDivision(tier int, members []seasonMember, matches []ladderMatch, sport string, start, end time.Time, memberDiv map[string]int, nightPoints map[string]int) []SeasonStanding {
	rows := make([]SeasonStanding, 0, len(members))
	index := make(map[string]int, len(members))
	divSet := make(map[string]struct{}, len(members))
	for _, m := range members {
		divSet[m.UserID] = struct{}{}
		index[m.UserID] = len(rows)
		rows = append(rows, SeasonStanding{
			UserID:       m.UserID,
			Name:         m.Name,
			DivisionTier: tier,
		})
	}
	h2h := map[string]map[string]int{}
	for _, match := range matches {
		if match.Sport != sport || match.PlayedAt.Before(start) || !match.PlayedAt.Before(end) {
			continue
		}
		sideA := sideIDs(match.PlayerA, match.PlayerA2)
		sideB := sideIDs(match.PlayerB, match.PlayerB2)
		if len(sideA) == 0 || len(sideB) == 0 || sharesID(sideA, sideB) {
			continue
		}
		if !allInSet(sideA, divSet) || !allInSet(sideB, divSet) {
			continue
		}
		if !sameDivision(sideA, sideB, memberDiv) {
			continue
		}
		applySeasonSide(rows, index, sideA, match.WinnerSide == "A", match.SetsA, match.SetsB)
		applySeasonSide(rows, index, sideB, match.WinnerSide == "B", match.SetsB, match.SetsA)
		recordHeadToHead(h2h, sideA, sideB, match.WinnerSide == "A")
	}
	for i := range rows {
		rows[i].Points += nightPoints[rows[i].UserID]
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return seasonStandingLess(rows[i], rows[j], h2h)
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

func applySeasonSide(rows []SeasonStanding, index map[string]int, ids []string, won bool, setsFor, setsAgainst int) {
	for _, id := range ids {
		i, ok := index[id]
		if !ok {
			continue
		}
		rows[i].Matches++
		if won {
			rows[i].Wins++
			rows[i].Points += seasonPointsWin
		} else {
			rows[i].Losses++
			rows[i].Points += seasonPointsLoss
		}
		rows[i].SetDiff += setsFor - setsAgainst
	}
}

func seasonStandingLess(a, b SeasonStanding, h2h map[string]map[string]int) bool {
	if a.Points != b.Points {
		return a.Points > b.Points
	}
	if a.SetDiff != b.SetDiff {
		return a.SetDiff > b.SetDiff
	}
	if a.Wins != b.Wins {
		return a.Wins > b.Wins
	}
	if aw, bw := h2hWins(h2h, a.UserID, b.UserID), h2hWins(h2h, b.UserID, a.UserID); aw != bw {
		return aw > bw
	}
	return a.UserID < b.UserID
}

func recordHeadToHead(h2h map[string]map[string]int, sideA, sideB []string, aWon bool) {
	winners, losers := sideA, sideB
	if !aWon {
		winners, losers = sideB, sideA
	}
	for _, w := range winners {
		if h2h[w] == nil {
			h2h[w] = map[string]int{}
		}
		for _, l := range losers {
			h2h[w][l]++
		}
	}
}

func h2hWins(h2h map[string]map[string]int, a, b string) int {
	if h2h == nil || h2h[a] == nil {
		return 0
	}
	return h2h[a][b]
}

func allInSet(ids []string, set map[string]struct{}) bool {
	for _, id := range ids {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func sameDivision(sideA, sideB []string, memberDiv map[string]int) bool {
	tier := 0
	for _, id := range append(append([]string{}, sideA...), sideB...) {
		t, ok := memberDiv[id]
		if !ok {
			return false
		}
		if tier == 0 {
			tier = t
			continue
		}
		if t != tier {
			return false
		}
	}
	return tier != 0
}

// socialNightPoints maps night placement onto league points:
// 1st=3, 2nd=2, 3rd=1, other players who appeared=1.
func socialNightPoints(standings []NightPlacement) map[string]int {
	out := map[string]int{}
	for _, s := range standings {
		if !s.Played || s.UserID == "" {
			continue
		}
		switch s.Rank {
		case 1:
			out[s.UserID] += 3
		case 2:
			out[s.UserID] += 2
		default:
			out[s.UserID] += 1
		}
	}
	return out
}

func mergeNightPoints(dst map[string]int, extra map[string]int) map[string]int {
	if dst == nil {
		dst = map[string]int{}
	}
	for id, pts := range extra {
		dst[id] += pts
	}
	return dst
}

func nightsToPoints(nights []socialNight, groupID string, members []seasonMember, start, end time.Time) map[string]int {
	memberSet := map[string]struct{}{}
	for _, m := range members {
		memberSet[m.UserID] = struct{}{}
	}
	pts := map[string]int{}
	for _, n := range nights {
		if n.Status != "COMPLETED" {
			continue
		}
		if n.FinishedAt.Before(start) || !n.FinishedAt.Before(end) {
			continue
		}
		if !nightAttributed(n, groupID, memberSet) {
			continue
		}
		pts = mergeNightPoints(pts, socialNightPoints(n.Placements))
	}
	return pts
}

func nightAttributed(n socialNight, groupID string, members map[string]struct{}) bool {
	if n.GroupID != "" && n.GroupID == groupID {
		return true
	}
	if n.GroupID != "" && n.GroupID != groupID {
		return false
	}
	if len(n.UserIDs) == 0 {
		return false
	}
	for _, id := range n.UserIDs {
		if _, ok := members[id]; !ok {
			return false
		}
	}
	return true
}

func isSocialNightFormat(format string) bool {
	switch format {
	case "AMERICANO", "MEXICANO", "KING_OF_THE_COURT":
		return true
	default:
		return false
	}
}

// splitPromotionRelegation stamps CHAMPION / PROMOTED / RELEGATED / STAYED
// onto already-ranked per-division slices. Divisions must be ordered by tier.
func splitPromotionRelegation(divisions [][]SeasonStanding) [][]SeasonStanding {
	out := make([][]SeasonStanding, len(divisions))
	last := len(divisions) - 1
	for i, rows := range divisions {
		copied := append([]SeasonStanding(nil), rows...)
		move := promoRelegateCount(len(copied))
		for j := range copied {
			copied[j].Outcome = outcomeStayed
		}
		if i == 0 && len(copied) > 0 {
			copied[0].Outcome = outcomeChampion
		}
		if i > 0 {
			for j := 0; j < move && j < len(copied); j++ {
				if copied[j].Outcome != outcomeChampion {
					copied[j].Outcome = outcomePromoted
				}
			}
		}
		if i < last {
			for j := 0; j < move && j < len(copied); j++ {
				idx := len(copied) - 1 - j
				if copied[idx].Outcome == outcomeStayed {
					copied[idx].Outcome = outcomeRelegated
				}
			}
		}
		out[i] = copied
	}
	return out
}

func promoRelegateCount(size int) int {
	if size < minDivisionForTwo {
		return promoRelegateOne
	}
	return promoRelegateTwo
}

func groupStandingsByDivision(rows []SeasonStanding) [][]SeasonStanding {
	by := map[int][]SeasonStanding{}
	for _, r := range rows {
		by[r.DivisionTier] = append(by[r.DivisionTier], r)
	}
	tiers := make([]int, 0, len(by))
	for t := range by {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)
	out := make([][]SeasonStanding, 0, len(tiers))
	for _, t := range tiers {
		div := by[t]
		sort.SliceStable(div, func(i, j int) bool {
			if div[i].Rank != div[j].Rank && div[i].Rank > 0 && div[j].Rank > 0 {
				return div[i].Rank < div[j].Rank
			}
			return seasonStandingLess(div[i], div[j], nil)
		})
		out = append(out, div)
	}
	return out
}

func applyOutcomes(rows []SeasonStanding) []SeasonStanding {
	grouped := groupStandingsByDivision(rows)
	stamped := splitPromotionRelegation(grouped)
	var out []SeasonStanding
	for _, div := range stamped {
		out = append(out, div...)
	}
	return out
}

func championUserID(rows []SeasonStanding) string {
	for _, r := range rows {
		if r.Outcome == outcomeChampion {
			return r.UserID
		}
	}
	for _, r := range rows {
		if r.DivisionTier == 1 && r.Rank == 1 {
			return r.UserID
		}
	}
	return ""
}

// nextSeasonDivisions applies promotion/relegation. Members who never had a
// division (joined after the season started) land in the lowest tier.
func nextSeasonDivisions(prev []SeasonStanding, newcomers []seasonMember) []seasonMember {
	lowest := 1
	seen := map[string]struct{}{}
	out := make([]seasonMember, 0, len(prev)+len(newcomers))
	for _, r := range prev {
		if r.DivisionTier > lowest {
			lowest = r.DivisionTier
		}
	}
	for _, r := range prev {
		tier := r.DivisionTier
		switch r.Outcome {
		case outcomePromoted:
			if tier > 1 {
				tier--
			}
		case outcomeRelegated:
			tier++
			if tier > lowest && r.DivisionTier == lowest {
				// opening a new bottom division is allowed when membership grows
			}
		}
		seen[r.UserID] = struct{}{}
		out = append(out, seasonMember{
			UserID:       r.UserID,
			Name:         r.Name,
			DivisionTier: tier,
		})
	}
	for _, n := range newcomers {
		if _, ok := seen[n.UserID]; ok {
			continue
		}
		seen[n.UserID] = struct{}{}
		out = append(out, seasonMember{
			UserID:       n.UserID,
			Name:         n.Name,
			DivisionTier: lowest,
		})
	}
	return out
}

func maxDivisionTier(members []seasonMember) int {
	max := 1
	for _, m := range members {
		if m.DivisionTier > max {
			max = m.DivisionTier
		}
	}
	return max
}

func divisionsFromStandings(rows []SeasonStanding) []Division {
	grouped := groupStandingsByDivision(rows)
	out := make([]Division, 0, len(grouped))
	for _, rows := range grouped {
		tier := 1
		if len(rows) > 0 {
			tier = rows[0].DivisionTier
		}
		copied := append([]SeasonStanding(nil), rows...)
		if copied == nil {
			copied = []SeasonStanding{}
		}
		out = append(out, Division{
			Tier:      tier,
			Name:      fmt.Sprintf("Division %d", tier),
			Standings: copied,
		})
	}
	if out == nil {
		out = []Division{}
	}
	return out
}

func seasonMemberIDs(members []seasonMember) []string {
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	return ids
}

func promotionIDs(rows []SeasonStanding) []string {
	var out []string
	for _, r := range rows {
		if r.Outcome == outcomePromoted {
			out = append(out, r.UserID)
		}
	}
	return out
}

func relegationIDs(rows []SeasonStanding) []string {
	var out []string
	for _, r := range rows {
		if r.Outcome == outcomeRelegated {
			out = append(out, r.UserID)
		}
	}
	return out
}
