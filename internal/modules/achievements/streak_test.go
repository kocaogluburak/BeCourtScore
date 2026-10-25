package achievements

import (
	"testing"
	"time"
)

func TestISOWeek_MondayUTC(t *testing.T) {
	// Sunday 26 Oct 2026 23:00 UTC is still ISO week starting Mon 26? 
	// 25 Oct 2026 is Sunday → week of Mon 19 Oct.
	sun := time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC)
	start, end := ISOWeek(sun)
	wantStart := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start=%s want %s", start, wantStart)
	}
	if !end.Equal(wantStart.AddDate(0, 0, 7)) {
		t.Fatalf("end=%s", end)
	}
	mon := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	ms, _ := ISOWeek(mon)
	if !ms.Equal(mon) {
		t.Fatalf("monday start=%s", ms)
	}
}

func TestApplyStreak_ConsecutiveIncrement(t *testing.T) {
	w1 := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)  // Mon week
	w2 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) // next Mon
	now := w2
	s := ApplyStreak(Streak{}, w1, now)
	if s.CurrentWeeks != 1 || s.LongestWeeks != 1 {
		t.Fatalf("first: %+v", s)
	}
	s = ApplyStreak(s, w2, now)
	if s.CurrentWeeks != 2 || s.LongestWeeks != 2 {
		t.Fatalf("second: %+v", s)
	}
	// same week is a no-op
	s = ApplyStreak(s, w2.Add(2*time.Hour), now)
	if s.CurrentWeeks != 2 {
		t.Fatalf("same week: %+v", s)
	}
}

func TestApplyStreak_GapResetsCurrentKeepsLongest(t *testing.T) {
	w1 := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	w2 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	gap := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) // skipped a week
	s := ApplyStreak(Streak{}, w1, gap)
	s = ApplyStreak(s, w2, gap)
	s = ApplyStreak(s, gap, gap)
	if s.CurrentWeeks != 1 {
		t.Fatalf("current after gap=%d", s.CurrentWeeks)
	}
	if s.LongestWeeks != 2 {
		t.Fatalf("longest=%d", s.LongestWeeks)
	}
}

func TestReconcileStreak_ResetsAfterMissedWeek(t *testing.T) {
	last := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) // week of Oct 5
	s := Streak{CurrentWeeks: 3, LongestWeeks: 5, LastActiveWeek: &last}
	// Still in the following week (Oct 12–18) — streak lives.
	alive := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	got := ReconcileStreak(s, alive)
	if got.CurrentWeeks != 3 {
		t.Fatalf("still alive: %+v", got)
	}
	// Week of Oct 19 has fully started with no play — reset.
	dead := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	got = ReconcileStreak(s, dead)
	if got.CurrentWeeks != 0 {
		t.Fatalf("should reset: %+v", got)
	}
	if got.LongestWeeks != 5 {
		t.Fatalf("longest mutated: %+v", got)
	}
}

func TestRemainingWeekDays(t *testing.T) {
	// Friday 23 Oct 2026 — 3 days left (Fri, Sat, Sun) until Mon 26.
	fri := time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC)
	if n := RemainingWeekDays(fri); n != 3 {
		t.Fatalf("friday remaining=%d", n)
	}
	sun := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	if n := RemainingWeekDays(sun); n != 1 {
		t.Fatalf("sunday remaining=%d", n)
	}
}
