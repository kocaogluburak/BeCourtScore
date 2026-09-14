package entitlement

import (
	"context"
	"errors"
	"time"
)

const (
	FreeMatchLimit         = 10
	FreeMaxParticipants    = 8
	ProMaxParticipants     = 128
	DefaultMaxParticipants = 32
	CodeProRequired        = "pro_required"
)

// ErrProRequired is returned when a Pro-gated action is denied.
var ErrProRequired = errors.New("pro required")

// Snapshot is the entitlement payload nested on user JSON.
type Snapshot struct {
	IsPro                     bool       `json:"is_pro"`
	ExpiresAt                 *time.Time `json:"expires_at,omitempty"`
	FinishedMatchCount        int        `json:"finished_match_count"`
	FreeMatchLimit            int        `json:"free_match_limit"`
	MaxTournamentParticipants int        `json:"max_tournament_participants"`
}

// Loader loads a user's entitlement snapshot.
type Loader interface {
	Load(ctx context.Context, userID string) (Snapshot, error)
}

// Gate enforces Pro limits. A nil Gate (tests) means allow everything.
type Gate interface {
	AllowCloudSync(ctx context.Context, userID string) error
	AllowStats(ctx context.Context, userID string) error
	AllowPublicLive(ctx context.Context, userID string) error
	AllowLargeTournament(ctx context.Context, userID string, maxParticipants int) error
	MaxParticipants(ctx context.Context, userID string) (int, error)
}

// WithinFreeQuota is true when the user is Pro or still under the free match cap.
func WithinFreeQuota(s Snapshot) bool {
	return s.IsPro || s.FinishedMatchCount < FreeMatchLimit
}
