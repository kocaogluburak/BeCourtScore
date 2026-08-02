package identity

import (
	"context"
	"fmt"
	"time"

	"courtscore/internal/platform/authkit"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Session holds the tokens returned after successful auth.
type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
}

// ServiceConfig holds tunable parameters for the Service.
type ServiceConfig struct {
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// userStore is the persistence surface used by Service.
// The concrete *repo implements it; tests substitute an in-memory fake.
type userStore interface {
	findUserByIdentity(ctx context.Context, provider, subject string) (User, bool, error)
	createUserWithIdentity(ctx context.Context, ext ExternalIdentity) (User, error)
	getUserByID(ctx context.Context, id string) (User, error)
	updateUser(ctx context.Context, id string, in UpdateInput) (User, error)
	saveRefreshToken(ctx context.Context, userID, hash string, expiresAt time.Time) error
	consumeRefreshToken(ctx context.Context, hash string) (string, error)
	revokeRefreshToken(ctx context.Context, hash string) error
}

// Service is the identity domain's business logic layer.
type Service struct {
	repo      userStore
	providers map[string]IdentityProvider
	cfg       ServiceConfig
}

// NewService creates a Service wired to the given Postgres pool.
func NewService(pool *pgxpool.Pool, cfg ServiceConfig) *Service {
	return &Service{
		repo:      &repo{pool: pool},
		providers: make(map[string]IdentityProvider),
		cfg:       cfg,
	}
}

// RegisterProvider registers an IdentityProvider under a name (e.g. "google").
// Call this for each provider before handling requests.
func (s *Service) RegisterProvider(name string, p IdentityProvider) {
	s.providers[name] = p
}

// AuthWithProvider verifies an external credential, upserts the user and
// returns a fresh session. isNewUser is true when the auth_identity was just created.
func (s *Service) AuthWithProvider(ctx context.Context, providerName, credential string) (Session, User, bool, error) {
	p, ok := s.providers[providerName]
	if !ok {
		return Session{}, User{}, false, fmt.Errorf("unknown provider: %s", providerName)
	}

	ext, err := p.Verify(ctx, credential)
	if err != nil {
		return Session{}, User{}, false, fmt.Errorf("verify credential: %w", err)
	}

	user, found, err := s.repo.findUserByIdentity(ctx, ext.Provider, ext.ProviderSubject)
	if err != nil {
		return Session{}, User{}, false, err
	}

	isNewUser := !found
	if isNewUser {
		user, err = s.repo.createUserWithIdentity(ctx, ext)
		if err != nil {
			return Session{}, User{}, false, err
		}
	} else {
		// Backfill name/surname/picture from provider if the user hasn't set them yet.
		var backfill UpdateInput
		if user.Name == nil && ext.GivenName != "" {
			v := ext.GivenName
			backfill.Name = &v
		}
		if user.Surname == nil && ext.FamilyName != "" {
			v := ext.FamilyName
			backfill.Surname = &v
		}
		if (user.ProfileIcon == nil || *user.ProfileIcon == "") && ext.Picture != "" {
			v := ext.Picture
			backfill.ProfileIcon = &v
		}
		if backfill.Name != nil || backfill.Surname != nil || backfill.ProfileIcon != nil {
			if updated, updErr := s.repo.updateUser(ctx, user.ID, backfill); updErr == nil {
				user = updated
			}
		}
	}

	session, err := s.issueSession(ctx, user.ID)
	if err != nil {
		return Session{}, User{}, false, err
	}
	return session, user, isNewUser, nil
}

// RefreshSession rotates a refresh token and returns a new session.
func (s *Service) RefreshSession(ctx context.Context, rawRefreshToken string) (Session, User, error) {
	hash := authkit.HashRefreshToken(rawRefreshToken)
	userID, err := s.repo.consumeRefreshToken(ctx, hash)
	if err != nil {
		return Session{}, User{}, fmt.Errorf("refresh: %w", err)
	}

	user, err := s.repo.getUserByID(ctx, userID)
	if err != nil {
		return Session{}, User{}, err
	}

	session, err := s.issueSession(ctx, userID)
	if err != nil {
		return Session{}, User{}, err
	}
	return session, user, nil
}

// RevokeSession revokes a refresh token (logout).
func (s *Service) RevokeSession(ctx context.Context, rawRefreshToken string) error {
	hash := authkit.HashRefreshToken(rawRefreshToken)
	return s.repo.revokeRefreshToken(ctx, hash)
}

// GetUser returns the user record for the given ID.
func (s *Service) GetUser(ctx context.Context, userID string) (User, error) {
	return s.repo.getUserByID(ctx, userID)
}

// UpdateUser applies optional profile fields and returns the updated user.
func (s *Service) UpdateUser(ctx context.Context, userID string, in UpdateInput) (User, error) {
	return s.repo.updateUser(ctx, userID, in)
}

// issueSession mints an access token + refresh token and persists the hash.
func (s *Service) issueSession(ctx context.Context, userID string) (Session, error) {
	access, err := authkit.IssueAccessToken(userID, s.cfg.JWTSecret, s.cfg.AccessTokenTTL)
	if err != nil {
		return Session{}, fmt.Errorf("issue access token: %w", err)
	}

	rawRefresh, hash, err := authkit.GenerateRefreshToken()
	if err != nil {
		return Session{}, fmt.Errorf("generate refresh token: %w", err)
	}

	expiresAt := time.Now().Add(s.cfg.RefreshTokenTTL)
	if err = s.repo.saveRefreshToken(ctx, userID, hash, expiresAt); err != nil {
		return Session{}, fmt.Errorf("save refresh token: %w", err)
	}

	return Session{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		ExpiresIn:    int(s.cfg.AccessTokenTTL.Seconds()),
	}, nil
}
