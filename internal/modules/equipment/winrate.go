package equipment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"courtscore/internal/modules/score"
)

var (
	_ score.UsageAccruer       = (*Service)(nil)
	_ score.EquipmentMatchSink = (*Service)(nil)
)

// MinWinRateSample is the minimum number of matches with the same gear combination before
// a win rate is shown. Same value as the Wrapped `has_enough_data` threshold (AC-17).
const MinWinRateSample = 5

// WinRate is GET /v1/me/equipment/win-rate. Only enough_data is set below the sample floor.
type WinRate struct {
	EnoughData    bool `json:"enough_data"`
	MatchesPlayed *int `json:"matches_played,omitempty"`
	MatchesWon    *int `json:"matches_won,omitempty"`
	WinPercent    *int `json:"win_percent,omitempty"`
}

// usageRow is one match_equipment_usage row.
type usageRow struct {
	MatchID  string
	UserID   string
	Sport    string
	ComboKey string
	ItemIDs  []string
	Won      bool
	PlayedAt time.Time
}

// comboPart is "<category.code>:<brand_id|CUSTOM:name>:<model_id|CUSTOM:name|NONE>".
// The key follows brand/model identity, not the physical row id, so a restring or regrip
// with the same brand/model keeps the combination (and its sample) intact. Custom names are
// matched case-insensitively.
func comboPart(it Item) string {
	brand := "CUSTOM:" + strings.ToLower(derefDisplay(it.CustomBrandName))
	if it.BrandID != nil {
		brand = *it.BrandID
	}
	model := "NONE"
	switch {
	case it.ModelID != nil:
		model = *it.ModelID
	case derefDisplay(it.CustomModelName) != "":
		model = "CUSTOM:" + strings.ToLower(derefDisplay(it.CustomModelName))
	}
	return it.Category.Code + ":" + brand + ":" + model
}

// comboKey is the sorted, pipe-joined set of comboParts of the active items. Pure.
func comboKey(items []Item) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, comboPart(it))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func winPercent(won, played int) int {
	if played <= 0 {
		return 0
	}
	return int(math.Round(float64(won) / float64(played) * 100))
}

func sameID(p *string, id string) bool { return p != nil && id != "" && *p == id }

// matchSide returns "A"/"B" if userID played in m, else "".
func matchSide(m score.Match, userID string) string {
	switch {
	case sameID(m.PlayerAUserID, userID) || sameID(m.PlayerA2UserID, userID):
		return "A"
	case sameID(m.PlayerBUserID, userID) || sameID(m.PlayerB2UserID, userID):
		return "B"
	}
	return ""
}

// OnMatchRecorded implements score.EquipmentMatchSink. Only the recorder (created_by) is
// tracked, and only if they played in the match and have active gear for that sport.
// Best-effort; never fails the match write.
func (s *Service) OnMatchRecorded(ctx context.Context, m score.Match) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("equipment: match hook panic", "err", rec)
		}
	}()
	sport := strings.ToUpper(strings.TrimSpace(m.Sport))
	if m.ID == "" || m.CreatedBy == "" || !validSport(sport) {
		return
	}
	side := matchSide(m, m.CreatedBy)
	if side == "" {
		return
	}
	items, err := s.repo.activeItemsForSport(ctx, m.CreatedBy, sport)
	if err != nil {
		slog.Warn("equipment: match usage items", "err", err, "match", m.ID)
		return
	}
	if len(items) == 0 {
		return
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	err = s.repo.insertUsage(ctx, usageRow{
		MatchID: m.ID, UserID: m.CreatedBy, Sport: sport, ComboKey: comboKey(items),
		ItemIDs: ids, Won: side == m.WinnerSide, PlayedAt: m.PlayedAt,
	})
	if err != nil {
		slog.Warn("equipment: match usage insert", "err", err, "match", m.ID)
	}
}

// OnMatchDeleted implements score.EquipmentMatchSink.
func (s *Service) OnMatchDeleted(ctx context.Context, matchID string) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("equipment: match delete hook panic", "err", rec)
		}
	}()
	if matchID == "" {
		return
	}
	if err := s.repo.deleteUsage(ctx, matchID); err != nil {
		slog.Warn("equipment: match usage delete", "err", err, "match", matchID)
	}
}

// WinRate returns the win percentage of the user's CURRENT active gear combination in a
// sport, from forward-only match usage rows (AC-16/17).
func (s *Service) WinRate(ctx context.Context, userID, sport string) (WinRate, error) {
	sport = strings.ToUpper(strings.TrimSpace(sport))
	if !validSport(sport) {
		return WinRate{}, fmt.Errorf("%w: sport required", ErrInvalid)
	}
	items, err := s.repo.activeItemsForSport(ctx, userID, sport)
	if err != nil {
		return WinRate{}, err
	}
	if len(items) == 0 {
		return WinRate{}, nil
	}
	played, won, err := s.repo.usageCounts(ctx, userID, sport, comboKey(items))
	if err != nil {
		return WinRate{}, err
	}
	if played < MinWinRateSample {
		return WinRate{}, nil
	}
	pct := winPercent(won, played)
	return WinRate{EnoughData: true, MatchesPlayed: &played, MatchesWon: &won, WinPercent: &pct}, nil
}
