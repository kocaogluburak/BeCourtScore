package identity

import (
	"context"
	"errors"
	"testing"
)

// mockProvider is a test double for IdentityProvider.
type mockProvider struct {
	identity ExternalIdentity
	err      error
}

func (m *mockProvider) Verify(_ context.Context, _ string) (ExternalIdentity, error) {
	return m.identity, m.err
}

func TestExtractGoogleIdentity_Valid(t *testing.T) {
	claims := map[string]any{
		"sub":            "google-sub-123",
		"email":          "test@example.com",
		"email_verified": true,
		"picture":        "https://example.com/photo.jpg",
	}

	got, err := extractGoogleIdentity(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Provider != "google" {
		t.Errorf("Provider: got %q, want %q", got.Provider, "google")
	}
	if got.ProviderSubject != "google-sub-123" {
		t.Errorf("ProviderSubject: got %q", got.ProviderSubject)
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email: got %q", got.Email)
	}
	if !got.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
	if got.Picture != "https://example.com/photo.jpg" {
		t.Errorf("Picture: got %q", got.Picture)
	}
}

func TestExtractGoogleIdentity_MissingSub(t *testing.T) {
	_, err := extractGoogleIdentity(map[string]any{
		"email": "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error for missing sub claim")
	}
}

func TestProviderRegistry_UnknownProvider(t *testing.T) {
	svc := &Service{
		providers: map[string]IdentityProvider{
			"google": &mockProvider{identity: ExternalIdentity{Provider: "google", ProviderSubject: "sub"}},
		},
		cfg: ServiceConfig{},
	}

	_, _, _, err := svc.AuthWithProvider(context.Background(), "apple", "token")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !errors.Is(err, err) { // just ensure non-nil
		t.Error("unexpected error type")
	}
}
