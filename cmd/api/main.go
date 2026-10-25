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
	"courtscore/internal/modules/achievements"
	"courtscore/internal/modules/billing"
	"courtscore/internal/modules/equipment"
	"courtscore/internal/modules/identity"
	"courtscore/internal/modules/invite"
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

	// Billing first so social/score/tournament can share the same gate.
	billingSvc := billing.NewService(pool, billing.NewVerifierFromEnv(), nil)

	// Social (friendships + seasons) + push + score (history + live matches)
	socialSvc := social.NewService(pool)
	socialSvc.SetHub(hub)
	socialSvc.SetGate(billingSvc)
	notifySvc := notify.NewService(pool, notify.NewFCMSenderFromEnv())
	scoreSvc := score.NewService(pool, socialSvc, hub, notifySvc)
	scoreSvc.SetGate(billingSvc)
	scoreSvc.SetSeasonNotifier(seasonMatchSink{social: socialSvc})
	scoreSvc.SetPublicWebOrigin(cfg.PublicWebOrigin)
	scoreSvc.SetUserLookup(socialLookup{social: socialSvc})

	inviteSvc := invite.NewService(pool)
	inviteSvc.SetPublicWebOrigin(cfg.PublicWebOrigin)

	tournamentSvc := tournament.NewService(pool, hub)
	tournamentSvc.SetGate(billingSvc)

	achSvc := achievements.NewService(pool, socialSvc, hub, notifySvc)
	scoreSvc.SetAchievements(achSvc)
	achSched := achievements.NewScheduler(achSvc)
	go achSched.Run(ctx)

	feedSvc := social.NewFeedService(pool, hub, notifySvc)
	scoreSvc.SetActivitySink(feedActivitySink{feed: feedSvc})

	equipmentSvc := equipment.NewService(pool, notifySvc)
	equipmentSvc.SetGate(billingSvc)
	scoreSvc.SetAccruer(equipmentSvc)
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := equipmentSvc.SweepReminders(ctx); err != nil {
					slog.Warn("equipment: reminder sweep", "err", err)
				}
			}
		}
	}()

	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := socialSvc.RunSeasonRollover(ctx, time.Now()); err != nil {
					slog.Error("season rollover", "err", err)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      httpapi.New(cfg, identitySvc, scoreSvc, socialSvc, tournamentSvc, notifySvc, billingSvc, inviteSvc, achSvc, equipmentSvc, feedSvc, hub),
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

// socialLookup adapts social.Service to score.UserLookup without score importing social.
type socialLookup struct{ social *social.Service }

func (l socialLookup) UserSummaryByID(ctx context.Context, _, targetID string) (score.UserSummary, error) {
	u, err := l.social.UserSummaryByID(ctx, targetID)
	if err != nil {
		return score.UserSummary{}, err
	}
	return score.UserSummary{
		ID: u.ID, Nickname: u.Nickname, Name: u.Name, Surname: u.Surname, ProfileIcon: u.ProfileIcon,
	}, nil
}

// feedActivitySink adapts social.FeedService to score.ActivitySink without
// the social package importing score.
type feedActivitySink struct{ feed *social.FeedService }

func (a feedActivitySink) OnMatchRecorded(ctx context.Context, m score.Match) {
	if a.feed == nil {
		return
	}
	a.feed.OnMatchRecorded(ctx, social.MatchRecord{
		ID:           m.ID,
		ActorID:      m.CreatedBy,
		Sport:        m.Sport,
		PlayerAName:  m.PlayerAName,
		PlayerBName:  m.PlayerBName,
		PlayerA2Name: m.PlayerA2Name,
		PlayerB2Name: m.PlayerB2Name,
		SetsA:        m.SetsA,
		SetsB:        m.SetsB,
		WinnerSide:   m.WinnerSide,
		Winner:       m.Winner,
		PlayedAt:     m.PlayedAt,
	})
}

func (a feedActivitySink) OnMatchDeleted(ctx context.Context, matchID string) {
	if a.feed == nil {
		return
	}
	a.feed.OnMatchDeleted(ctx, matchID)
}

// seasonMatchSink adapts social.Service to score.SeasonNotifier without
// the social package importing score.
type seasonMatchSink struct{ social *social.Service }

func (a seasonMatchSink) OnMatchFinished(ctx context.Context, m score.FinishedMatch) {
	if a.social == nil {
		return
	}
	a.social.OnMatchFinished(ctx, social.SeasonMatch{
		Sport:     m.Sport,
		PlayedAt:  m.PlayedAt,
		PlayerIDs: []string{m.PlayerAUserID, m.PlayerA2UserID, m.PlayerBUserID, m.PlayerB2UserID},
	})
}
