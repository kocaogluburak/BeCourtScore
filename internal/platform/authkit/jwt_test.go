package authkit_test

import (
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
)

var testSecret = []byte("test-secret-for-unit-tests-only")

func TestIssueAndParseAccessToken(t *testing.T) {
	userID := "user-123"
	token, err := authkit.IssueAccessToken(userID, testSecret, time.Minute)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	got, err := authkit.ParseAccessToken(token, testSecret)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if got != userID {
		t.Errorf("userID: got %q, want %q", got, userID)
	}
}

func TestParseAccessToken_WrongSecret(t *testing.T) {
	token, _ := authkit.IssueAccessToken("user-1", testSecret, time.Minute)
	_, err := authkit.ParseAccessToken(token, []byte("wrong-secret"))
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestParseAccessToken_Expired(t *testing.T) {
	token, _ := authkit.IssueAccessToken("user-1", testSecret, -time.Second)
	_, err := authkit.ParseAccessToken(token, testSecret)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	tok1, hash1, err := authkit.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if tok1 == "" || hash1 == "" {
		t.Fatal("expected non-empty token and hash")
	}

	tok2, hash2, _ := authkit.GenerateRefreshToken()
	if tok1 == tok2 || hash1 == hash2 {
		t.Fatal("expected unique tokens")
	}

	if got := authkit.HashRefreshToken(tok1); got != hash1 {
		t.Errorf("HashRefreshToken mismatch: got %q, want %q", got, hash1)
	}
}
