package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
)

type fakeStore struct {
	byIdentity map[string]User // key: provider\0subject
	byID       map[string]User
	tokens     map[string]string // hash → userID
	createErr  error
	consumeErr error
	revokeErr  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byIdentity: map[string]User{},
		byID:       map[string]User{},
		tokens:     map[string]string{},
	}
}

func identityKey(provider, subject string) string { return provider + "\x00" + subject }

func (f *fakeStore) findUserByIdentity(_ context.Context, provider, subject string) (User, bool, error) {
	u, ok := f.byIdentity[identityKey(provider, subject)]
	return u, ok, nil
}

func (f *fakeStore) createUserWithIdentity(_ context.Context, ext ExternalIdentity) (User, error) {
	if f.createErr != nil {
		return User{}, f.createErr
	}
	email := ext.Email
	u := User{
		ID:            "user-" + ext.ProviderSubject,
		Email:         &email,
		EmailVerified: ext.EmailVerified,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if ext.GivenName != "" {
		v := ext.GivenName
		u.Name = &v
	}
	if ext.FamilyName != "" {
		v := ext.FamilyName
		u.Surname = &v
	}
	if ext.Picture != "" {
		v := ext.Picture
		u.ProfileIcon = &v
	}
	f.byID[u.ID] = u
	f.byIdentity[identityKey(ext.Provider, ext.ProviderSubject)] = u
	return u, nil
}

func (f *fakeStore) getUserByID(_ context.Context, id string) (User, error) {
	u, ok := f.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) updateUser(_ context.Context, id string, in UpdateInput) (User, error) {
	u, ok := f.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	if in.Name != nil {
		u.Name = in.Name
	}
	if in.Surname != nil {
		u.Surname = in.Surname
	}
	if in.ProfileIcon != nil {
		u.ProfileIcon = in.ProfileIcon
	}
	if in.Nickname != nil {
		u.Nickname = in.Nickname
	}
	f.byID[id] = u
	for k, v := range f.byIdentity {
		if v.ID == id {
			f.byIdentity[k] = u
		}
	}
	return u, nil
}

func (f *fakeStore) deleteUser(_ context.Context, id string) error {
	u, ok := f.byID[id]
	if !ok {
		return ErrNotFound
	}
	delete(f.byID, id)
	for k, v := range f.byIdentity {
		if v.ID == id {
			delete(f.byIdentity, k)
		}
	}
	for hash, uid := range f.tokens {
		if uid == u.ID {
			delete(f.tokens, hash)
		}
	}
	return nil
}

func (f *fakeStore) saveRefreshToken(_ context.Context, userID, hash string, _ time.Time) error {
	f.tokens[hash] = userID
	return nil
}

func (f *fakeStore) consumeRefreshToken(_ context.Context, hash string) (string, error) {
	if f.consumeErr != nil {
		return "", f.consumeErr
	}
	userID, ok := f.tokens[hash]
	if !ok {
		return "", ErrInvalidToken
	}
	delete(f.tokens, hash)
	return userID, nil
}

func (f *fakeStore) revokeRefreshToken(_ context.Context, hash string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	delete(f.tokens, hash)
	return nil
}

func testService(store *fakeStore) *Service {
	svc := &Service{
		repo:      store,
		providers: map[string]IdentityProvider{},
		cfg: ServiceConfig{
			JWTSecret:       []byte("service-test-secret"),
			AccessTokenTTL:  time.Hour,
			RefreshTokenTTL: 24 * time.Hour,
		},
	}
	svc.RegisterProvider("google", &mockProvider{
		identity: ExternalIdentity{
			Provider:        "google",
			ProviderSubject: "sub-1",
			Email:           "ada@example.com",
			EmailVerified:   true,
			GivenName:       "Ada",
			FamilyName:      "Lovelace",
		},
	})
	return svc
}

func TestAuthWithProvider_CreatesNewUserAndSession(t *testing.T) {
	store := newFakeStore()
	svc := testService(store)

	session, user, isNew, err := svc.AuthWithProvider(context.Background(), "google", "id-token")
	if err != nil {
		t.Fatalf("AuthWithProvider: %v", err)
	}
	if !isNew {
		t.Fatal("expected new user")
	}
	if user.ID == "" {
		t.Fatal("expected user id")
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("expected session tokens")
	}
	if session.ExpiresIn <= 0 {
		t.Fatalf("expires_in: %d", session.ExpiresIn)
	}
	if len(store.tokens) != 1 {
		t.Fatalf("expected 1 refresh token stored, got %d", len(store.tokens))
	}
}

func TestAuthWithProvider_ExistingUser(t *testing.T) {
	store := newFakeStore()
	svc := testService(store)

	_, first, _, err := svc.AuthWithProvider(context.Background(), "google", "t1")
	if err != nil {
		t.Fatalf("first auth: %v", err)
	}
	session, second, isNew, err := svc.AuthWithProvider(context.Background(), "google", "t2")
	if err != nil {
		t.Fatalf("second auth: %v", err)
	}
	if isNew {
		t.Fatal("expected existing user")
	}
	if first.ID != second.ID {
		t.Fatalf("user id changed: %q vs %q", first.ID, second.ID)
	}
	if session.AccessToken == "" {
		t.Fatal("expected access token")
	}
}

func TestAuthWithProvider_UnknownProvider(t *testing.T) {
	svc := testService(newFakeStore())
	_, _, _, err := svc.AuthWithProvider(context.Background(), "apple", "tok")
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown provider error, got %v", err)
	}
}

func TestAuthWithProvider_VerifyFailure(t *testing.T) {
	store := newFakeStore()
	svc := testService(store)
	svc.RegisterProvider("google", &mockProvider{err: errors.New("bad token")})

	_, _, _, err := svc.AuthWithProvider(context.Background(), "google", "tok")
	if err == nil {
		t.Fatal("expected verify error")
	}
}

func TestRefreshSession_RotatesToken(t *testing.T) {
	store := newFakeStore()
	svc := testService(store)

	first, user, _, err := svc.AuthWithProvider(context.Background(), "google", "tok")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	oldHash := authkit.HashRefreshToken(first.RefreshToken)

	second, gotUser, err := svc.RefreshSession(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if gotUser.ID != user.ID {
		t.Fatalf("user: got %q want %q", gotUser.ID, user.ID)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("expected rotated refresh token")
	}
	if _, ok := store.tokens[oldHash]; ok {
		t.Fatal("old refresh token should be consumed")
	}
	if len(store.tokens) != 1 {
		t.Fatalf("expected 1 active token, got %d", len(store.tokens))
	}
}

func TestRefreshSession_InvalidToken(t *testing.T) {
	svc := testService(newFakeStore())
	_, _, err := svc.RefreshSession(context.Background(), "not-a-token")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRevokeSession_RemovesToken(t *testing.T) {
	store := newFakeStore()
	svc := testService(store)

	session, _, _, err := svc.AuthWithProvider(context.Background(), "google", "tok")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := svc.RevokeSession(context.Background(), session.RefreshToken); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(store.tokens) != 0 {
		t.Fatalf("expected tokens cleared, got %d", len(store.tokens))
	}
	_, _, err = svc.RefreshSession(context.Background(), session.RefreshToken)
	if err == nil {
		t.Fatal("expected refresh to fail after revoke")
	}
}

func TestDeleteUser_RemovesAccount(t *testing.T) {
	store := newFakeStore()
	svc := testService(store)

	_, user, _, err := svc.AuthWithProvider(context.Background(), "google", "tok")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := svc.DeleteUser(context.Background(), user.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := svc.GetUser(context.Background(), user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if len(store.byID) != 0 {
		t.Fatalf("expected byID empty, got %d", len(store.byID))
	}
	if len(store.tokens) != 0 {
		t.Fatalf("expected tokens cleared, got %d", len(store.tokens))
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	svc := testService(newFakeStore())
	err := svc.DeleteUser(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
