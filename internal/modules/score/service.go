package score

import (
	"context"
	"fmt"

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

// Service is the score domain's business logic layer.
type Service struct {
	repo    *repo
	friends FriendChecker
}

// NewService creates a Service wired to the given Postgres pool.
func NewService(pool *pgxpool.Pool, friends FriendChecker) *Service {
	return &Service{repo: &repo{pool: pool}, friends: friends}
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
