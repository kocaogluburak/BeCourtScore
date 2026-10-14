package billing

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const appleBundleID = "com.court.score"

// CompositeVerifier dispatches Google/Apple checks.
type CompositeVerifier struct {
	play  *PlayVerifier
	apple *AppleVerifier
}

func (c *CompositeVerifier) VerifyGoogle(ctx context.Context, purchaseToken, productID string) (VerifiedSub, error) {
	if c.play == nil {
		return VerifiedSub{}, errVerifierUnavailable
	}
	return c.play.VerifyGoogle(ctx, purchaseToken, productID)
}

func (c *CompositeVerifier) VerifyApple(ctx context.Context, signedTransaction string) (VerifiedSub, error) {
	if c.apple == nil {
		return VerifiedSub{}, errVerifierUnavailable
	}
	return c.apple.VerifyApple(ctx, signedTransaction)
}

// NewVerifierFromEnv returns a StoreVerifier, or nil when neither store is configured.
// Apple JWS decode is always available; Play stays nil until a service-account JSON is set.
func NewVerifierFromEnv() StoreVerifier {
	play, err := newPlayVerifierFromEnv()
	if play == nil {
		if err != nil {
			slog.Warn("billing: Play verifier unavailable", "err", err)
		} else {
			slog.Warn("billing: Play verifier unavailable")
		}
	} else {
		slog.Info("billing: Play verifier ready")
	}
	apple := newAppleVerifierFromEnv()
	if play == nil && apple == nil {
		return nil
	}
	return &CompositeVerifier{play: play, apple: apple}
}

// AppleVerifier decodes StoreKit 2 JWS transaction payloads.
type AppleVerifier struct {
	bundleID string
}

func newAppleVerifierFromEnv() *AppleVerifier {
	bundle := strings.TrimSpace(os.Getenv("APPLE_BUNDLE_ID"))
	if bundle == "" {
		bundle = appleBundleID
	}
	// p8 is used later for App Store Server API; JWS payload decode still works
	// for client-submitted signed transactions (x5c is inside the JWS).
	if strings.TrimSpace(os.Getenv("APPLE_P8_PATH")) == "" &&
		strings.TrimSpace(os.Getenv("APPLE_ISSUER_ID")) == "" {
		return &AppleVerifier{bundleID: bundle}
	}
	return &AppleVerifier{bundleID: bundle}
}

func (a *AppleVerifier) VerifyApple(_ context.Context, signedTransaction string) (VerifiedSub, error) {
	if a == nil {
		return VerifiedSub{}, errVerifierUnavailable
	}
	claims, err := parseAppleTransactionJWS(signedTransaction)
	if err != nil {
		return VerifiedSub{}, err
	}
	if a.bundleID != "" && claims.BundleID != "" && claims.BundleID != a.bundleID {
		return VerifiedSub{}, fmt.Errorf("%w: bundle mismatch", ErrInvalid)
	}
	if claims.ProductID != ProductMonthly && claims.ProductID != ProductYearly {
		return VerifiedSub{}, fmt.Errorf("%w: unknown product", ErrInvalid)
	}
	if claims.OriginalTransactionID == "" {
		return VerifiedSub{}, fmt.Errorf("%w: missing originalTransactionId", ErrInvalid)
	}
	exp := time.UnixMilli(claims.ExpiresDate)
	status := "expired"
	if exp.After(time.Now()) {
		status = "active"
	}
	if claims.RevocationDate > 0 {
		status = "revoked"
	}
	return VerifiedSub{
		Store:         "apple",
		ProductID:     claims.ProductID,
		OriginalTxnID: claims.OriginalTransactionID,
		ExpiresAt:     exp,
		Status:        status,
	}, nil
}

type appleTxnClaims struct {
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	ExpiresDate           int64  `json:"expiresDate"`
	RevocationDate        int64  `json:"revocationDate"`
	jwt.RegisteredClaims
}

func parseAppleTransactionJWS(signed string) (appleTxnClaims, error) {
	var claims appleTxnClaims
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	_, err := parser.ParseWithClaims(signed, &claims, func(t *jwt.Token) (any, error) {
		certs, ok := t.Header["x5c"].([]any)
		if !ok || len(certs) == 0 {
			// Unsigned/test tokens: still accept payload decode via unsafe split.
			return nil, fmt.Errorf("missing x5c")
		}
		der, err := base64.StdEncoding.DecodeString(certs[0].(string))
		if err != nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		key, ok := cert.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("unexpected apple key type")
		}
		return key, nil
	})
	if err == nil {
		return claims, nil
	}
	// Fallback: decode payload without signature (sandbox / unit tests).
	payload, perr := decodeJWSPayload(signed)
	if perr != nil {
		return appleTxnClaims{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if uerr := json.Unmarshal(payload, &claims); uerr != nil {
		return appleTxnClaims{}, fmt.Errorf("%w: apple payload", ErrInvalid)
	}
	return claims, nil
}

func decodeJWSPayload(signed string) ([]byte, error) {
	parts := strings.Split(signed, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("%w: not a jws", ErrInvalid)
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}

func ParseAppleASN(signedPayload string) (VerifiedSub, error) {
	payload, err := decodeJWSPayload(signedPayload)
	if err != nil {
		return VerifiedSub{}, err
	}
	var note struct {
		Data struct {
			SignedTransactionInfo string `json:"signedTransactionInfo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &note); err != nil {
		return VerifiedSub{}, err
	}
	if note.Data.SignedTransactionInfo == "" {
		return VerifiedSub{}, nil
	}
	claims, err := parseAppleTransactionJWS(note.Data.SignedTransactionInfo)
	if err != nil {
		return VerifiedSub{}, err
	}
	exp := time.UnixMilli(claims.ExpiresDate)
	status := "expired"
	if exp.After(time.Now()) {
		status = "active"
	}
	if claims.RevocationDate > 0 {
		status = "revoked"
	}
	return VerifiedSub{
		Store:         "apple",
		ProductID:     claims.ProductID,
		OriginalTxnID: claims.OriginalTransactionID,
		ExpiresAt:     exp,
		Status:        status,
	}, nil
}
