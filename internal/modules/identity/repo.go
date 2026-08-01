package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User is the application's user record.
type User struct {
	ID            string     `json:"id"`
	Email         *string    `json:"email"`
	EmailVerified bool       `json:"email_verified"`
	Nickname      *string    `json:"nickname"`
	Name          *string    `json:"name"`
	Surname       *string    `json:"surname"`
	ProfileIcon   *string    `json:"profile_icon"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// UpdateInput holds the optional profile fields a user can change.
type UpdateInput struct {
	Nickname    *string
	Name        *string
	Surname     *string
	ProfileIcon *string
}

type repo struct{ pool *pgxpool.Pool }

// findUserByIdentity looks up a user via the auth_identities table.
// Returns (user, true, nil) if found, (User{}, false, nil) if not found.
func (r *repo) findUserByIdentity(ctx context.Context, provider, subject string) (User, bool, error) {
	const q = `
		SELECT u.id, u.email, u.email_verified, u.nickname, u.name, u.surname,
		       u.profile_icon, u.created_at, u.updated_at
		FROM auth_identities ai
		JOIN users u ON u.id = ai.user_id
		WHERE ai.provider = $1 AND ai.provider_subject = $2`

	var u User
	err := r.pool.QueryRow(ctx, q, provider, subject).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &u.Nickname, &u.Name, &u.Surname,
			&u.ProfileIcon, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("findUserByIdentity: %w", err)
	}
	return u, true, nil
}

// createUserWithIdentity inserts a new user and its auth_identity in one transaction.
func (r *repo) createUserWithIdentity(ctx context.Context, ext ExternalIdentity) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var u User
	var emailArg *string
	if ext.Email != "" {
		emailArg = &ext.Email
	}
	var iconArg *string
	if ext.Picture != "" {
		iconArg = &ext.Picture
	}
	var nameArg *string
	if ext.GivenName != "" {
		nameArg = &ext.GivenName
	}
	var surnameArg *string
	if ext.FamilyName != "" {
		surnameArg = &ext.FamilyName
	}

	const insertUser = `
		INSERT INTO users (email, email_verified, profile_icon, name, surname)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, email_verified, nickname, name, surname,
		          profile_icon, created_at, updated_at`

	err = tx.QueryRow(ctx, insertUser, emailArg, ext.EmailVerified, iconArg, nameArg, surnameArg).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &u.Nickname, &u.Name, &u.Surname,
			&u.ProfileIcon, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}

	const insertIdentity = `
		INSERT INTO auth_identities (user_id, provider, provider_subject)
		VALUES ($1, $2, $3)`
	if _, err = tx.Exec(ctx, insertIdentity, u.ID, ext.Provider, ext.ProviderSubject); err != nil {
		return User{}, fmt.Errorf("insert auth_identity: %w", err)
	}

	return u, tx.Commit(ctx)
}

// getUserByID fetches a user by primary key.
func (r *repo) getUserByID(ctx context.Context, id string) (User, error) {
	const q = `
		SELECT id, email, email_verified, nickname, name, surname,
		       profile_icon, created_at, updated_at
		FROM users WHERE id = $1`

	var u User
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &u.Nickname, &u.Name, &u.Surname,
			&u.ProfileIcon, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("getUserByID: %w", err)
	}
	return u, nil
}

// updateUser applies non-nil fields from UpdateInput.
func (r *repo) updateUser(ctx context.Context, id string, in UpdateInput) (User, error) {
	const q = `
		UPDATE users SET
			nickname     = COALESCE($2, nickname),
			name         = COALESCE($3, name),
			surname      = COALESCE($4, surname),
			profile_icon = COALESCE($5, profile_icon),
			updated_at   = NOW()
		WHERE id = $1
		RETURNING id, email, email_verified, nickname, name, surname,
		          profile_icon, created_at, updated_at`

	var u User
	err := r.pool.QueryRow(ctx, q, id, in.Nickname, in.Name, in.Surname, in.ProfileIcon).
		Scan(&u.ID, &u.Email, &u.EmailVerified, &u.Nickname, &u.Name, &u.Surname,
			&u.ProfileIcon, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return User{}, fmt.Errorf("%w: nickname already taken", ErrConflict)
	}
	if err != nil {
		return User{}, fmt.Errorf("updateUser: %w", err)
	}
	return u, nil
}

// saveRefreshToken stores a hashed refresh token.
func (r *repo) saveRefreshToken(ctx context.Context, userID, hash string, expiresAt time.Time) error {
	const q = `INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, userID, hash, expiresAt)
	return err
}

// consumeRefreshToken looks up a valid (non-expired, non-revoked) refresh token
// and atomically revokes it. Returns the associated userID.
func (r *repo) consumeRefreshToken(ctx context.Context, hash string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	const q = `
		SELECT user_id FROM refresh_tokens
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()`
	if err = tx.QueryRow(ctx, q, hash).Scan(&userID); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidToken
	} else if err != nil {
		return "", fmt.Errorf("consumeRefreshToken query: %w", err)
	}

	if _, err = tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=NOW() WHERE token_hash=$1`, hash); err != nil {
		return "", fmt.Errorf("revoke token: %w", err)
	}
	return userID, tx.Commit(ctx)
}

// revokeRefreshToken marks a token as revoked (logout).
func (r *repo) revokeRefreshToken(ctx context.Context, hash string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at=NOW() WHERE token_hash=$1 AND revoked_at IS NULL`,
		hash)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Sentinel errors.
var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidToken = errors.New("invalid or expired refresh token")
)
