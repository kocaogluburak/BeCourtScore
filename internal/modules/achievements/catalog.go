package achievements

// Badge category / tier values are stable API contract strings.
const (
	CategoryMilestone   = "milestone"
	CategoryPerformance = "performance"
	CategoryTournament  = "tournament"
	CategoryConsistency = "consistency"

	TierBronze = "bronze"
	TierSilver = "silver"
	TierGold   = "gold"
)

// Badge codes are the API contract. Names/descriptions live in client i18n.
const (
	BadgeFirstMatch     = "first_match"
	BadgeFirstWin       = "first_win"
	BadgeMatches10      = "matches_10"
	BadgeMatches50      = "matches_50"
	BadgeMatches100     = "matches_100"
	BadgeWinStreak5     = "win_streak_5"
	BadgeElo1600        = "elo_1600"
	BadgeTournamentWin1 = "tournament_win_1"
	BadgeTiebreakMaster = "tiebreak_master"
	BadgeWeeksStreak4   = "weeks_streak_4"
	BadgeWeeksStreak12  = "weeks_streak_12"
	BadgeGoalCrusher    = "goal_crusher"
)

// CatalogEntry is one code-defined badge (no localized text).
type CatalogEntry struct {
	Code      string `json:"code"`
	Category  string `json:"category"`
	Tier      string `json:"tier"`
	Threshold int    `json:"threshold"`
	Icon      string `json:"icon"`
	Sort      int    `json:"sort"`
}

// Catalog is the v1 badge set, ordered by Sort.
func Catalog() []CatalogEntry {
	return []CatalogEntry{
		{Code: BadgeFirstMatch, Category: CategoryMilestone, Tier: TierBronze, Threshold: 1, Icon: "first_match", Sort: 10},
		{Code: BadgeFirstWin, Category: CategoryMilestone, Tier: TierBronze, Threshold: 1, Icon: "first_win", Sort: 20},
		{Code: BadgeMatches10, Category: CategoryMilestone, Tier: TierBronze, Threshold: 10, Icon: "matches_10", Sort: 30},
		{Code: BadgeMatches50, Category: CategoryMilestone, Tier: TierSilver, Threshold: 50, Icon: "matches_50", Sort: 40},
		{Code: BadgeMatches100, Category: CategoryMilestone, Tier: TierGold, Threshold: 100, Icon: "matches_100", Sort: 50},
		{Code: BadgeWinStreak5, Category: CategoryPerformance, Tier: TierSilver, Threshold: 5, Icon: "win_streak_5", Sort: 60},
		{Code: BadgeElo1600, Category: CategoryPerformance, Tier: TierGold, Threshold: 1600, Icon: "elo_1600", Sort: 70},
		{Code: BadgeTournamentWin1, Category: CategoryTournament, Tier: TierGold, Threshold: 1, Icon: "tournament_win_1", Sort: 80},
		{Code: BadgeTiebreakMaster, Category: CategoryPerformance, Tier: TierSilver, Threshold: 10, Icon: "tiebreak_master", Sort: 90},
		{Code: BadgeWeeksStreak4, Category: CategoryConsistency, Tier: TierBronze, Threshold: 4, Icon: "weeks_streak_4", Sort: 100},
		{Code: BadgeWeeksStreak12, Category: CategoryConsistency, Tier: TierGold, Threshold: 12, Icon: "weeks_streak_12", Sort: 110},
		{Code: BadgeGoalCrusher, Category: CategoryConsistency, Tier: TierSilver, Threshold: 4, Icon: "goal_crusher", Sort: 120},
	}
}

func catalogByCode() map[string]CatalogEntry {
	out := make(map[string]CatalogEntry, 12)
	for _, e := range Catalog() {
		out[e.Code] = e
	}
	return out
}
