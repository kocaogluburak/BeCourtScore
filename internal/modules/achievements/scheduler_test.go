package achievements

import (
	"context"
	"testing"
	"time"
)

func TestSendWeeklyReminders_SelectsAndDedupes(t *testing.T) {
	store := newFakeStore()
	store.candidates = []ReminderCandidate{
		{UserID: "need", WeeklyTarget: 2, ReminderOptIn: true, Current: 0},
		{UserID: "done", WeeklyTarget: 2, ReminderOptIn: true, Current: 2},
		{UserID: "optout", WeeklyTarget: 2, ReminderOptIn: false, Current: 0},
	}
	push := &fakePush{}
	// Friday 18:00 UTC → remaining days = 3, inside the window.
	svc := newTestService(store, nil)
	svc.push = push
	svc.now = func() time.Time { return time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC) }

	if err := svc.SendWeeklyReminders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if push.n != 1 || push.user != "need" || push.typ != "goal.reminder" {
		t.Fatalf("push=%+v", push)
	}
	// second pass: dedupe via goal_reminders_sent
	if err := svc.SendWeeklyReminders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if push.n != 1 {
		t.Fatalf("dedupe failed n=%d", push.n)
	}
}

func TestSendWeeklyReminders_SkipsEarlyWeek(t *testing.T) {
	store := newFakeStore()
	store.candidates = []ReminderCandidate{
		{UserID: "need", WeeklyTarget: 2, ReminderOptIn: true, Current: 0},
	}
	push := &fakePush{}
	svc := newTestService(store, nil)
	svc.push = push
	// Monday — remaining days = 7
	svc.now = func() time.Time { return time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC) }
	if err := svc.SendWeeklyReminders(context.Background()); err != nil {
		t.Fatal(err)
	}
	if push.n != 0 {
		t.Fatalf("early-week push n=%d", push.n)
	}
}

func TestReconcileStreaks_ResetsStale(t *testing.T) {
	last := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.streaks = []streakRow{{
		UserID: "u1",
		Streak: Streak{CurrentWeeks: 4, LongestWeeks: 6, LastActiveWeek: &last},
	}}
	svc := newTestService(store, nil)
	svc.now = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) }
	if err := svc.ReconcileStreaks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.streak.CurrentWeeks != 0 || store.streak.LongestWeeks != 6 {
		t.Fatalf("streak=%+v", store.streak)
	}
}

func TestBackfill_UnlocksExisting(t *testing.T) {
	store := newFakeStore()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	matches := &fakeMatches{
		ids: []string{"u1"},
		matches: map[string][]CountedMatch{
			"u1": {match("m1", true, now)},
		},
	}
	svc := newTestService(store, matches)
	if err := svc.Backfill(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.badges["u1"]) == 0 {
		t.Fatal("expected backfill unlock")
	}

	sched := NewScheduler(svc)
	sched.SetNow(func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) }) // not reminder hour
	sched.RunBackfillOnce(context.Background())
	sched.RunBackfillOnce(context.Background()) // once
	sched.Tick(context.Background())
}

func TestScheduler_TickRemindersOnlyAtHour(t *testing.T) {
	store := newFakeStore()
	store.candidates = []ReminderCandidate{
		{UserID: "need", WeeklyTarget: 2, ReminderOptIn: true, Current: 0},
	}
	push := &fakePush{}
	svc := newTestService(store, nil)
	svc.push = push
	svc.now = func() time.Time { return time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC) }
	sched := NewScheduler(svc)
	sched.SetNow(func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) })
	sched.SetReminderHour(18)
	sched.Tick(context.Background())
	if push.n != 0 {
		t.Fatalf("reminded off-hour n=%d", push.n)
	}
	sched.SetNow(func() time.Time { return time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC) })
	sched.Tick(context.Background())
	if push.n != 1 {
		t.Fatalf("expected reminder at hour n=%d", push.n)
	}
}
