package equipment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"courtscore/internal/platform/entitlement"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store interface {
	insert(ctx context.Context, userID string, in CreateInput, isActive bool) (Item, error)
	insertCreate(ctx context.Context, userID string, in CreateInput, isPro bool) (Item, error)
	listForUser(ctx context.Context, userID string, limit, offset int) ([]Item, int64, error)
	getByID(ctx context.Context, id, userID string) (Item, error)
	update(ctx context.Context, id, userID string, in UpdateInput, clearNotified bool) (Item, error)
	delete(ctx context.Context, id, userID string) error
	hasActive(ctx context.Context, userID, typ string) (bool, error)
	activate(ctx context.Context, id, userID string) (Item, error)
	reset(ctx context.Context, id, userID string) (Item, error)
	accrueActive(ctx context.Context, userID string, seconds int) ([]Item, error)
	markNotified(ctx context.Context, id string) (bool, error)
	listDueReminders(ctx context.Context) ([]Item, error)
}

type repo struct{ pool *pgxpool.Pool }

const itemCols = `id, user_id, type, name, purchase_date, lifespan_hours,
	accumulated_seconds, is_active, notified_at, created_at, updated_at`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	var purchase *time.Time
	err := row.Scan(
		&it.ID, &it.UserID, &it.Type, &it.Name, &purchase, &it.LifespanHours,
		&it.AccumulatedSeconds, &it.IsActive, &it.NotifiedAt, &it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return Item{}, err
	}
	if purchase != nil {
		s := purchase.Format("2006-01-02")
		it.PurchaseDate = &s
	}
	return it, nil
}

func (r *repo) insertCreate(ctx context.Context, userID string, in CreateInput, isPro bool) (Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("create begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text))`, userID); err != nil {
		return Item{}, fmt.Errorf("create lock: %w", err)
	}
	var count int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM equipment WHERE user_id = $1`, userID).Scan(&count); err != nil {
		return Item{}, fmt.Errorf("create count: %w", err)
	}
	if !isPro && count >= freeEquipmentLimit {
		return Item{}, entitlement.ErrProRequired
	}
	var hasActive bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM equipment WHERE user_id = $1 AND type = $2 AND is_active)`,
		userID, in.Type,
	).Scan(&hasActive); err != nil {
		return Item{}, fmt.Errorf("create has active: %w", err)
	}
	it, err := scanItem(tx.QueryRow(ctx, `
		INSERT INTO equipment (user_id, type, name, purchase_date, lifespan_hours, is_active)
		VALUES ($1, $2, $3, $4::date, $5, $6)
		RETURNING `+itemCols,
		userID, in.Type, in.Name, in.PurchaseDate, in.LifespanHours, !hasActive,
	))
	if err != nil {
		return Item{}, fmt.Errorf("insert equipment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("create commit: %w", err)
	}
	return it, nil
}

func (r *repo) insert(ctx context.Context, userID string, in CreateInput, isActive bool) (Item, error) {
	it, err := scanItem(r.pool.QueryRow(ctx, `
		INSERT INTO equipment (user_id, type, name, purchase_date, lifespan_hours, is_active)
		VALUES ($1, $2, $3, $4::date, $5, $6)
		RETURNING `+itemCols,
		userID, in.Type, in.Name, in.PurchaseDate, in.LifespanHours, isActive,
	))
	if err != nil {
		return Item{}, fmt.Errorf("insert equipment: %w", err)
	}
	return it, nil
}

func (r *repo) listForUser(ctx context.Context, userID string, limit, offset int) ([]Item, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM equipment WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count equipment: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+itemCols+`
		  FROM equipment
		 WHERE user_id = $1
		 ORDER BY is_active DESC, type, created_at DESC
		 LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list equipment: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan equipment: %w", err)
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

func (r *repo) getByID(ctx context.Context, id, userID string) (Item, error) {
	it, err := scanItem(r.pool.QueryRow(ctx, `
		SELECT `+itemCols+` FROM equipment WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get equipment: %w", err)
	}
	return it, nil
}

func (r *repo) update(ctx context.Context, id, userID string, in UpdateInput, clearNotified bool) (Item, error) {
	it, err := scanItem(r.pool.QueryRow(ctx, `
		UPDATE equipment SET
			name = COALESCE($3, name),
			lifespan_hours = COALESCE($4, lifespan_hours),
			purchase_date = CASE
				WHEN $5 THEN $6::date
				ELSE purchase_date
			END,
			notified_at = CASE WHEN $7 THEN NULL ELSE notified_at END,
			updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+itemCols,
		id, userID, in.Name, in.LifespanHours, in.SetPurchaseDate, in.PurchaseDate, clearNotified,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("update equipment: %w", err)
	}
	return it, nil
}

func (r *repo) delete(ctx context.Context, id, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM equipment WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete equipment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) hasActive(ctx context.Context, userID, typ string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM equipment WHERE user_id = $1 AND type = $2 AND is_active`,
		userID, typ).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("has active equipment: %w", err)
	}
	return n > 0, nil
}

func (r *repo) activate(ctx context.Context, id, userID string) (Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("activate begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	it, err := scanItem(tx.QueryRow(ctx, `
		SELECT `+itemCols+` FROM equipment WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("activate get: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE equipment SET is_active = FALSE, updated_at = NOW()
		 WHERE user_id = $1 AND type = $2 AND is_active AND id <> $3`,
		userID, it.Type, id); err != nil {
		return Item{}, fmt.Errorf("activate deactivate: %w", err)
	}

	it, err = scanItem(tx.QueryRow(ctx, `
		UPDATE equipment
		   SET is_active = TRUE, notified_at = NULL, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+itemCols, id, userID))
	if err != nil {
		return Item{}, fmt.Errorf("activate set: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("activate commit: %w", err)
	}
	return it, nil
}

func (r *repo) reset(ctx context.Context, id, userID string) (Item, error) {
	it, err := scanItem(r.pool.QueryRow(ctx, `
		UPDATE equipment
		   SET accumulated_seconds = 0, notified_at = NULL, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+itemCols, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("reset equipment: %w", err)
	}
	return it, nil
}

func (r *repo) accrueActive(ctx context.Context, userID string, seconds int) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE equipment
		   SET accumulated_seconds = accumulated_seconds + $2, updated_at = NOW()
		 WHERE user_id = $1 AND is_active = TRUE
		 RETURNING `+itemCols, userID, seconds)
	if err != nil {
		return nil, fmt.Errorf("accrue equipment: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan accrue: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *repo) markNotified(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipment SET notified_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND notified_at IS NULL`, id)
	if err != nil {
		return false, fmt.Errorf("mark notified: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *repo) listDueReminders(ctx context.Context) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+itemCols+`
		  FROM equipment
		 WHERE is_active
		   AND notified_at IS NULL
		   AND accumulated_seconds >= lifespan_hours::bigint * 3600`)
	if err != nil {
		return nil, fmt.Errorf("list due equipment: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan due: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
