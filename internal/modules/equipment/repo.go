package equipment

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"courtscore/internal/platform/entitlement"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type store interface {
	getCategory(ctx context.Context, id string) (Category, error)
	brandExists(ctx context.Context, id string) (bool, error)
	getModel(ctx context.Context, id string) (ModelRef, error)
	catalogCategories(ctx context.Context, sports []string) ([]catalogRow, error)

	insertCreate(ctx context.Context, userID string, row itemRow, isPro bool) (Item, error)
	listForUser(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Item, int64, error)
	getByID(ctx context.Context, id, userID string) (Item, error)
	update(ctx context.Context, id, userID string, row itemRow, clearNotified bool) (Item, error)
	delete(ctx context.Context, id, userID string) error
	activate(ctx context.Context, id, userID string) (Item, error)
	reset(ctx context.Context, id, userID string) (Item, error)
	archive(ctx context.Context, id, userID string) (Item, error)
	renew(ctx context.Context, id, userID string, row itemRow) (Item, error)
	accrueActive(ctx context.Context, userID string, seconds int) ([]Item, error)
	markNotified(ctx context.Context, id string) (bool, error)
	listDueReminders(ctx context.Context) ([]Item, error)
}

type repo struct{ pool *pgxpool.Pool }

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRE.MatchString(s) }

// itemSelect joins the catalog so every read yields the full Item.
// renewals_this_year is derived at read time from the item's own lineage (AC-15).
const itemSelect = `
	SELECT e.id, e.user_id, COALESCE(e.sport, c.sport, ''), e.type, e.name,
	       COALESCE(c.id::text, ''), COALESCE(c.code, ''), c.parent_category_id,
	       COALESCE(c.is_critical, FALSE), COALESCE(c.supports_renew, FALSE),
	       COALESCE(c.threshold_type, 'NONE'),
	       e.brand_id, b.name, b.accent_color_hex, e.model_id, m.name,
	       e.custom_brand_name, e.custom_model_name, e.nickname, e.attributes,
	       e.purchase_date, e.lifespan_hours, e.lifespan_days, e.accumulated_seconds,
	       e.is_active, e.retired_at, e.notified_at, e.parent_item_id, e.lineage_id,
	       (SELECT COUNT(*) FROM equipment r
	         WHERE r.lineage_id = e.lineage_id AND r.retired_at IS NOT NULL
	           AND EXTRACT(year FROM r.retired_at) = EXTRACT(year FROM NOW())),
	       e.created_at, e.updated_at
	  FROM equipment e
	  LEFT JOIN equipment_categories c ON c.id = e.category_id
	  LEFT JOIN equipment_brands b ON b.id = e.brand_id
	  LEFT JOIN equipment_models m ON m.id = e.model_id`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	var purchase *time.Time
	var parentCat *string
	err := row.Scan(
		&it.ID, &it.UserID, &it.Sport, &it.LegacyType, &it.LegacyName,
		&it.Category.ID, &it.Category.Code, &parentCat,
		&it.Category.IsCritical, &it.Category.SupportsRenew, &it.Category.ThresholdType,
		&it.BrandID, &it.BrandName, &it.BrandAccentHex, &it.ModelID, &it.ModelName,
		&it.CustomBrandName, &it.CustomModelName, &it.Nickname, &it.Attributes,
		&purchase, &it.LifespanHours, &it.LifespanDays, &it.AccumulatedSeconds,
		&it.IsActive, &it.RetiredAt, &it.NotifiedAt, &it.ParentItemID, &it.LineageID,
		&it.RenewalsThisYear, &it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return Item{}, err
	}
	it.Category.ParentCategoryID = parentCat
	it.Category.Sport = it.Sport
	if purchase != nil {
		s := purchase.Format("2006-01-02")
		it.PurchaseDate = &s
	}
	return it, nil
}

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func selectOne(ctx context.Context, q queryer, id string) (Item, error) {
	if !isUUID(id) {
		return Item{}, ErrNotFound
	}
	it, err := scanItem(q.QueryRow(ctx, itemSelect+` WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("get equipment: %w", err)
	}
	return it, nil
}

func selectMany(ctx context.Context, q queryer, where string, args ...any) ([]Item, error) {
	rows, err := q.Query(ctx, itemSelect+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query equipment: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan equipment: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *repo) getCategory(ctx context.Context, id string) (Category, error) {
	if !isUUID(id) {
		return Category{}, ErrNotFound
	}
	var c Category
	err := r.pool.QueryRow(ctx, `
		SELECT id, sport, code, parent_category_id, is_critical, supports_renew, threshold_type, is_active
		  FROM equipment_categories WHERE id = $1`, id).
		Scan(&c.ID, &c.Sport, &c.Code, &c.ParentCategoryID, &c.IsCritical, &c.SupportsRenew, &c.ThresholdType, &c.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, fmt.Errorf("get category: %w", err)
	}
	return c, nil
}

func (r *repo) brandExists(ctx context.Context, id string) (bool, error) {
	if !isUUID(id) {
		return false, nil
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM equipment_brands WHERE id = $1)`, id).Scan(&ok); err != nil {
		return false, fmt.Errorf("brand exists: %w", err)
	}
	return ok, nil
}

func (r *repo) getModel(ctx context.Context, id string) (ModelRef, error) {
	if !isUUID(id) {
		return ModelRef{}, ErrNotFound
	}
	var m ModelRef
	err := r.pool.QueryRow(ctx, `SELECT category_id, brand_id FROM equipment_models WHERE id = $1`, id).
		Scan(&m.CategoryID, &m.BrandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ModelRef{}, ErrNotFound
	}
	if err != nil {
		return ModelRef{}, fmt.Errorf("get model: %w", err)
	}
	return m, nil
}

func attrsOrEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

const insertSQL = `
	INSERT INTO equipment
	    (user_id, sport, category_id, brand_id, custom_brand_name, model_id, custom_model_name,
	     nickname, name, purchase_date, lifespan_hours, lifespan_days, attributes,
	     parent_item_id, lineage_id, is_active)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::date, $11, $12, $13, $14, COALESCE($15::uuid, gen_random_uuid()), $16)
	RETURNING id`

func insertRow(ctx context.Context, tx pgx.Tx, userID string, row itemRow, lineageID *string, active bool) (string, error) {
	var id string
	err := tx.QueryRow(ctx, insertSQL,
		userID, row.Sport, row.CategoryID, row.BrandID, row.CustomBrandName, row.ModelID, row.CustomModelName,
		row.Nickname, row.LegacyName, row.PurchaseDate, row.LifespanHours, row.LifespanDays, attrsOrEmpty(row.Attributes),
		row.ParentItemID, lineageID, active).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert equipment: %w", err)
	}
	return id, nil
}

func (r *repo) insertCreate(ctx context.Context, userID string, row itemRow, isPro bool) (Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("create begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text))`, userID); err != nil {
		return Item{}, fmt.Errorf("create lock: %w", err)
	}
	// Free limit counts live (non-retired) rows only: archived generations are free (§2.4).
	var count int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM equipment WHERE user_id = $1 AND retired_at IS NULL`, userID).Scan(&count); err != nil {
		return Item{}, fmt.Errorf("create count: %w", err)
	}
	if !isPro && count >= freeEquipmentLimit {
		return Item{}, entitlement.ErrProRequired
	}
	var hasActive bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM equipment
		               WHERE user_id = $1 AND category_id = $2 AND is_active AND retired_at IS NULL)`,
		userID, row.CategoryID,
	).Scan(&hasActive); err != nil {
		return Item{}, fmt.Errorf("create has active: %w", err)
	}
	id, err := insertRow(ctx, tx, userID, row, nil, !hasActive)
	if err != nil {
		return Item{}, err
	}
	it, err := selectOne(ctx, tx, id)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("create commit: %w", err)
	}
	return it, nil
}

func listWhere(userID string, f ListFilter) (string, []any) {
	where := ` WHERE e.user_id = $1`
	args := []any{userID}
	if f.View == ViewArchive {
		where += ` AND e.retired_at IS NOT NULL`
	} else {
		where += ` AND e.retired_at IS NULL`
	}
	if f.Sport != "" {
		args = append(args, f.Sport)
		where += fmt.Sprintf(` AND e.sport = $%d`, len(args))
	}
	if f.CategoryID != "" {
		if isUUID(f.CategoryID) {
			args = append(args, f.CategoryID)
			where += fmt.Sprintf(` AND e.category_id = $%d`, len(args))
		} else {
			where += ` AND FALSE`
		}
	}
	return where, args
}

func (r *repo) listForUser(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Item, int64, error) {
	where, args := listWhere(userID, f)
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM equipment e`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count equipment: %w", err)
	}
	order := ` ORDER BY e.is_active DESC, e.sport, c.display_order, e.created_at DESC, e.id`
	if f.View == ViewArchive {
		order = ` ORDER BY e.retired_at DESC, e.id`
	}
	args = append(args, limit, offset)
	items, err := selectMany(ctx, r.pool, where+order+fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *repo) getByID(ctx context.Context, id, userID string) (Item, error) {
	it, err := selectOne(ctx, r.pool, id)
	if err != nil {
		return Item{}, err
	}
	if it.UserID != userID {
		return Item{}, ErrNotFound
	}
	return it, nil
}

func (r *repo) update(ctx context.Context, id, userID string, row itemRow, clearNotified bool) (Item, error) {
	if !isUUID(id) {
		return Item{}, ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipment SET
			brand_id = $3, custom_brand_name = $4, model_id = $5, custom_model_name = $6,
			nickname = $7, lifespan_hours = $8, lifespan_days = $9, purchase_date = $10::date,
			attributes = $11, parent_item_id = $12,
			notified_at = CASE WHEN $13 THEN NULL ELSE notified_at END,
			updated_at = NOW()
		 WHERE id = $1 AND user_id = $2`,
		id, userID, row.BrandID, row.CustomBrandName, row.ModelID, row.CustomModelName,
		row.Nickname, row.LifespanHours, row.LifespanDays, row.PurchaseDate,
		attrsOrEmpty(row.Attributes), row.ParentItemID, clearNotified)
	if err != nil {
		return Item{}, fmt.Errorf("update equipment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Item{}, ErrNotFound
	}
	return r.getByID(ctx, id, userID)
}

func (r *repo) delete(ctx context.Context, id, userID string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM equipment WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete equipment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *repo) activate(ctx context.Context, id, userID string) (Item, error) {
	if !isUUID(id) {
		return Item{}, ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("activate begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var categoryID, legacyType *string
	var retiredAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT category_id, type, retired_at FROM equipment
		 WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID).Scan(&categoryID, &legacyType, &retiredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("activate get: %w", err)
	}
	if retiredAt != nil {
		return Item{}, fmt.Errorf("%w: archived item cannot be activated", ErrInvalid)
	}

	// Single active per category (uq_equipment_active_category); legacy rows without a
	// category (rollback leftovers) fall back to the old per-type scope.
	switch {
	case categoryID != nil:
		_, err = tx.Exec(ctx, `
			UPDATE equipment SET is_active = FALSE, updated_at = NOW()
			 WHERE user_id = $1 AND category_id = $2 AND is_active AND retired_at IS NULL AND id <> $3`,
			userID, *categoryID, id)
	case legacyType != nil:
		_, err = tx.Exec(ctx, `
			UPDATE equipment SET is_active = FALSE, updated_at = NOW()
			 WHERE user_id = $1 AND type = $2 AND is_active AND id <> $3`,
			userID, *legacyType, id)
	}
	if err != nil {
		return Item{}, fmt.Errorf("activate deactivate: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE equipment SET is_active = TRUE, notified_at = NULL, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2`, id, userID); err != nil {
		return Item{}, fmt.Errorf("activate set: %w", err)
	}
	it, err := selectOne(ctx, tx, id)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("activate commit: %w", err)
	}
	return it, nil
}

func (r *repo) reset(ctx context.Context, id, userID string) (Item, error) {
	if !isUUID(id) {
		return Item{}, ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipment
		   SET accumulated_seconds = 0, notified_at = NULL, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return Item{}, fmt.Errorf("reset equipment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Item{}, ErrNotFound
	}
	return r.getByID(ctx, id, userID)
}

func (r *repo) archive(ctx context.Context, id, userID string) (Item, error) {
	if !isUUID(id) {
		return Item{}, ErrNotFound
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE equipment SET retired_at = NOW(), is_active = FALSE, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND retired_at IS NULL`, id, userID)
	if err != nil {
		return Item{}, fmt.Errorf("archive equipment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Distinguish foreign/missing (404) from already archived (409).
		if _, err := r.getByID(ctx, id, userID); err != nil {
			return Item{}, err
		}
		return Item{}, fmt.Errorf("%w: item already archived", ErrConflict)
	}
	return r.getByID(ctx, id, userID)
}

// renew retires the old row and inserts the next generation in one transaction
// (AC-13/14). The old row is deactivated first so the new row can take the active slot.
func (r *repo) renew(ctx context.Context, id, userID string, row itemRow) (Item, error) {
	if !isUUID(id) {
		return Item{}, ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("renew begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lineageID string
	var wasActive bool
	var retiredAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT lineage_id, is_active, retired_at FROM equipment
		 WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID).Scan(&lineageID, &wasActive, &retiredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("renew get: %w", err)
	}
	if retiredAt != nil {
		return Item{}, fmt.Errorf("%w: item already archived", ErrConflict)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE equipment SET retired_at = NOW(), is_active = FALSE, updated_at = NOW()
		 WHERE id = $1`, id); err != nil {
		return Item{}, fmt.Errorf("renew retire: %w", err)
	}
	// The new generation inherits the slot: active iff the old one was active.
	newID, err := insertRow(ctx, tx, userID, row, &lineageID, wasActive)
	if err != nil {
		return Item{}, err
	}
	it, err := selectOne(ctx, tx, newID)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Item{}, fmt.Errorf("renew commit: %w", err)
	}
	return it, nil
}

func (r *repo) accrueActive(ctx context.Context, userID string, seconds int) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE equipment
		   SET accumulated_seconds = accumulated_seconds + $2, updated_at = NOW()
		 WHERE user_id = $1 AND is_active = TRUE AND retired_at IS NULL
		 RETURNING id`, userID, seconds)
	if err != nil {
		return nil, fmt.Errorf("accrue equipment: %w", err)
	}
	ids, err := collectIDs(rows)
	if err != nil {
		return nil, err
	}
	return r.itemsByIDs(ctx, ids)
}

func collectIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *repo) itemsByIDs(ctx context.Context, ids []string) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return selectMany(ctx, r.pool, ` WHERE e.id = ANY($1::uuid[])`, ids)
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
	return selectMany(ctx, r.pool, `
		 WHERE e.is_active AND e.retired_at IS NULL
		   AND e.notified_at IS NULL
		   AND c.threshold_type IN ('HOURS', 'HOURS_OR_DAYS')
		   AND e.lifespan_hours IS NOT NULL
		   AND e.accumulated_seconds >= e.lifespan_hours::bigint * 3600`)
}
