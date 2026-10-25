package social

import (
	"context"
	"errors"
	"time"
)

// RunSeasonRollover freezes due ACTIVE seasons, stamps promotion/relegation,
// opens the next ACTIVE season, and publishes SSE. Idempotent.
func (s *Service) RunSeasonRollover(ctx context.Context, now time.Time) error {
	if s == nil || s.seasons == nil {
		return nil
	}
	due, err := s.seasons.listDueActiveSeasons(ctx, now)
	if err != nil {
		return err
	}
	for _, se := range due {
		if err := s.rolloverOne(ctx, se); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) rolloverOne(ctx context.Context, se Season) error {
	if se.Status != seasonStatusActive {
		return nil
	}
	rows, err := s.computeLiveStandings(ctx, se)
	if err != nil {
		return err
	}
	rows = applyOutcomes(rows)
	champ := championUserID(rows)
	if err := s.seasons.freezeAndComplete(ctx, se.ID, champ, rows); err != nil {
		return err
	}
	// Re-read: a concurrent worker may have completed first.
	done, err := s.seasons.getSeason(ctx, se.ID)
	if err != nil {
		return err
	}
	if done.Status != seasonStatusCompleted {
		return nil
	}
	members, err := s.seasons.listSeasonMembers(ctx, se.ID)
	if err != nil {
		return err
	}
	s.publishSeason(seasonMemberIDs(members), "season.completed", map[string]any{
		"season_id":        done.ID,
		"group_id":         done.GroupID,
		"champion_user_id": champ,
		"promotions":       promotionIDs(rows),
		"relegations":      relegationIDs(rows),
	})

	groupMembers, err := s.groups.listMembers(ctx, se.GroupID)
	if err != nil {
		return err
	}
	newcomers := make([]seasonMember, 0, len(groupMembers))
	for _, m := range groupMembers {
		newcomers = append(newcomers, seasonMember{UserID: m.UserID, Name: m.Name})
	}
	nextMembers := nextSeasonDivisions(rows, newcomers)
	if len(nextMembers) == 0 {
		return nil
	}
	nextNum := se.SeasonNumber + 1
	weeks := int(done.EndsAt.Sub(done.StartsAt).Hours()/24) / 7
	if !validSeasonWeeks(weeks) {
		weeks = defaultSeasonWeeks
	}
	start := done.EndsAt.UTC()
	if start.IsZero() {
		start = s.clock().UTC()
	}
	_, end := seasonWindow(start, weeks)
	next, err := s.seasons.createSeason(ctx, Season{
		GroupID:           se.GroupID,
		SeasonNumber:      nextNum,
		Name:              defaultSeasonName(nextNum),
		Status:            seasonStatusActive,
		StartsAt:          start,
		EndsAt:            end,
		DivisionCount:     maxDivisionTier(nextMembers),
		CountSocialNights: se.CountSocialNights,
	}, nextMembers)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil
		}
		return err
	}
	s.publishSeason(seasonMemberIDs(nextMembers), "season.started", map[string]any{
		"season_id":     next.ID,
		"group_id":      next.GroupID,
		"season_number": next.SeasonNumber,
	})
	return nil
}
