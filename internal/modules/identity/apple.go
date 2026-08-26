package identity

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleIssuer  = "https://appleid.apple.com"
	appleKeysURL = "https://appleid.apple.com/auth/keys"
)

// AppleProvider verifies "Sign in with Apple" identity tokens (a signed JWT)
// against Apple's published public keys and returns an ExternalIdentity.
//
// Unlike Google, Apple only includes the user's name in the initial
// authorization response (not in the identity token), so GivenName/FamilyName
// are typically empty here and the user can set them later on the profile.
type AppleProvider struct {
	audiences []string
	keys      *appleKeySet
}

// NewAppleProvider builds a provider. audiences must contain the app's bundle
// identifier(s) used as the token audience (e.g. "com.court.score").
func NewAppleProvider(audiences []string) *AppleProvider {
	return &AppleProvider{
		audiences: audiences,
		keys:      newAppleKeySet(appleKeysURL),
	}
}

func (a *AppleProvider) Verify(ctx context.Context, idToken string) (ExternalIdentity, error) {
	if len(a.audiences) == 0 {
		return ExternalIdentity{}, fmt.Errorf("no apple client IDs configured")
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(appleIssuer),
	)

	claims := jwt.MapClaims{}
	_, err := parser.ParseWithClaims(idToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing kid header")
		}
		return a.keys.key(ctx, kid)
	})
	if err != nil {
		return ExternalIdentity{}, fmt.Errorf("apple token validation failed: %w", err)
	}

	aud, err := claims.GetAudience()
	if err != nil || !audienceAllowed(aud, a.audiences) {
		return ExternalIdentity{}, fmt.Errorf("apple token audience mismatch")
	}

	return extractAppleIdentity(claims)
}

// audienceAllowed reports whether any token audience matches an allowed value.
func audienceAllowed(tokenAud []string, allowed []string) bool {
	for _, got := range tokenAud {
		for _, want := range allowed {
			if got == want {
				return true
			}
		}
	}
	return false
}

// extractAppleIdentity normalises the verified Apple claims. It is kept pure
// (no network / crypto) so it can be unit-tested directly.
func extractAppleIdentity(claims map[string]any) (ExternalIdentity, error) {
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return ExternalIdentity{}, fmt.Errorf("missing sub claim")
	}
	email, _ := claims["email"].(string)

	return ExternalIdentity{
		Provider:        "apple",
		ProviderSubject: sub,
		Email:           email,
		EmailVerified:   parseAppleBool(claims["email_verified"]),
	}, nil
}

// parseAppleBool handles Apple's habit of encoding booleans as either a real
// JSON bool or the strings "true"/"false".
func parseAppleBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	default:
		return false
	}
}

// ── Apple public key set (JWKS) ─────────────────────────────────────────────

// appleKeySet fetches and caches Apple's RSA public keys, refreshing on a TTL
// or when an unknown key id is requested (Apple rotates keys periodically).
type appleKeySet struct {
	url    string
	client *http.Client
	ttl    time.Duration

	mu     sync.RWMutex
	keys   map[string]*rsa.PublicKey
	expiry time.Time
}

func newAppleKeySet(url string) *appleKeySet {
	return &appleKeySet{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		ttl:    time.Hour,
		keys:   make(map[string]*rsa.PublicKey),
	}
}

func (s *appleKeySet) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	s.mu.RLock()
	k, ok := s.keys[kid]
	fresh := time.Now().Before(s.expiry)
	s.mu.RUnlock()
	if ok && fresh {
		return k, nil
	}

	if err := s.refresh(ctx); err != nil {
		if ok {
			return k, nil // network hiccup: fall back to a cached (stale) key
		}
		return nil, err
	}

	s.mu.RLock()
	k, ok = s.keys[kid]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown apple key id: %s", kid)
	}
	return k, nil
}

func (s *appleKeySet) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch apple keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch apple keys: unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
			Kty string `json:"kty"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode apple keys: %w", err)
	}

	next := make(map[string]*rsa.PublicKey, len(payload.Keys))
	for _, jwk := range payload.Keys {
		if jwk.Kty != "RSA" || jwk.Kid == "" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(jwk.N, jwk.E)
		if err != nil {
			continue
		}
		next[jwk.Kid] = pub
	}
	if len(next) == 0 {
		return fmt.Errorf("no usable apple keys returned")
	}

	s.mu.Lock()
	s.keys = next
	s.expiry = time.Now().Add(s.ttl)
	s.mu.Unlock()
	return nil
}

// rsaPublicKeyFromJWK builds an RSA public key from the base64url-encoded
// modulus (n) and exponent (e) of a JWK.
func rsaPublicKeyFromJWK(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, fmt.Errorf("empty modulus or exponent")
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
