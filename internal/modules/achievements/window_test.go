package achievements

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeAllPush struct {
	n    int
	user string
	typ  string
	data map[string]string
	args []string
}

func (f *fakeAllPush) SendLocalizedToAllPlatforms(_ context.Context, userID, typ, _, _ string, args []string, data map[string]string) error {
	f.n++
	f.user = userID
	f.typ = typ
	f.args = append([]string{}, args...)
	f.data = data
	return nil
}

func TestPulseWindow_CityEvenings(t *testing.T) {
	istanbul, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Fatal(err)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	week := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	at := func(loc *time.Location, day, hour int) time.Time {
		return time.Date(2026, 11, day, hour, 0, 0, 0, loc)
	}
	cases := []struct {
		name string
		now  time.Time
		loc  *time.Location
		ok   bool
	}{
		{"istanbul sunday", at(istanbul, 8, 18), istanbul, true},
		{"istanbul saturday", at(istanbul, 7, 18), istanbul, false},
		{"istanbul 17", at(istanbul, 8, 17), istanbul, false},
		{"istanbul 19", at(istanbul, 8, 19), istanbul, false},
		{"new york sunday", at(ny, 8, 18), ny, true},
		{"chicago sunday", at(chicago, 8, 18), chicago, false},
		{"chicago saturday", at(chicago, 7, 18), chicago, true},
		{"los angeles sunday", at(la, 8, 18), la, false},
		{"los angeles saturday", at(la, 7, 18), la, true},
		{"nil zone", at(istanbul, 8, 18), nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, ok := PulseWindow(tc.now, tc.loc)
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v start=%s", ok, tc.ok, start)
			}
			if ok && !start.Equal(week) {
				t.Fatalf("week=%s", start)
			}
		})
	}
}

func TestPulseCandidatesSQL_SkipsNullZone(t *testing.T) {
	if !strings.Contains(pulseCandidatesSQL, "time_zone IS NOT NULL") {
		t.Fatal(pulseCandidatesSQL)
	}
}

func TestSendWeeklyPulses_NewsOnceAndReminderStaysSeparate(t *testing.T) {
	istanbul, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		t.Fatal(err)
	}
	sundayLocal := time.Date(2026, 11, 8, 18, 0, 0, 0, istanbul)
	store := newFakeStore()
	store.goal = GoalConfig{WeeklyTarget: 2, ReminderOptIn: false}
	store.pulseCands = []PulseCandidate{{UserID: "u1", TimeZone: "Europe/Istanbul"}}
	matches := &fakeMatches{matches: map[string][]CountedMatch{
		"u1": {match("m1", true, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))},
	}}
	all := &fakeAllPush{}
	remind := &fakePush{}
	svc := newTestService(store, matches)
	svc.now = func() time.Time { return sundayLocal }
	svc.pushAll = all
	svc.push = remind

	if err := svc.SendWeeklyPulses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if all.n != 1 || all.typ != "weekly.pulse" || all.user != "u1" {
		t.Fatalf("push=%+v", all)
	}
	if len(all.args) != 0 {
		t.Fatalf("loc args=%v", all.args)
	}
	if all.data["title_loc_key"] != "pulse_push_title" || all.data["body_loc_key"] != "pulse_push_body" {
		t.Fatalf("data=%v", all.data)
	}
	if len(store.pulses) != 1 || len(store.reminders) != 0 {
		t.Fatalf("pulses=%v reminders=%v", store.pulses, store.reminders)
	}
	if err := svc.SendWeeklyPulses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if all.n != 1 {
		t.Fatalf("second send n=%d", all.n)
	}

	card, err := svc.GetPulse(context.Background(), "u1")
	if err != nil || card.Goal == nil || card.Goal.Current != 1 {
		t.Fatalf("get=%+v err=%v", card.Goal, err)
	}
	if len(store.pulses) != 1 {
		t.Fatal("GET wrote a send row")
	}
	if remind.n != 0 {
		t.Fatal("pulse path used the goal reminder sender")
	}
}

func TestSendWeeklyPulses_NoNewsWritesNoRow(t *testing.T) {
	istanbul, _ := time.LoadLocation("Europe/Istanbul")
	now := time.Date(2026, 11, 8, 18, 0, 0, 0, istanbul)
	store := newFakeStore()
	store.goal = GoalConfig{WeeklyTarget: 2, ReminderOptIn: true}
	store.pulseCands = []PulseCandidate{
		{UserID: "idle", TimeZone: "Europe/Istanbul"},
		{UserID: "blank", TimeZone: ""},
	}
	all := &fakeAllPush{}
	svc := newTestService(store, &fakeMatches{matches: map[string][]CountedMatch{}})
	svc.now = func() time.Time { return now }
	svc.pushAll = all
	if err := svc.SendWeeklyPulses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if all.n != 0 || len(store.pulses) != 0 {
		t.Fatalf("n=%d pulses=%v", all.n, store.pulses)
	}
}

func TestScheduler_PulseOncePerHourAndReminderSeparate(t *testing.T) {
	istanbul, _ := time.LoadLocation("Europe/Istanbul")
	sunday := time.Date(2026, 11, 8, 18, 0, 0, 0, istanbul)
	store := newFakeStore()
	store.goal = GoalConfig{WeeklyTarget: 1, ReminderOptIn: false}
	store.pulseCands = []PulseCandidate{{UserID: "u1", TimeZone: "Europe/Istanbul"}}
	matches := &fakeMatches{matches: map[string][]CountedMatch{
		"u1": {match("m1", true, time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))},
	}}
	all := &fakeAllPush{}
	remind := &fakePush{}
	svc := newTestService(store, matches)
	svc.now = func() time.Time { return sunday }
	svc.pushAll = all
	svc.push = remind
	sched := NewScheduler(svc)
	sched.SetNow(func() time.Time { return sunday })
	sched.Tick(context.Background())
	sched.Tick(context.Background())
	if all.n != 1 {
		t.Fatalf("pulse n=%d", all.n)
	}
	if remind.n != 0 {
		t.Fatalf("reminder n=%d", remind.n)
	}

	// UTC 18 on a late-week day still sends the goal reminder and skips a closed pulse window.
	fridayUTC := time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC)
	store.candidates = []ReminderCandidate{{UserID: "need", WeeklyTarget: 2, ReminderOptIn: true, Current: 0}}
	store.pulseCands = []PulseCandidate{{UserID: "u1", TimeZone: "UTC"}}
	svc.now = func() time.Time { return fridayUTC }
	sched.SetNow(func() time.Time { return fridayUTC })
	sched.Tick(context.Background())
	if remind.n != 1 || remind.typ != "goal.reminder" {
		t.Fatalf("reminder=%+v", remind)
	}
	if all.n != 1 {
		t.Fatalf("friday is not the last evening, pulse n=%d", all.n)
	}
}
