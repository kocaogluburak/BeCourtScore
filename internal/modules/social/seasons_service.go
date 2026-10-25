package social

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"courtscore/internal/platform/sse"
)

// SeasonMatch is the slim snapshot score passes through SeasonNotifier.
type SeasonMatch struct {
	Sport     string
	PlayedAt  time.Time
	PlayerIDs []string
}

func (s *Service) requireMember(ctx context.Context, userID, groupID string) (Group, error) {
	g, err := s.groups.getGroup(ctx, groupID)
	if err != nil {
		return Group{}, err
	}
	ok, err := s.groups.isMember(ctx, groupID, userID)
	if err != nil {
		return Group{}, err
	}
	if !ok {
		return Group{}, ErrForbidden
	}
	return g, nil
}

func (s *Service) requireOwner(ctx context.Context, userID, groupID string) (Group, error) {
	g, err := s.requireMember(ctx, userID, groupID)
	if err != nil {
		return Group{}, err
	}
	if g.OwnerID != userID {
		return Group{}, ErrForbidden
	}
	return g, nil
}

// ListSeasons returns seasons for a group (current + past), newest first.
func (s *Service) ListSeasons(ctx context.Context, userID, groupID string, limit, offset int) ([]SeasonSummary, int64, error) {
	if s.seasons == nil {
		return []SeasonSummary{}, 0, nil
	}
	if _, err := s.requireMember(ctx, userID, groupID); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.seasons.listSeasons(ctx, groupID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]SeasonSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, seasonSummaryOf(row))
	}
	return out, total, nil
}

// GetCurrentSeason returns the live ACTIVE season, or ErrNoActiveSeason.
func (s *Service) GetCurrentSeason(ctx context.Context, userID, groupID string) (SeasonDetail, error) {
	if s.seasons == nil {
		return SeasonDetail{}, ErrNoActiveSeason
	}
	if _, err := s.requireMember(ctx, userID, groupID); err != nil {
		return SeasonDetail{}, err
	}
	se, err := s.seasons.getActiveSeason(ctx, groupID)
	if err != nil {
		return SeasonDetail{}, err
	}
	return s.detailForSeason(ctx, se, false)
}

// GetSeason returns one season. COMPLETED (archive) is Pro-gated.
func (s *Service) GetSeason(ctx context.Context, userID, groupID, seasonID string) (SeasonDetail, error) {
	if s.seasons == nil {
		return SeasonDetail{}, ErrNotFound
	}
	if _, err := s.requireMember(ctx, userID, groupID); err != nil {
		return SeasonDetail{}, err
	}
	se, err := s.seasons.getSeason(ctx, seasonID)
	if err != nil {
		return SeasonDetail{}, err
	}
	if se.GroupID != groupID {
		return SeasonDetail{}, ErrNotFound
	}
	if se.Status == seasonStatusCompleted && s.gate != nil {
		if err := s.gate.AllowSeasonArchive(ctx, userID); err != nil {
			return SeasonDetail{}, err
		}
	}
	return s.detailForSeason(ctx, se, se.Status == seasonStatusCompleted)
}

// StartSeason opens an ACTIVE season for the group. 8+ members require Pro.
func (s *Service) StartSeason(ctx context.Context, userID, groupID string, cfg SeasonConfig) (SeasonDetail, error) {
	if s.seasons == nil {
		return SeasonDetail{}, fmt.Errorf("%w: seasons unavailable", ErrInvalid)
	}
	g, err := s.requireOwner(ctx, userID, groupID)
	if err != nil {
		return SeasonDetail{}, err
	}
	cfg = normalizeSeasonConfig(cfg)
	if !validSeasonWeeks(cfg.Weeks) {
		return SeasonDetail{}, fmt.Errorf("%w: weeks", ErrInvalid)
	}
	if _, err := s.seasons.getActiveSeason(ctx, groupID); err == nil {
		return SeasonDetail{}, fmt.Errorf("%w: active season exists", ErrConflict)
	} else if err != nil && !errors.Is(err, ErrNoActiveSeason) && !errors.Is(err, ErrNotFound) {
		return SeasonDetail{}, err
	}
	members, err := s.groups.listMembers(ctx, groupID)
	if err != nil {
		return SeasonDetail{}, err
	}
	if s.gate != nil {
		if err := s.gate.AllowLargeLeague(ctx, userID, len(members)); err != nil {
			return SeasonDetail{}, err
		}
	}
	next, err := s.seasons.maxSeasonNumber(ctx, groupID)
	if err != nil {
		return SeasonDetail{}, err
	}
	start := s.clock().UTC()
	_, end := seasonWindow(start, cfg.Weeks)
	seeds := seedFirstDivisions(members, cfg.DivisionCount)
	created, err := s.seasons.createSeason(ctx, Season{
		GroupID:           g.ID,
		SeasonNumber:      next + 1,
		Name:              defaultSeasonName(next + 1),
		Status:            seasonStatusActive,
		StartsAt:          start,
		EndsAt:            end,
		DivisionCount:     maxDivisionTier(seeds),
		CountSocialNights: cfg.CountSocialNights,
	}, seeds)
	if err != nil {
		return SeasonDetail{}, err
	}
	detail, err := s.detailForSeason(ctx, created, false)
	if err != nil {
		return SeasonDetail{}, err
	}
	s.publishSeason(seasonMemberIDs(seeds), "season.started", map[string]any{
		"season_id":     created.ID,
		"group_id":      created.GroupID,
		"season_number": created.SeasonNumber,
	})
	return detail, nil
}

// FinalizeSeason is the owner manual rollover (same path as the scheduler).
func (s *Service) FinalizeSeason(ctx context.Context, userID, groupID, seasonID string) (SeasonDetail, error) {
	if s.seasons == nil {
		return SeasonDetail{}, ErrNotFound
	}
	if _, err := s.requireOwner(ctx, userID, groupID); err != nil {
		return SeasonDetail{}, err
	}
	se, err := s.seasons.getSeason(ctx, seasonID)
	if err != nil {
		return SeasonDetail{}, err
	}
	if se.GroupID != groupID {
		return SeasonDetail{}, ErrNotFound
	}
	if err := s.rolloverOne(ctx, se); err != nil {
		return SeasonDetail{}, err
	}
	done, err := s.seasons.getSeason(ctx, seasonID)
	if err != nil {
		return SeasonDetail{}, err
	}
	return s.detailForSeason(ctx, done, true)
}

func (s *Service) currentSeasonSummary(ctx context.Context, groupID string) *SeasonSummary {
	if s.seasons == nil {
		return nil
	}
	se, err := s.seasons.getActiveSeason(ctx, groupID)
	if err != nil {
		return nil
	}
	sum := seasonSummaryOf(se)
	return &sum
}

func (s *Service) detailForSeason(ctx context.Context, se Season, preferFrozen bool) (SeasonDetail, error) {
	if preferFrozen || se.Status == seasonStatusCompleted {
		frozen, err := s.seasons.listFrozenStandings(ctx, se.ID)
		if err != nil {
			return SeasonDetail{}, err
		}
		if len(frozen) > 0 || se.Status == seasonStatusCompleted {
			return SeasonDetail{
				SeasonSummary:     seasonSummaryOf(se),
				CountSocialNights: se.CountSocialNights,
				Divisions:         divisionsFromStandings(frozen),
				IsFrozen:          true,
			}, nil
		}
	}
	rows, err := s.computeLiveStandings(ctx, se)
	if err != nil {
		return SeasonDetail{}, err
	}
	return SeasonDetail{
		SeasonSummary:     seasonSummaryOf(se),
		CountSocialNights: se.CountSocialNights,
		Divisions:         divisionsFromStandings(rows),
		IsFrozen:          false,
	}, nil
}

func (s *Service) computeLiveStandings(ctx context.Context, se Season) ([]SeasonStanding, error) {
	g, err := s.groups.getGroup(ctx, se.GroupID)
	if err != nil {
		return nil, err
	}
	members, err := s.seasons.listSeasonMembers(ctx, se.ID)
	if err != nil {
		return nil, err
	}
	matches, err := s.seasons.listSeasonMatches(ctx, se.GroupID, g.Sport, se.StartsAt, se.EndsAt)
	if err != nil {
		return nil, err
	}
	var nights []socialNight
	if se.CountSocialNights {
		nights, err = s.seasons.listSeasonSocialNights(ctx, se.GroupID, se.StartsAt, se.EndsAt)
		if err != nil {
			return nil, err
		}
	}
	pts := nightsToPoints(nights, se.GroupID, members, se.StartsAt, se.EndsAt)
	rows := buildSeasonStandings(members, matches, g.Sport, se.StartsAt, se.EndsAt, pts)
	if rows == nil {
		rows = []SeasonStanding{}
	}
	return rows, nil
}

func (s *Service) publishSeason(userIDs []string, typ string, data any) {
	if s.hub == nil {
		return
	}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		s.hub.Publish(id, sse.Event{Type: typ, Data: data})
	}
}

// OnMatchFinished publishes season.standings_updated to members of any
// ACTIVE season that includes a registered player from the finished match.
func (s *Service) OnMatchFinished(ctx context.Context, m SeasonMatch) {
	if s == nil || s.seasons == nil || s.hub == nil {
		return
	}
	ids := sideIDs(m.PlayerIDs...)
	if len(ids) == 0 {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("social: season hook panic", "err", rec)
		}
	}()
	seasons, err := s.seasons.listActiveSeasonsForUsers(ctx, ids)
	if err != nil {
		slog.Warn("social: list active seasons", "err", err)
		return
	}
	for _, se := range seasons {
		members, err := s.seasons.listSeasonMembers(ctx, se.ID)
		if err != nil {
			continue
		}
		s.publishSeason(seasonMemberIDs(members), "season.standings_updated", map[string]any{
			"season_id": se.ID,
			"group_id":  se.GroupID,
		})
	}
}
