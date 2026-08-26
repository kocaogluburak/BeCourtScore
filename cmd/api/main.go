package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"courtscore/internal/httpapi"
	"courtscore/internal/migrate"
	"courtscore/internal/modules/identity"
	"courtscore/internal/modules/notify"
	"courtscore/internal/modules/score"
	"courtscore/internal/modules/social"
	"courtscore/internal/modules/tournament"
	"courtscore/internal/platform/config"
	"courtscore/internal/platform/db"
	"courtscore/internal/platform/sse"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Database
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Migrations
	if err = migrate.Run(ctx, pool); err != nil {
		slog.Error("migrations", "err", err)
		os.Exit(1)
	}

	// SSE hub
	hub := sse.NewHub()

	// Identity service
	identitySvc := identity.NewService(pool, identity.ServiceConfig{
		JWTSecret:       cfg.JWTSecret,
		AccessTokenTTL:  cfg.AccessTokenTTL,
		RefreshTokenTTL: cfg.RefreshTokenTTL,
	})
	identitySvc.RegisterProvider("google", identity.NewGoogleProvider(cfg.GoogleClientIDs))
	identitySvc.RegisterProvider("apple", identity.NewAppleProvider(cfg.AppleClientIDs))

	// Social (friendships) + push + score (history + live matches)
	socialSvc := social.NewService(pool)
	notifySvc := notify.NewService(pool, notify.NewFCMSenderFromEnv())
	scoreSvc := score.NewService(pool, socialSvc, hub, notifySvc)

	// Tournament (brackets, round-robin, live bracket events)
	tournamentSvc := tournament.NewService(pool, hub)

	// HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      httpapi.New(cfg, identitySvc, scoreSvc, socialSvc, tournamentSvc, notifySvc, hub),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE connections stream indefinitely
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err = srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}
