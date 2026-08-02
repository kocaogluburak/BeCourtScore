package social

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserSummary is the public projection of a user (no email).
type UserSummary struct {
	ID          string  `json:"id"`
	Nickname    *string `json:"nickname"`
	Name        *string `json:"name"`
	Surname     *string `json:"surname"`
	ProfileIcon *string `json:"profile_icon"`
}

// UserProfile is returned by GET /v1/users/{userID} for any authenticated
// viewer. FriendshipStatus is one of: "self" | "accepted" |
// "pending_sent" | "pending_received" | "none".
type UserProfile struct {
	UserSummary
	FriendshipStatus string `json:"friendship_status"`
}

// SearchResult is a user search hit annotated with the viewer's
// friendship status: none | pending_sent | pending_received | accepted.
// EmailMasked is a privacy-preserving hint so same-name users can be told apart.
type SearchResult struct {
	UserSummary
	EmailMasked      string `json:"email_masked,omitempty"`
	FriendshipStatus string `json:"friendship_status"`
}

// Friendship is a raw friendship row.
type Friendship struct {
	ID          string    `json:"id"`
	RequesterID string    `json:"requester_id"`
	AddresseeID string    `json:"addressee_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IncomingRequest is a pending request shown to the addressee.
type IncomingRequest struct {
	ID        string      `json:"id"`
	Requester UserSummary `json:"requester"`
	CreatedAt time.Time   `json:"created_at"`
}

type repo struct{ pool *pgxpool.Pool }

const userCols = `id, nickname, name, surname, profile_icon`

func (r *repo) getUserSummary(ctx context.Context, id string) (UserSummary, error) {
	var u UserSummary
	err := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Nickname, &u.Name, &u.Surname, &u.ProfileIcon)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSummary{}, ErrNotFound
	}
	if err != nil {
		return UserSummary{}, fmt.Errorf("getUserSummary: %w", err)
	}
	return u, nil
}

// searchUsers matches nickname/name/surname prefixes (case-insensitive),
// "name surname" full-name prefix, or exact email when the query contains '@'.
// Results include a masked email.
func (r *repo) searchUsers(ctx context.Context, viewerID, query string, limit, offset int) ([]SearchResult, int64, error) {
	query = strings.TrimSpace(query)
	var where string
	var pattern string
	if looksLikeEmail(query) {
		where = `u.id <> $1 AND LOWER(u.email) = LOWER($2)`
		pattern = query
	} else {
		where = `u.id <> $1 AND (
		          LOWER(COALESCE(u.nickname, '')) LIKE LOWER($2) || '%' ESCAPE '\'
		          OR LOWER(COALESCE(u.name, '')) LIKE LOWER($2) || '%' ESCAPE '\'
		          OR LOWER(COALESCE(u.surname, '')) LIKE LOWER($2) || '%' ESCAPE '\'
		          OR LOWER(TRIM(BOTH FROM COALESCE(u.name, '') || ' ' || COALESCE(u.surname, '')))
		             LIKE LOWER($2) || '%' ESCAPE '\'
		        )`
		pattern = escapeLike(query)
	}

	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users u WHERE `+where, viewerID, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Join via unordered-pair unique index instead of an OR bidirectional join.
	q := `
		SELECT u.id, u.nickname, u.name, u.surname, u.profile_icon, u.email,
		       f.status, f.requester_id
		FROM users u
		LEFT JOIN friendships f
		  ON LEAST(f.requester_id, f.addressee_id) = LEAST($1::uuid, u.id)
		 AND GREATEST(f.requester_id, f.addressee_id) = GREATEST($1::uuid, u.id)
		WHERE ` + where + `
		ORDER BY LOWER(COALESCE(u.nickname, u.name, '')) ASC
		LIMIT $3 OFFSET $4`

	rows, err := r.pool.Query(ctx, q, viewerID, pattern, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var res SearchResult
		var email string
		var status, requesterID *string
		if err := rows.Scan(&res.ID, &res.Nickname, &res.Name, &res.Surname, &res.ProfileIcon, &email,
			&status, &requesterID); err != nil {
			return nil, 0, fmt.Errorf("scan search result: %w", err)
		}
		res.EmailMasked = maskEmail(email)
		res.FriendshipStatus = deriveStatus(status, requesterID, viewerID)
		results = append(results, res)
	}
	return results, total, rows.Err()
}

func deriveStatus(status, requesterID *string, viewerID string) string {
	if status == nil {
		return "none"
	}
	switch *status {
	case "accepted":
		return "accepted"
	case "pending":
		if requesterID != nil && *requesterID == viewerID {
			return "pending_sent"
		}
		return "pending_received"
	default: // rejected → requester may re-request
		return "none"
	}
}

// findBetween returns the friendship row between two users in either direction.
func (r *repo) findBetween(ctx context.Context, a, b string) (Friendship, error) {
	// Uses uq_friendships_pair (LEAST/GREATEST) instead of an OR scan.
	const q = `
		SELECT id, requester_id, addressee_id, status, created_at, updated_at
		FROM friendships
		WHERE LEAST(requester_id, addressee_id) = LEAST($1::uuid, $2::uuid)
		  AND GREATEST(requester_id, addressee_id) = GREATEST($1::uuid, $2::uuid)`
	var f Friendship
	err := r.pool.QueryRow(ctx, q, a, b).
		Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Friendship{}, ErrNotFound
	}
	if err != nil {
		return Friendship{}, fmt.Errorf("findBetween: %w", err)
	}
	return f, nil
}

func (r *repo) insertRequest(ctx context.Context, requesterID, addresseeID string) (Friendship, error) {
	const q = `
		INSERT INTO friendships (requester_id, addressee_id, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at`
	var f Friendship
	err := r.pool.QueryRow(ctx, q, requesterID, addresseeID).
		Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return Friendship{}, fmt.Errorf("insertRequest: %w", err)
	}
	return f, nil
}

// reopenRequest turns a rejected row back into a pending one, possibly
// flipping direction to the new requester.
func (r *repo) reopenRequest(ctx context.Context, id, requesterID, addresseeID string) (Friendship, error) {
	const q = `
		UPDATE friendships
		SET requester_id = $2, addressee_id = $3, status = 'pending', updated_at = NOW()
		WHERE id = $1
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at`
	var f Friendship
	err := r.pool.QueryRow(ctx, q, id, requesterID, addresseeID).
		Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return Friendship{}, fmt.Errorf("reopenRequest: %w", err)
	}
	return f, nil
}

// updateStatus transitions a pending request; only the addressee may do so.
func (r *repo) updateStatus(ctx context.Context, id, addresseeID, status string) (Friendship, error) {
	const q = `
		UPDATE friendships
		SET status = $3, updated_at = NOW()
		WHERE id = $1 AND addressee_id = $2 AND status = 'pending'
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at`
	var f Friendship
	err := r.pool.QueryRow(ctx, q, id, addresseeID, status).
		Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Friendship{}, ErrNotFound
	}
	if err != nil {
		return Friendship{}, fmt.Errorf("updateStatus: %w", err)
	}
	return f, nil
}

// listFriends returns accepted friends of userID with the total count.
// Uses UNION (not ALL) so bidirectional accepted rows cannot duplicate a friend.
func (r *repo) listFriends(ctx context.Context, userID string, limit, offset int) ([]UserSummary, int64, error) {
	const friendIDs = `
		SELECT addressee_id AS friend_id FROM friendships
		WHERE requester_id = $1 AND status = 'accepted'
		UNION
		SELECT requester_id AS friend_id FROM friendships
		WHERE addressee_id = $1 AND status = 'accepted'`

	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM (`+friendIDs+`) f`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count friends: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.nickname, u.name, u.surname, u.profile_icon
		FROM (`+friendIDs+`) f
		JOIN users u ON u.id = f.friend_id
		ORDER BY LOWER(COALESCE(u.nickname, u.name, '')) ASC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()

	friends := []UserSummary{}
	for rows.Next() {
		var u UserSummary
		if err := rows.Scan(&u.ID, &u.Nickname, &u.Name, &u.Surname, &u.ProfileIcon); err != nil {
			return nil, 0, fmt.Errorf("scan friend: %w", err)
		}
		friends = append(friends, u)
	}
	return friends, total, rows.Err()
}

// listIncomingRequests returns pending requests addressed to userID.
func (r *repo) listIncomingRequests(ctx context.Context, userID string, limit, offset int) ([]IncomingRequest, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM friendships WHERE addressee_id = $1 AND status = 'pending'`,
		userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count requests: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT f.id, f.created_at, u.id, u.nickname, u.name, u.surname, u.profile_icon
		FROM friendships f
		JOIN users u ON u.id = f.requester_id
		WHERE f.addressee_id = $1 AND f.status = 'pending'
		ORDER BY f.created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list requests: %w", err)
	}
	defer rows.Close()

	reqs := []IncomingRequest{}
	for rows.Next() {
		var req IncomingRequest
		if err := rows.Scan(&req.ID, &req.CreatedAt,
			&req.Requester.ID, &req.Requester.Nickname, &req.Requester.Name,
			&req.Requester.Surname, &req.Requester.ProfileIcon); err != nil {
			return nil, 0, fmt.Errorf("scan request: %w", err)
		}
		reqs = append(reqs, req)
	}
	return reqs, total, rows.Err()
}

// deleteAccepted removes an accepted friendship between two users.
func (r *repo) deleteAccepted(ctx context.Context, a, b string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM friendships
		WHERE status = 'accepted'
		  AND ((requester_id = $1 AND addressee_id = $2)
		    OR (requester_id = $2 AND addressee_id = $1))`, a, b)
	if err != nil {
		return fmt.Errorf("deleteAccepted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// areFriends reports whether an accepted friendship exists between two users.
func (r *repo) areFriends(ctx context.Context, a, b string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM friendships
			WHERE status = 'accepted'
			  AND LEAST(requester_id, addressee_id) = LEAST($1::uuid, $2::uuid)
			  AND GREATEST(requester_id, addressee_id) = GREATEST($1::uuid, $2::uuid))`, a, b).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("areFriends: %w", err)
	}
	return exists, nil
}

// Sentinel errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid input")
)
