package billing

import (
	"context"
	"time"

	"courtscore/internal/platform/entitlement"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ProductMonthly = "pro_monthly"
	ProductYearly  = "pro_yearly"
)

// Sub is a persisted store subscription.
type Sub struct {
	UserID        string
	Store         string
	ProductID     string
	OriginalTxnID string
	Status        string
	ExpiresAt     time.Time
	GraceUntil    *time.Time
	PurchaseToken *string
}

type store interface {
	upsert(ctx context.Context, s Sub) error
	activeForUser(ctx context.Context, userID string, now time.Time) (Sub, bool, error)
	byOriginalTxn(ctx context.Context, store, originalTxnID string) (Sub, bool, error)
	countFinishedMatches(ctx context.Context, userID string) (int, error)
}

// VerifiedSub is the result of a store receipt check.
type VerifiedSub struct {
	Store         string
	ProductID     string
	OriginalTxnID string
	ExpiresAt     time.Time
	GraceUntil    *time.Time
	Status        string // active | grace | expired | revoked
	PurchaseToken string
}

// StoreVerifier checks a purchase with Apple or Google.
type StoreVerifier interface {
	VerifyGoogle(ctx context.Context, purchaseToken, productID string) (VerifiedSub, error)
	VerifyApple(ctx context.Context, signedTransaction string) (VerifiedSub, error)
}

// Service is the billing domain.
type Service struct {
	repo   store
	verify StoreVerifier
	hub    UserNotifier
	now    func() time.Time
}

// UserNotifier publishes user.updated after a subscription change.
type UserNotifier interface {
	NotifyUserUpdated(ctx context.Context, userID string)
}

// NewService wires billing to Postgres.
func NewService(pool *pgxpool.Pool, verify StoreVerifier, hub UserNotifier) *Service {
	return &Service{
		repo:   &repo{pool: pool},
		verify: verify,
		hub:    hub,
		now:    time.Now,
	}
}

func (s *Service) Load(ctx context.Context, userID string) (entitlement.Snapshot, error) {
	count, err := s.repo.countFinishedMatches(ctx, userID)
	if err != nil {
		return entitlement.Snapshot{}, err
	}
	snap := entitlement.Snapshot{
		FinishedMatchCount:        count,
		FreeMatchLimit:            entitlement.FreeMatchLimit,
		MaxTournamentParticipants: entitlement.FreeMaxParticipants,
	}
	sub, ok, err := s.repo.activeForUser(ctx, userID, s.now())
	if err != nil {
		return entitlement.Snapshot{}, err
	}
	if ok {
		snap.IsPro = true
		exp := sub.ExpiresAt
		snap.ExpiresAt = &exp
		snap.MaxTournamentParticipants = entitlement.ProMaxParticipants
	} else if entitlement.WithinFreeQuota(snap) {
		snap.MaxTournamentParticipants = entitlement.ProMaxParticipants
	}
	return snap, nil
}

func (s *Service) AllowCloudSync(ctx context.Context, userID string) error {
	return s.requireQuota(ctx, userID)
}

func (s *Service) AllowStats(ctx context.Context, userID string) error {
	return s.requireQuota(ctx, userID)
}

func (s *Service) AllowPublicLive(ctx context.Context, userID string) error {
	return s.requireQuota(ctx, userID)
}

func (s *Service) AllowLargeTournament(ctx context.Context, userID string, maxParticipants int) error {
	if maxParticipants <= entitlement.FreeMaxParticipants {
		return nil
	}
	return s.requireQuota(ctx, userID)
}

func (s *Service) MaxParticipants(ctx context.Context, userID string) (int, error) {
	snap, err := s.Load(ctx, userID)
	if err != nil {
		return entitlement.FreeMaxParticipants, err
	}
	return snap.MaxTournamentParticipants, nil
}

func (s *Service) requireQuota(ctx context.Context, userID string) error {
	snap, err := s.Load(ctx, userID)
	if err != nil {
		return err
	}
	if entitlement.WithinFreeQuota(snap) {
		return nil
	}
	return entitlement.ErrProRequired
}

func (s *Service) ApplyVerified(ctx context.Context, userID string, v VerifiedSub) error {
	token := v.PurchaseToken
	var tokenPtr *string
	if token != "" {
		tokenPtr = &token
	}
	status := v.Status
	if status == "" {
		status = "active"
	}
	err := s.repo.upsert(ctx, Sub{
		UserID:        userID,
		Store:         v.Store,
		ProductID:     v.ProductID,
		OriginalTxnID: v.OriginalTxnID,
		Status:        status,
		ExpiresAt:     v.ExpiresAt,
		GraceUntil:    v.GraceUntil,
		PurchaseToken: tokenPtr,
	})
	if err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.NotifyUserUpdated(ctx, userID)
	}
	return nil
}

func (s *Service) ApplyVerifiedByTxn(ctx context.Context, v VerifiedSub) error {
	existing, ok, err := s.repo.byOriginalTxn(ctx, v.Store, v.OriginalTxnID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // unknown purchase; ignore webhook until a client links it
	}
	return s.ApplyVerified(ctx, existing.UserID, v)
}

func (s *Service) VerifyGoogle(ctx context.Context, userID, purchaseToken, productID string) error {
	if s.verify == nil {
		return errVerifierUnavailable
	}
	v, err := s.verify.VerifyGoogle(ctx, purchaseToken, productID)
	if err != nil {
		return err
	}
	return s.ApplyVerified(ctx, userID, v)
}

func (s *Service) VerifyApple(ctx context.Context, userID, signedTransaction string) error {
	if s.verify == nil {
		return errVerifierUnavailable
	}
	v, err := s.verify.VerifyApple(ctx, signedTransaction)
	if err != nil {
		return err
	}
	return s.ApplyVerified(ctx, userID, v)
}

func (s *Service) HandleGoogleRTDN(ctx context.Context, dataB64 string) error {
	token, product, err := ParseGoogleRTDN(dataB64)
	if err != nil {
		return err
	}
	if token == "" || s.verify == nil {
		return nil
	}
	v, err := s.verify.VerifyGoogle(ctx, token, product)
	if err != nil {
		return err
	}
	return s.ApplyVerifiedByTxn(ctx, v)
}

func (s *Service) HandleAppleASN(ctx context.Context, signedPayload string) error {
	v, err := ParseAppleASN(signedPayload)
	if err != nil {
		return err
	}
	if v.OriginalTxnID == "" {
		return nil
	}
	return s.ApplyVerifiedByTxn(ctx, v)
}
