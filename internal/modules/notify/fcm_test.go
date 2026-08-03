package notify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFCMSenderFromEnv_UnsetUsesNoop(t *testing.T) {
	t.Setenv("FIREBASE_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	s := NewFCMSenderFromEnv()
	if _, ok := s.(NoopSender); !ok {
		t.Fatalf("want NoopSender, got %T", s)
	}
}

func TestNewFCMSenderFromEnv_MissingFileUsesNoop(t *testing.T) {
	t.Setenv("FIREBASE_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))
	s := NewFCMSenderFromEnv()
	if _, ok := s.(NoopSender); !ok {
		t.Fatalf("want NoopSender, got %T", s)
	}
}

func TestNewFCMSenderFromEnv_InvalidJSONUsesNoop(t *testing.T) {
	t.Setenv("FIREBASE_CREDENTIALS_JSON", `{"type":"service_account"}`)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	s := NewFCMSenderFromEnv()
	if _, ok := s.(NoopSender); !ok {
		t.Fatalf("want NoopSender, got %T", s)
	}
}

func TestNewFCMSenderFromEnv_UnreadableHostPathUsesNoop(t *testing.T) {
	// Mirrors Docker when .env points at a Mac-only path that is not mounted.
	t.Setenv("FIREBASE_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/Users/someone/Downloads/firebase-adminsdk.json")
	s := NewFCMSenderFromEnv()
	if _, ok := s.(NoopSender); !ok {
		t.Fatalf("want NoopSender, got %T", s)
	}
}

func TestNewFCMSenderFromEnv_ReadsCredentialsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.json")
	// Readable file with missing project_id → init fails → Noop (proves file was opened).
	if err := os.WriteFile(path, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIREBASE_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
	s := NewFCMSenderFromEnv()
	if _, ok := s.(NoopSender); !ok {
		t.Fatalf("want NoopSender after invalid creds file, got %T", s)
	}
}
