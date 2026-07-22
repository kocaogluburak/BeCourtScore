package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFS embed.FS

// Run applies pending SQL migrations in lexicographic order.
// Applied migrations are tracked in the schema_migrations table.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(sqlFS, "sql")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		var count int
		if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name=$1`, name).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		data, err := sqlFS.ReadFile("sql/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}

		if _, err = tx.Exec(ctx, string(data)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("exec migration %s: %w", name, err)
		}

		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}

		slog.Info("migration applied", "name", name)
	}

	return nil
}
