package score

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"courtscore/internal/platform/sse"
)

const rematchExpiry = 48 * time.Hour

// UserSummary is the public projection of a user used on rematch payloads.
// Same JSON shape as social.UserSummary; defined here so score does not import social.
type UserSummary struct {
	ID          string  `json:"id"`
	Nickname    *string `json:"nickname"`
	Name        *string `json:"name"`
	Surname     *string `json:"surname"`
	ProfileIcon *string `json:"profile_icon"`
}

// UserLookup hydrates challenger/opponent summaries without importing social.
// Implemented by social.Service via a thin adapter in cmd/api/main.go.
type UserLookup interface {
	UserSummaryByID(ctx context.Context, viewerID, targetID string) (UserSummary, error)
}

// RematchChallenge is a pending (or closed) rematch request between friends.
type RematchChallenge struct {
	ID               string      `json:"id"`
	ChallengerID     string      `json:"challenger_id"`
	OpponentID       string      `json:"opponent_id"`
	Challenger       UserSummary `json:"challenger"`
	Opponent         UserSummary `json:"opponent"`
	Sport            string      `json:"sport"`
	SetsToWin        int         `json:"sets_to_win"`
	AdvantageEnabled bool        `json:"advantage_enabled"`
	PointsToWin      int         `json:"points_to_win"`
	SetRule          *string     `json:"set_rule,omitempty"`
	FinalSet         *string     `json:"final_set,omitempty"`
	SourceMatchID    *string     `json:"source_match_id,omitempty"`
	Status           string      `json:"status"`
	LiveMatchID      *string     `json:"live_match_id,omitempty"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// RematchCreateInput is the body of POST /v1/rematches.
type RematchCreateInput struct {
	OpponentID       string
	Sport            string
	SourceMatchID    *string
	SetsToWin        int
	AdvantageEnabled *bool
	PointsToWin      int
	SetRule          *string
	FinalSet         *string
}

// RematchPrefill is returned on accept so clients open Match Setup pre-filled (option A).
type RematchPrefill struct {
	Sport            string      `json:"sport"`
	Opponent         UserSummary `json:"opponent"`
	SetsToWin        int         `json:"sets_to_win"`
	AdvantageEnabled bool        `json:"advantage_enabled"`
	PointsToWin      int         `json:"points_to_win"`
	SetRule          *string     `json:"set_rule,omitempty"`
	FinalSet         *string     `json:"final_set,omitempty"`
	LiveMatchID      *string     `json:"live_match_id,omitempty"`
}

func (s *Service) SetUserLookup(u UserLookup) { s.users = u }

// CreateRematch opens a pending challenge to an accepted friend.
func (s *Service) CreateRematch(ctx context.Context, userID string, in RematchCreateInput) (RematchChallenge, error) {
	s.expireStaleRematches(ctx)
	in.OpponentID = strings.TrimSpace(in.OpponentID)
	if in.OpponentID == "" {
		return RematchChallenge{}, fmt.Errorf("%w: opponent_id required", ErrInvalid)
	}
	if in.OpponentID == userID {
		return RematchChallenge{}, fmt.Errorf("%w: cannot rematch yourself", ErrInvalid)
	}
	ok, err := s.friends.AreFriends(ctx, userID, in.OpponentID)
	if err != nil {
		return RematchChallenge{}, err
	}
	if !ok {
		return RematchChallenge{}, fmt.Errorf("%w: opponent must be an accepted friend", ErrForbidden)
	}

	sport := strings.TrimSpace(in.Sport)
	setsToWin := in.SetsToWin
	adv := true
	if in.AdvantageEnabled != nil {
		adv = *in.AdvantageEnabled
	}
	points := in.PointsToWin
	setRule, finalSet := in.SetRule, in.FinalSet

	if in.SourceMatchID != nil && strings.TrimSpace(*in.SourceMatchID) != "" {
		srcID := strings.TrimSpace(*in.SourceMatchID)
		in.SourceMatchID = &srcID
		m, err := s.GetMatch(ctx, userID, srcID)
		if err != nil {
			return RematchChallenge{}, err
		}
		if sport == "" {
			sport = m.Sport
		}
		if m.SetRule != "" && setRule == nil {
			r := m.SetRule
			setRule = &r
		}
		if m.FinalSet != "" && finalSet == nil {
			f := m.FinalSet
			finalSet = &f
		}
	}
	if !validSports[sport] {
		return RematchChallenge{}, fmt.Errorf("%w: unknown sport %q", ErrInvalid, sport)
	}
	if setsToWin < 1 {
		setsToWin = 2
	}
	if points < 1 {
		points = 11
	}
	in.Sport = sport
	in.SetsToWin = setsToWin
	in.AdvantageEnabled = &adv
	in.PointsToWin = points
	in.SetRule = setRule
	in.FinalSet = finalSet

	ch, err := s.repo.insertRematch(ctx, userID, in)
	if err != nil {
		return RematchChallenge{}, err
	}
	ch = s.hydrateRematch(ctx, userID, ch)
	s.publishRematch("rematch.received", ch.OpponentID, ch)
	s.pushRematch(ctx, ch.OpponentID, "Rematch challenge",
		fmt.Sprintf("%s wants a rematch", displayName(ch.Challenger)),
		map[string]string{
			"type":       "rematch.received",
			"rematch_id": ch.ID,
			"sport":      ch.Sport,
		})
	return ch, nil
}

// ListIncomingRematches returns pending challenges addressed to the user.
func (s *Service) ListIncomingRematches(ctx context.Context, userID string, limit, offset int) ([]RematchChallenge, int64, error) {
	s.expireStaleRematches(ctx)
	items, total, err := s.repo.listIncomingRematches(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return s.hydrateRematchList(ctx, userID, items), total, nil
}

// ListOutgoingRematches returns pending challenges the user sent.
func (s *Service) ListOutgoingRematches(ctx context.Context, userID string, limit, offset int) ([]RematchChallenge, int64, error) {
	s.expireStaleRematches(ctx)
	items, total, err := s.repo.listOutgoingRematches(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return s.hydrateRematchList(ctx, userID, items), total, nil
}

// AcceptRematch is opponent-only; pending → accepted. Returns a Match Setup prefill (option A).
func (s *Service) AcceptRematch(ctx context.Context, userID, id string) (RematchPrefill, error) {
	s.expireStaleRematches(ctx)
	ch, err := s.repo.getRematch(ctx, id)
	if err != nil {
		return RematchPrefill{}, err
	}
	if ch.OpponentID != userID {
		return RematchPrefill{}, ErrForbidden
	}
	if ch.Status != "pending" {
		return RematchPrefill{}, ErrWrongState
	}
	updated, err := s.repo.updateRematchStatus(ctx, id, "pending", "accepted")
	if err != nil {
		return RematchPrefill{}, err
	}
	updated = s.hydrateRematch(ctx, userID, updated)
	prefill := RematchPrefill{
		Sport:            updated.Sport,
		Opponent:         updated.Challenger,
		SetsToWin:        updated.SetsToWin,
		AdvantageEnabled: updated.AdvantageEnabled,
		PointsToWin:      updated.PointsToWin,
		SetRule:          updated.SetRule,
		FinalSet:         updated.FinalSet,
	}
	s.publishRematch("rematch.accepted", updated.ChallengerID, prefill)
	s.pushRematch(ctx, updated.ChallengerID, "Rematch accepted",
		fmt.Sprintf("%s accepted your rematch", displayName(updated.Opponent)),
		map[string]string{
			"type":        "rematch.accepted",
			"rematch_id":  updated.ID,
			"opponent_id": updated.OpponentID,
		})
	return prefill, nil
}

// RejectRematch is opponent-only; pending → rejected.
func (s *Service) RejectRematch(ctx context.Context, userID, id string) (RematchChallenge, error) {
	ch, err := s.repo.getRematch(ctx, id)
	if err != nil {
		return RematchChallenge{}, err
	}
	if ch.OpponentID != userID {
		return RematchChallenge{}, ErrForbidden
	}
	if ch.Status != "pending" {
		return RematchChallenge{}, ErrWrongState
	}
	updated, err := s.repo.updateRematchStatus(ctx, id, "pending", "rejected")
	if err != nil {
		return RematchChallenge{}, err
	}
	updated = s.hydrateRematch(ctx, userID, updated)
	s.publishRematch("rematch.rejected", updated.ChallengerID, updated)
	s.pushRematch(ctx, updated.ChallengerID, "Rematch declined",
		fmt.Sprintf("%s declined your rematch", displayName(updated.Opponent)),
		map[string]string{
			"type":       "rematch.rejected",
			"rematch_id": updated.ID,
		})
	return updated, nil
}

// CancelRematch is challenger-only; pending → cancelled.
func (s *Service) CancelRematch(ctx context.Context, userID, id string) (RematchChallenge, error) {
	ch, err := s.repo.getRematch(ctx, id)
	if err != nil {
		return RematchChallenge{}, err
	}
	if ch.ChallengerID != userID {
		return RematchChallenge{}, ErrForbidden
	}
	if ch.Status != "pending" {
		return RematchChallenge{}, ErrWrongState
	}
	updated, err := s.repo.updateRematchStatus(ctx, id, "pending", "cancelled")
	if err != nil {
		return RematchChallenge{}, err
	}
	updated = s.hydrateRematch(ctx, userID, updated)
	s.publishRematch("rematch.cancelled", updated.OpponentID, updated)
	s.pushRematch(ctx, updated.OpponentID, "Rematch cancelled",
		fmt.Sprintf("%s cancelled the rematch", displayName(updated.Challenger)),
		map[string]string{
			"type":       "rematch.cancelled",
			"rematch_id": updated.ID,
		})
	return updated, nil
}

func (s *Service) expireStaleRematches(ctx context.Context) {
	if err := s.repo.expirePendingRematches(ctx, time.Now().Add(-rematchExpiry)); err != nil {
		slog.Warn("expire rematches failed", "err", err)
	}
}

func (s *Service) hydrateRematchList(ctx context.Context, viewerID string, items []RematchChallenge) []RematchChallenge {
	out := make([]RematchChallenge, len(items))
	for i, ch := range items {
		out[i] = s.hydrateRematch(ctx, viewerID, ch)
	}
	return out
}

func (s *Service) hydrateRematch(ctx context.Context, viewerID string, ch RematchChallenge) RematchChallenge {
	ch.Challenger = s.lookupUser(ctx, viewerID, ch.ChallengerID)
	ch.Opponent = s.lookupUser(ctx, viewerID, ch.OpponentID)
	return ch
}

func (s *Service) lookupUser(ctx context.Context, viewerID, id string) UserSummary {
	if s.users != nil {
		if u, err := s.users.UserSummaryByID(ctx, viewerID, id); err == nil && u.ID != "" {
			return u
		}
	}
	if u, err := s.repo.userSummary(ctx, id); err == nil && u.ID != "" {
		return u
	}
	return UserSummary{ID: id}
}

func (s *Service) publishRematch(typ, userID string, data any) {
	if s.hub == nil || userID == "" {
		return
	}
	s.hub.Publish(userID, sse.Event{Type: typ, Data: data})
}

func (s *Service) pushRematch(ctx context.Context, userID, title, body string, data map[string]string) {
	if s.push == nil || userID == "" {
		return
	}
	if err := s.push.SendToUser(ctx, userID, title, body, data); err != nil {
		slog.Warn("rematch push failed", "err", err, "type", data["type"])
	}
}

func displayName(u UserSummary) string {
	if u.Nickname != nil && *u.Nickname != "" {
		return *u.Nickname
	}
	parts := []string{}
	if u.Name != nil && *u.Name != "" {
		parts = append(parts, *u.Name)
	}
	if u.Surname != nil && *u.Surname != "" {
		parts = append(parts, *u.Surname)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return "A friend"
}
