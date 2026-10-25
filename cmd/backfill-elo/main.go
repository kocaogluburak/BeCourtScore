// One-off Elo history backfill for Wrapped elo_peak.
//
// Replays finished rated singles in played_at order from 1500 and inserts
// missing user_rating_history rows. Safe and idempotent (unique user_id+match_id).
// Does not rewrite user_ratings.
//
//	cd BeCourtScore && go run ./cmd/backfill-elo
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"courtscore/internal/migrate"
	"courtscore/internal/modules/score"
	"courtscore/internal/platform/config"
	"courtscore/internal/platform/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err = migrate.Run(ctx, pool); err != nil {
		slog.Error("migrations", "err", err)
		os.Exit(1)
	}

	svc := score.NewService(pool, nil, nil, nil)
	n, err := svc.BackfillRatingHistory(ctx)
	if err != nil {
		slog.Error("elo backfill", "err", err)
		os.Exit(1)
	}
	slog.Info("elo backfill done", "inserted", n)
}
