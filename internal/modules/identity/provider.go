package identity

import "context"

// ExternalIdentity is the normalised result from any OAuth/social provider.
type ExternalIdentity struct {
	Provider        string // e.g. "google", "apple"
	ProviderSubject string // provider-specific unique user ID
	Email           string
	EmailVerified   bool
	Picture         string // optional profile picture URL
}

// IdentityProvider verifies a provider-specific credential and returns a
// normalised ExternalIdentity. Implement this interface to add new providers
// (Apple, email/OTP, …) without changing the core service.
type IdentityProvider interface {
	Verify(ctx context.Context, credential string) (ExternalIdentity, error)
}
