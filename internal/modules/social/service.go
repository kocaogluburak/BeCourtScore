package social

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the social domain's business logic layer.
type Service struct {
	repo *repo
}

// NewService creates a Service wired to the given Postgres pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{repo: &repo{pool: pool}}
}

// AreFriends reports whether two users have an accepted friendship.
// Also implements score.FriendChecker.
func (s *Service) AreFriends(ctx context.Context, a, b string) (bool, error) {
	return s.repo.areFriends(ctx, a, b)
}

// SearchUsers finds users by nickname/name prefix or exact email.
func (s *Service) SearchUsers(ctx context.Context, viewerID, query string, limit, offset int) ([]SearchResult, int64, error) {
	if len(query) < 2 {
		return nil, 0, fmt.Errorf("%w: query must be at least 2 characters", ErrInvalid)
	}
	return s.repo.searchUsers(ctx, viewerID, query, limit, offset)
}

// GetUserProfile returns a public profile. Allowed for self or accepted friends.
func (s *Service) GetUserProfile(ctx context.Context, viewerID, targetID string) (UserSummary, error) {
	if viewerID != targetID {
		ok, err := s.repo.areFriends(ctx, viewerID, targetID)
		if err != nil {
			return UserSummary{}, err
		}
		if !ok {
			return UserSummary{}, ErrForbidden
		}
	}
	return s.repo.getUserSummary(ctx, targetID)
}

// SendRequest creates (or re-opens) a friend request to addresseeID.
func (s *Service) SendRequest(ctx context.Context, requesterID, addresseeID string) (Friendship, error) {
	if requesterID == addresseeID {
		return Friendship{}, fmt.Errorf("%w: cannot friend yourself", ErrInvalid)
	}
	if _, err := s.repo.getUserSummary(ctx, addresseeID); err != nil {
		return Friendship{}, err // ErrNotFound when the target doesn't exist
	}

	existing, err := s.repo.findBetween(ctx, requesterID, addresseeID)
	switch {
	case err == nil:
		switch existing.Status {
		case "accepted":
			return Friendship{}, fmt.Errorf("%w: already friends", ErrConflict)
		case "pending":
			return Friendship{}, fmt.Errorf("%w: request already pending", ErrConflict)
		default: // rejected → re-open with the new direction
			return s.repo.reopenRequest(ctx, existing.ID, requesterID, addresseeID)
		}
	case err == ErrNotFound:
		return s.repo.insertRequest(ctx, requesterID, addresseeID)
	default:
		return Friendship{}, err
	}
}

// AcceptRequest transitions a pending request to accepted. Addressee only.
func (s *Service) AcceptRequest(ctx context.Context, userID, requestID string) (Friendship, error) {
	return s.repo.updateStatus(ctx, requestID, userID, "accepted")
}

// RejectRequest transitions a pending request to rejected. Addressee only.
func (s *Service) RejectRequest(ctx context.Context, userID, requestID string) (Friendship, error) {
	return s.repo.updateStatus(ctx, requestID, userID, "rejected")
}

// ListFriends returns the user's accepted friends.
func (s *Service) ListFriends(ctx context.Context, userID string, limit, offset int) ([]UserSummary, int64, error) {
	return s.repo.listFriends(ctx, userID, limit, offset)
}

// ListIncomingRequests returns pending requests addressed to the user.
func (s *Service) ListIncomingRequests(ctx context.Context, userID string, limit, offset int) ([]IncomingRequest, int64, error) {
	return s.repo.listIncomingRequests(ctx, userID, limit, offset)
}

// Unfriend removes an accepted friendship between userID and otherID.
func (s *Service) Unfriend(ctx context.Context, userID, otherID string) error {
	return s.repo.deleteAccepted(ctx, userID, otherID)
}
