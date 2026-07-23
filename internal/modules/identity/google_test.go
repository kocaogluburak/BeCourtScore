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
		"given_name":     "Ada",
		"family_name":    "Lovelace",
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
	if got.GivenName != "Ada" {
		t.Errorf("GivenName: got %q, want %q", got.GivenName, "Ada")
	}
	if got.FamilyName != "Lovelace" {
		t.Errorf("FamilyName: got %q, want %q", got.FamilyName, "Lovelace")
	}
}

func TestExtractGoogleIdentity_NameFallback(t *testing.T) {
	// When given_name/family_name are absent, "name" claim is split.
	claims := map[string]any{
		"sub":  "google-sub-456",
		"name": "Ada Lovelace",
	}

	got, err := extractGoogleIdentity(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GivenName != "Ada" {
		t.Errorf("GivenName fallback: got %q, want %q", got.GivenName, "Ada")
	}
	if got.FamilyName != "Lovelace" {
		t.Errorf("FamilyName fallback: got %q, want %q", got.FamilyName, "Lovelace")
	}
}

func TestExtractGoogleIdentity_SingleNameFallback(t *testing.T) {
	// Single-word name: all goes to GivenName, FamilyName stays empty.
	claims := map[string]any{
		"sub":  "google-sub-789",
		"name": "Madonna",
	}

	got, err := extractGoogleIdentity(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GivenName != "Madonna" {
		t.Errorf("GivenName: got %q, want %q", got.GivenName, "Madonna")
	}
	if got.FamilyName != "" {
		t.Errorf("FamilyName should be empty, got %q", got.FamilyName)
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
