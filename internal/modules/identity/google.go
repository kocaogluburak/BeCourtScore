package identity

import (
	"context"
	"fmt"

	"google.golang.org/api/idtoken"
)

// GoogleProvider verifies Google ID tokens and returns an ExternalIdentity.
// It tries each configured audience (Android client ID, iOS client ID, …)
// until one succeeds, matching how Google issues tokens per-platform.
type GoogleProvider struct {
	audiences []string
}

func NewGoogleProvider(audiences []string) *GoogleProvider {
	return &GoogleProvider{audiences: audiences}
}

func (g *GoogleProvider) Verify(ctx context.Context, idToken string) (ExternalIdentity, error) {
	if len(g.audiences) == 0 {
		return ExternalIdentity{}, fmt.Errorf("no google client IDs configured")
	}

	var lastErr error
	for _, aud := range g.audiences {
		payload, err := idtoken.Validate(ctx, idToken, aud)
		if err != nil {
			lastErr = err
			continue
		}
		return extractGoogleIdentity(payload.Claims)
	}
	return ExternalIdentity{}, fmt.Errorf("google token validation failed: %w", lastErr)
}

func extractGoogleIdentity(claims map[string]any) (ExternalIdentity, error) {
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return ExternalIdentity{}, fmt.Errorf("missing sub claim")
	}
	email, _ := claims["email"].(string)
	emailVerified, _ := claims["email_verified"].(bool)
	picture, _ := claims["picture"].(string)

	return ExternalIdentity{
		Provider:        "google",
		ProviderSubject: sub,
		Email:           email,
		EmailVerified:   emailVerified,
		Picture:         picture,
	}, nil
}
