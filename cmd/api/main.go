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
	tournamentSvc.SetDoublesRater(doublesNightSink{score: scoreSvc})

	achSvc := achievements.NewService(pool, socialSvc, hub, notifySvc)
	achSvc.SetPulseBoard(pulseBoard{social: socialSvc})
	achSvc.SetPulseAudience(pulseAudience{social: socialSvc})
	achSvc.SetPulsePush(notifySvc)
	scoreSvc.SetPulseHook(matchPulseSink{ach: achSvc})
	tournamentSvc.SetSocialPulse(nightPulseSink{ach: achSvc})
	scoreSvc.SetAchievements(achSvc)
	achSched := achievements.NewScheduler(achSvc)
	go achSched.Run(ctx)

	feedSvc := social.NewFeedService(pool, hub, notifySvc)
	feedSvc.SetPulseHook(feedPulseSink{ach: achSvc})
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

// pulseBoard adapts social.Service to achievements.PulseBoard.
// achievements and social do not import each other.
type pulseBoard struct{ social *social.Service }

func (p pulseBoard) WeeklyRank(ctx context.Context, userID string, start, end time.Time) (*achievements.PulseRank, error) {
	if p.social == nil {
		return nil, nil
	}
	rank, err := p.social.WeeklyRank(ctx, userID, start, end)
	if err != nil || rank == nil {
		return nil, err
	}
	return &achievements.PulseRank{
		GroupID: rank.GroupID, GroupName: rank.GroupName, Rank: rank.Rank, Points: rank.Points,
	}, nil
}

func (p pulseBoard) WeeklyFriends(ctx context.Context, userID string, start, end time.Time) (*achievements.PulseFriends, error) {
	if p.social == nil {
		return nil, nil
	}
	friends, err := p.social.WeeklyFriends(ctx, userID, start, end)
	if err != nil || friends == nil {
		return nil, err
	}
	return &achievements.PulseFriends{Names: friends.Names, Others: friends.Others}, nil
}

func (p pulseBoard) WeeklySocial(ctx context.Context, userID string, start, end time.Time) (*achievements.PulseSocial, error) {
	if p.social == nil {
		return nil, nil
	}
	social, err := p.social.WeeklySocialInteraction(ctx, userID, start, end)
	if err != nil || social == nil {
		return nil, err
	}
	return &achievements.PulseSocial{Received: social.Received, Given: social.Given}, nil
}

// pulseAudience adapts social audience queries to achievements.
type pulseAudience struct{ social *social.Service }

func (p pulseAudience) MatchAudience(ctx context.Context, participantIDs []string) ([]string, error) {
	if p.social == nil {
		return nil, nil
	}
	return p.social.MatchAudience(ctx, participantIDs)
}

func (p pulseAudience) NightAudience(ctx context.Context, playerIDs []string, groupID string) ([]string, error) {
	if p.social == nil {
		return nil, nil
	}
	return p.social.NightAudience(ctx, playerIDs, groupID)
}

type matchPulseSink struct{ ach *achievements.Service }

func (m matchPulseSink) OnMatchPulse(ctx context.Context, participantUserIDs []string) {
	if m.ach == nil {
		return
	}
	m.ach.NotifyMatchPulse(ctx, participantUserIDs)
}

type feedPulseSink struct{ ach *achievements.Service }

func (f feedPulseSink) NotifyFeedPulse(ctx context.Context, userIDs []string) {
	if f.ach == nil {
		return
	}
	f.ach.NotifyFeedPulse(ctx, userIDs)
}

type nightPulseSink struct{ ach *achievements.Service }

func (n nightPulseSink) OnSocialNight(ctx context.Context, playerIDs []string, groupID string) {
	if n.ach == nil {
		return
	}
	n.ach.NotifyNightPulse(ctx, playerIDs, groupID)
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

// doublesNightSink adapts score.Service to tournament.DoublesRater.
type doublesNightSink struct{ score *score.Service }

func (a doublesNightSink) RateDoublesFixtures(ctx context.Context, sport string, fixtures []tournament.DoublesFixture) error {
	if a.score == nil || len(fixtures) == 0 {
		return nil
	}
	out := make([]score.DoublesFixture, len(fixtures))
	for i, f := range fixtures {
		out[i] = score.DoublesFixture{
			TournamentMatchID: f.TournamentMatchID,
			PlayedAt:          f.PlayedAt,
			SideA:             f.SideA,
			SideB:             f.SideB,
			AWon:              f.AWon,
		}
	}
	return a.score.RateDoublesFixtures(ctx, sport, out)
}
