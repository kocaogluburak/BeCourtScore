package billing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid             = errors.New("invalid input")
	errVerifierUnavailable = errors.New("store verifier unavailable")
)

type repo struct{ pool *pgxpool.Pool }

func (r *repo) upsert(ctx context.Context, s Sub) error {
	const q = `
		INSERT INTO subscriptions (
			user_id, store, product_id, original_txn_id, status, expires_at, grace_until, purchase_token
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (store, original_txn_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			product_id = EXCLUDED.product_id,
			status = EXCLUDED.status,
			expires_at = EXCLUDED.expires_at,
			grace_until = EXCLUDED.grace_until,
			purchase_token = COALESCE(EXCLUDED.purchase_token, subscriptions.purchase_token),
			updated_at = NOW()`
	_, err := r.pool.Exec(ctx, q,
		s.UserID, s.Store, s.ProductID, s.OriginalTxnID, s.Status, s.ExpiresAt, s.GraceUntil, s.PurchaseToken,
	)
	return err
}

func (r *repo) activeForUser(ctx context.Context, userID string, now time.Time) (Sub, bool, error) {
	const q = `
		SELECT user_id, store, product_id, original_txn_id, status, expires_at, grace_until, purchase_token
		FROM subscriptions
		WHERE user_id = $1
		  AND status IN ('active', 'grace')
		  AND (expires_at > $2 OR (grace_until IS NOT NULL AND grace_until > $2))
		ORDER BY expires_at DESC
		LIMIT 1`
	s, err := scanSub(r.pool.QueryRow(ctx, q, userID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Sub{}, false, nil
	}
	if err != nil {
		return Sub{}, false, err
	}
	return s, true, nil
}

func (r *repo) byOriginalTxn(ctx context.Context, store, originalTxnID string) (Sub, bool, error) {
	const q = `
		SELECT user_id, store, product_id, original_txn_id, status, expires_at, grace_until, purchase_token
		FROM subscriptions
		WHERE store = $1 AND original_txn_id = $2`
	s, err := scanSub(r.pool.QueryRow(ctx, q, store, originalTxnID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Sub{}, false, nil
	}
	if err != nil {
		return Sub{}, false, err
	}
	return s, true, nil
}

func (r *repo) countFinishedMatches(ctx context.Context, userID string) (int, error) {
	const q = `
		SELECT COUNT(*)::int FROM matches
		WHERE created_by = $1
		   OR player_a_user_id = $1
		   OR player_b_user_id = $1`
	var n int
	err := r.pool.QueryRow(ctx, q, userID).Scan(&n)
	return n, err
}

func scanSub(row pgx.Row) (Sub, error) {
	var s Sub
	err := row.Scan(&s.UserID, &s.Store, &s.ProductID, &s.OriginalTxnID, &s.Status,
		&s.ExpiresAt, &s.GraceUntil, &s.PurchaseToken)
	return s, err
}
