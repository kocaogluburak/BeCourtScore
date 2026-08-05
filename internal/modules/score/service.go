package score

import (
	"context"
	"fmt"
	"log/slog"

	"courtscore/internal/platform/sse"
	"github.com/jackc/pgx/v5/pgxpool"
)

var validSports = map[string]bool{
	"TENNIS": true, "PADEL": true, "SQUASH": true, "PING_PONG": true,
}

// FriendChecker reports whether two users have an accepted friendship.
// Implemented by the social module; wired in cmd/api/main.go.
type FriendChecker interface {
	AreFriends(ctx context.Context, userA, userB string) (bool, error)
}

// PushSender delivers OS push notifications to a user (FCM). Best-effort.
type PushSender interface {
	SendToUser(ctx context.Context, userID, title, body string, data map[string]string) error
}

// store is the persistence surface used by Service.
// The concrete *repo implements it; tests substitute an in-memory fake.
type store interface {
	insert(ctx context.Context, createdBy string, in CreateInput) (Match, error)
	listForUser(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Match, int64, error)
	getByID(ctx context.Context, id string) (Match, error)
	deleteByID(ctx context.Context, id, createdBy string) error
	insertLive(ctx context.Context, createdBy string, in LiveStartInput) (LiveMatch, error)
	getLiveByID(ctx context.Context, id string) (LiveMatch, error)
	updateLiveScore(ctx context.Context, id string, u LiveScoreUpdate) (LiveMatch, error)
	endLive(ctx context.Context, id string, in LiveEndInput, historyID *string) (LiveMatch, error)
	cancelLive(ctx context.Context, id string) (LiveMatch, error)
	listOpenLiveByCreator(ctx context.Context, userID string, limit, offset int) ([]LiveMatch, int64, error)
}

// Service is the score domain's business logic layer.
type Service struct {
	repo    store
	friends FriendChecker
	hub     *sse.Hub
	push    PushSender
}

// NewService creates a Service wired to the given Postgres pool.
func NewService(pool *pgxpool.Pool, friends FriendChecker, hub *sse.Hub, push PushSender) *Service {
	return &Service{repo: &repo{pool: pool}, friends: friends, hub: hub, push: push}
}

// CreateMatch validates and records a finished match.
func (s *Service) CreateMatch(ctx context.Context, userID string, in CreateInput) (Match, error) {
	if !validSports[in.Sport] {
		return Match{}, fmt.Errorf("%w: unknown sport %q", ErrInvalid, in.Sport)
	}
	if in.PlayerAName == "" || in.PlayerBName == "" {
		return Match{}, fmt.Errorf("%w: player names required", ErrInvalid)
	}
	if in.WinnerSide != "A" && in.WinnerSide != "B" {
		return Match{}, fmt.Errorf("%w: winner_side must be A or B", ErrInvalid)
	}
	if in.SetsA < 0 || in.SetsB < 0 {
		return Match{}, fmt.Errorf("%w: sets must be >= 0", ErrInvalid)
	}
	return s.repo.insert(ctx, userID, in)
}

// ListMyMatches returns the caller's match history (participant or recorder).
func (s *Service) ListMyMatches(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Match, int64, error) {
	return s.repo.listForUser(ctx, userID, f, limit, offset)
}

// ListUserMatches returns another user's history. Allowed for self or an
// accepted friend of the target user.
func (s *Service) ListUserMatches(ctx context.Context, viewerID, targetID string, f ListFilter, limit, offset int) ([]Match, int64, error) {
	if viewerID != targetID {
		ok, err := s.friends.AreFriends(ctx, viewerID, targetID)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			return nil, 0, ErrForbidden
		}
	}
	return s.repo.listForUser(ctx, targetID, f, limit, offset)
}

// GetMatch returns a match if the viewer is a participant, the recorder,
// or an accepted friend of a registered participant.
func (s *Service) GetMatch(ctx context.Context, viewerID, matchID string) (Match, error) {
	m, err := s.repo.getByID(ctx, matchID)
	if err != nil {
		return Match{}, err
	}

	if m.CreatedBy == viewerID ||
		(m.PlayerAUserID != nil && *m.PlayerAUserID == viewerID) ||
		(m.PlayerBUserID != nil && *m.PlayerBUserID == viewerID) {
		return m, nil
	}

	for _, participant := range []*string{&m.CreatedBy, m.PlayerAUserID, m.PlayerBUserID} {
		if participant == nil {
			continue
		}
		ok, err := s.friends.AreFriends(ctx, viewerID, *participant)
		if err != nil {
			return Match{}, err
		}
		if ok {
			return m, nil
		}
	}
	return Match{}, ErrForbidden
}

// DeleteMatch removes a match; only its creator may delete it.
func (s *Service) DeleteMatch(ctx context.Context, userID, matchID string) error {
	return s.repo.deleteByID(ctx, matchID, userID)
}

// StartLiveMatch creates an in-progress session and notifies the registered opponent.
func (s *Service) StartLiveMatch(ctx context.Context, userID string, in LiveStartInput) (LiveMatch, error) {
	if !validSports[in.Sport] {
		return LiveMatch{}, fmt.Errorf("%w: unknown sport %q", ErrInvalid, in.Sport)
	}
	if in.PlayerAName == "" || in.PlayerBName == "" {
		return LiveMatch{}, fmt.Errorf("%w: player names required", ErrInvalid)
	}
	if in.SetsToWin < 1 {
		in.SetsToWin = 2
	}
	if in.PointsToWin < 1 {
		in.PointsToWin = 11
	}
	if err := s.requireFriendIfSet(ctx, userID, in.PlayerAUserID); err != nil {
		return LiveMatch{}, err
	}
	if err := s.requireFriendIfSet(ctx, userID, in.PlayerBUserID); err != nil {
		return LiveMatch{}, err
	}

	m, err := s.repo.insertLive(ctx, userID, in)
	if err != nil {
		return LiveMatch{}, err
	}
	s.publishLive("match.started", m)
	s.pushOpponent(ctx, userID, m)
	return m, nil
}

// GetLiveMatch returns a live match if the viewer is creator or a registered participant.
func (s *Service) GetLiveMatch(ctx context.Context, viewerID, id string) (LiveMatch, error) {
	m, err := s.repo.getLiveByID(ctx, id)
	if err != nil {
		return LiveMatch{}, err
	}
	if !s.canViewLive(viewerID, m) {
		return LiveMatch{}, ErrForbidden
	}
	return m, nil
}

// UpdateLiveMatch applies a score snapshot from the scorer (creator only).
func (s *Service) UpdateLiveMatch(ctx context.Context, userID, id string, u LiveScoreUpdate) (LiveMatch, error) {
	m, err := s.repo.getLiveByID(ctx, id)
	if err != nil {
		return LiveMatch{}, err
	}
	if m.CreatedBy != userID {
		return LiveMatch{}, ErrForbidden
	}
	if u.WinnerSide != nil && *u.WinnerSide != "A" && *u.WinnerSide != "B" {
		return LiveMatch{}, fmt.Errorf("%w: winner_side must be A or B", ErrInvalid)
	}
	updated, err := s.repo.updateLiveScore(ctx, id, u)
	if err != nil {
		return LiveMatch{}, err
	}
	s.publishLive("match.score_updated", updated)
	return updated, nil
}

// EndLiveMatch finalizes the live session, archives to history, notifies spectators.
func (s *Service) EndLiveMatch(ctx context.Context, userID, id string, in LiveEndInput) (LiveMatch, error) {
	m, err := s.repo.getLiveByID(ctx, id)
	if err != nil {
		return LiveMatch{}, err
	}
	if m.CreatedBy != userID {
		return LiveMatch{}, ErrForbidden
	}
	if in.WinnerSide != "A" && in.WinnerSide != "B" {
		return LiveMatch{}, fmt.Errorf("%w: winner_side must be A or B", ErrInvalid)
	}

	hist, err := s.repo.insert(ctx, userID, CreateInput{
		Sport:         m.Sport,
		PlayerAName:   m.PlayerAName,
		PlayerBName:   m.PlayerBName,
		PlayerAUserID: m.PlayerAUserID,
		PlayerBUserID: m.PlayerBUserID,
		SetsA:         in.SetsA,
		SetsB:         in.SetsB,
		WinnerSide:    in.WinnerSide,
	})
	if err != nil {
		return LiveMatch{}, err
	}
	ended, err := s.repo.endLive(ctx, id, in, &hist.ID)
	if err != nil {
		return LiveMatch{}, err
	}
	s.publishLive("match.ended", ended)
	return ended, nil
}

// ListMyOpenLiveMatches returns in-progress sessions created by the user.
func (s *Service) ListMyOpenLiveMatches(ctx context.Context, userID string, limit, offset int) ([]LiveMatch, int64, error) {
	return s.repo.listOpenLiveByCreator(ctx, userID, limit, offset)
}

// CancelLiveMatch ends an in-progress session without writing match history.
// Creator only — used to abandon stuck / abandoned scoreboard sessions.
func (s *Service) CancelLiveMatch(ctx context.Context, userID, id string) (LiveMatch, error) {
	m, err := s.repo.getLiveByID(ctx, id)
	if err != nil {
		return LiveMatch{}, err
	}
	if m.CreatedBy != userID {
		return LiveMatch{}, ErrForbidden
	}
	if m.Status != "IN_PROGRESS" {
		return LiveMatch{}, ErrWrongState
	}
	cancelled, err := s.repo.cancelLive(ctx, id)
	if err != nil {
		return LiveMatch{}, err
	}
	s.publishLive("match.cancelled", cancelled)
	return cancelled, nil
}

func (s *Service) requireFriendIfSet(ctx context.Context, userID string, other *string) error {
	if other == nil || *other == "" || *other == userID {
		return nil
	}
	ok, err := s.friends.AreFriends(ctx, userID, *other)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: opponent must be an accepted friend", ErrForbidden)
	}
	return nil
}

func (s *Service) canViewLive(viewerID string, m LiveMatch) bool {
	if m.CreatedBy == viewerID {
		return true
	}
	if m.PlayerAUserID != nil && *m.PlayerAUserID == viewerID {
		return true
	}
	if m.PlayerBUserID != nil && *m.PlayerBUserID == viewerID {
		return true
	}
	return false
}

func (s *Service) publishLive(typ string, m LiveMatch) {
	if s.hub == nil {
		return
	}
	ev := sse.Event{Type: typ, Data: m}
	seen := map[string]bool{}
	for _, id := range liveRecipientIDs(m) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		s.hub.Publish(id, ev)
	}
}

func (s *Service) pushOpponent(ctx context.Context, starterID string, m LiveMatch) {
	if s.push == nil {
		return
	}
	opp := opponentUserID(starterID, m)
	if opp == "" {
		return
	}
	title := "Live match started"
	body := fmt.Sprintf("%s vs %s — tap to watch the score", m.PlayerAName, m.PlayerBName)
	data := map[string]string{
		"type":          "match.started",
		"live_match_id": m.ID,
	}
	if err := s.push.SendToUser(ctx, opp, title, body, data); err != nil {
		slog.Warn("live match push failed", "err", err)
	}
}

func liveRecipientIDs(m LiveMatch) []string {
	ids := []string{m.CreatedBy}
	if m.PlayerAUserID != nil {
		ids = append(ids, *m.PlayerAUserID)
	}
	if m.PlayerBUserID != nil {
		ids = append(ids, *m.PlayerBUserID)
	}
	return ids
}

func opponentUserID(starterID string, m LiveMatch) string {
	for _, id := range []*string{m.PlayerAUserID, m.PlayerBUserID} {
		if id != nil && *id != "" && *id != starterID {
			return *id
		}
	}
	return ""
}
