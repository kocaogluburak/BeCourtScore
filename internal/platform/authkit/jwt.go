package authkit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTypeAccess = "access"

type Claims struct {
	jwt.RegisteredClaims
	Type string `json:"type"`
}

// IssueAccessToken creates a signed JWT for the given userID.
func IssueAccessToken(userID string, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Type: tokenTypeAccess,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ParseAccessToken validates a JWT and returns the userID claim.
func ParseAccessToken(tokenStr string, secret []byte) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.Type != tokenTypeAccess {
		return "", fmt.Errorf("invalid token claims")
	}
	return claims.Subject, nil
}

// GenerateRefreshToken returns a random opaque token and its SHA-256 hex hash.
func GenerateRefreshToken() (token string, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("rand.Read: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	return token, hash, nil
}

// HashRefreshToken returns the SHA-256 hex hash of a refresh token.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
