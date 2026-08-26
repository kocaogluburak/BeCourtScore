package identity

import "testing"

func TestExtractAppleIdentity_Valid(t *testing.T) {
	claims := map[string]any{
		"sub":            "001234.abcdef.5678",
		"email":          "user@privaterelay.appleid.com",
		"email_verified": "true",
	}

	got, err := extractAppleIdentity(claims)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Provider != "apple" {
		t.Errorf("Provider: got %q, want %q", got.Provider, "apple")
	}
	if got.ProviderSubject != "001234.abcdef.5678" {
		t.Errorf("ProviderSubject: got %q", got.ProviderSubject)
	}
	if got.Email != "user@privaterelay.appleid.com" {
		t.Errorf("Email: got %q", got.Email)
	}
	if !got.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
}

func TestExtractAppleIdentity_BoolEmailVerified(t *testing.T) {
	got, err := extractAppleIdentity(map[string]any{
		"sub":            "sub-1",
		"email_verified": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.EmailVerified {
		t.Error("expected EmailVerified=true for real bool claim")
	}
}

func TestExtractAppleIdentity_MissingSub(t *testing.T) {
	_, err := extractAppleIdentity(map[string]any{
		"email": "user@example.com",
	})
	if err == nil {
		t.Fatal("expected error for missing sub claim")
	}
}

func TestParseAppleBool(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"string true", "true", true},
		{"string false", "false", false},
		{"bool true", true, true},
		{"bool false", false, false},
		{"nil", nil, false},
		{"other", 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseAppleBool(c.in); got != c.want {
				t.Errorf("parseAppleBool(%v): got %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestAudienceAllowed(t *testing.T) {
	allowed := []string{"com.court.score", "com.court.score.watchkitapp"}

	if !audienceAllowed([]string{"com.court.score"}, allowed) {
		t.Error("expected matching audience to be allowed")
	}
	if audienceAllowed([]string{"com.other.app"}, allowed) {
		t.Error("expected non-matching audience to be rejected")
	}
	if audienceAllowed(nil, allowed) {
		t.Error("expected empty audience to be rejected")
	}
}

func TestRSAPublicKeyFromJWK_Invalid(t *testing.T) {
	if _, err := rsaPublicKeyFromJWK("!!!not-base64!!!", "AQAB"); err == nil {
		t.Error("expected error for invalid modulus encoding")
	}
	if _, err := rsaPublicKeyFromJWK("", ""); err == nil {
		t.Error("expected error for empty modulus/exponent")
	}
}
