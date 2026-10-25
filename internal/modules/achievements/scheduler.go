package achievements

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultTick         = time.Hour
	defaultReminderHour = 18 // 18:00 UTC
)

// Scheduler runs reminder + streak reconcile hourly and backfill once at boot.
type Scheduler struct {
	svc          *Service
	tick         time.Duration
	reminderHour int
	now          func() time.Time
	backfillOnce sync.Once
}

func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{
		svc:          svc,
		tick:         defaultTick,
		reminderHour: defaultReminderHour,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Scheduler) SetTick(d time.Duration) { s.tick = d }
func (s *Scheduler) SetReminderHour(h int)   { s.reminderHour = h }
func (s *Scheduler) SetNow(fn func() time.Time) {
	s.now = fn
}

// Run blocks until ctx is cancelled. Backfill runs once immediately.
func (s *Scheduler) Run(ctx context.Context) {
	s.runBackfill(ctx)
	if err := s.svc.ReconcileStreaks(ctx); err != nil {
		slog.Warn("achievements: initial streak reconcile", "err", err)
	}
	s.maybeRemind(ctx)

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

// Tick is the hourly pass (exported for tests).
func (s *Scheduler) Tick(ctx context.Context) {
	if err := s.svc.ReconcileStreaks(ctx); err != nil {
		slog.Warn("achievements: streak reconcile", "err", err)
	}
	s.maybeRemind(ctx)
}

func (s *Scheduler) maybeRemind(ctx context.Context) {
	if s.now().UTC().Hour() != s.reminderHour {
		return
	}
	if err := s.svc.SendWeeklyReminders(ctx); err != nil {
		slog.Warn("achievements: weekly reminders", "err", err)
	}
}

func (s *Scheduler) runBackfill(ctx context.Context) {
	s.backfillOnce.Do(func() {
		if err := s.svc.Backfill(ctx); err != nil {
			slog.Warn("achievements: backfill", "err", err)
		}
	})
}

// RunBackfillOnce exposes the boot sweep for tests.
func (s *Scheduler) RunBackfillOnce(ctx context.Context) {
	s.runBackfill(ctx)
}
