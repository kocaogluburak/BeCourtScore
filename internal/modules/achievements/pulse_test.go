package achievements

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"courtscore/internal/platform/sse"
)

type fakeBoard struct {
	rank    *PulseRank
	friends *PulseFriends
}

func (f *fakeBoard) WeeklyRank(context.Context, string, time.Time, time.Time) (*PulseRank, error) {
	return f.rank, nil
}
func (f *fakeBoard) WeeklyFriends(context.Context, string, time.Time, time.Time) (*PulseFriends, error) {
	return f.friends, nil
}

func TestPulseDaysLeft_MondaySixSundayZero(t *testing.T) {
	monday := time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 11, 8, 23, 0, 0, 0, time.UTC)
	if PulseDaysLeft(monday) != 6 || RemainingWeekDays(monday) != 7 {
		t.Fatalf("monday pulse=%d remaining=%d", PulseDaysLeft(monday), RemainingWeekDays(monday))
	}
	if PulseDaysLeft(sunday) != 0 || RemainingWeekDays(sunday) != 1 {
		t.Fatalf("sunday pulse=%d remaining=%d", PulseDaysLeft(sunday), RemainingWeekDays(sunday))
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	// Sunday 18:00 Pacific is Monday 02:00 UTC, the new ISO week.
	pacificSunday := time.Date(2026, 11, 8, 18, 0, 0, 0, loc)
	if pacificSunday.UTC().Weekday() != time.Monday {
		t.Fatalf("utc weekday=%s", pacificSunday.UTC().Weekday())
	}
	if PulseDaysLeft(pacificSunday) != 6 {
		t.Fatalf("pacific sunday evening days_left=%d", PulseDaysLeft(pacificSunday))
	}
}

func TestGetPulse_GoalMatchesGoalAndStaysAtZero(t *testing.T) {
	monday := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	store := newFakeStore()
	store.goal = GoalConfig{WeeklyTarget: 3, ReminderOptIn: true}
	matches := &fakeMatches{matches: map[string][]CountedMatch{
		"u1": {match("m1", true, monday)},
	}}
	svc := newTestService(store, matches)
	svc.now = func() time.Time { return monday }

	goal, err := svc.GetGoal(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	card, err := svc.GetPulse(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if card.Goal == nil || card.Goal.Current != goal.Week.Current || card.Goal.Current != 1 || card.Goal.Target != 3 || card.Goal.Reached {
		t.Fatalf("pulse=%+v goal=%+v", card.Goal, goal.Week)
	}
	if card.DaysLeft != 6 {
		t.Fatalf("days=%d", card.DaysLeft)
	}

	store.goal = GoalConfig{WeeklyTarget: 0, ReminderOptIn: true}
	card, err = svc.GetPulse(context.Background(), "u1")
	if err != nil || card.Goal != nil {
		t.Fatalf("target 0 goal=%+v err=%v", card.Goal, err)
	}

	store.goal = GoalConfig{WeeklyTarget: 1, ReminderOptIn: false}
	svc.matches = &fakeMatches{matches: map[string][]CountedMatch{}}
	card, err = svc.GetPulse(context.Background(), "u1")
	if err != nil || card.Goal == nil || card.Goal.Current != 0 || card.Goal.Reached || card.Goal.Target != 1 {
		t.Fatalf("zero progress=%+v err=%v", card.Goal, err)
	}
}

type fakeAudience struct {
	match []string
	night []string
	err   error
}

func (f *fakeAudience) MatchAudience(context.Context, []string) ([]string, error) {
	return f.match, f.err
}
func (f *fakeAudience) NightAudience(context.Context, []string, string) ([]string, error) {
	return f.night, f.err
}

func TestNotifyPulse_AudienceGetsWeekStartOnly(t *testing.T) {
	monday := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	svc := newTestService(nil, nil)
	svc.now = func() time.Time { return monday }
	svc.SetPulseAudience(&fakeAudience{
		match: []string{"player", "friend", "member"},
		night: []string{"player", "member"},
	})
	player, unsubP := svc.hub.Subscribe("player")
	friend, unsubF := svc.hub.Subscribe("friend")
	member, unsubM := svc.hub.Subscribe("member")
	optout, unsubO := svc.hub.Subscribe("optout")
	defer unsubP()
	defer unsubF()
	defer unsubM()
	defer unsubO()

	svc.NotifyMatchPulse(context.Background(), []string{"player"})
	assertPulseEvent(t, player)
	assertPulseEvent(t, friend)
	assertPulseEvent(t, member)
	assertNoPulse(t, optout)

	svc.NotifyNightPulse(context.Background(), []string{"player"}, "g1")
	assertPulseEvent(t, player)
	assertPulseEvent(t, member)
	assertNoPulse(t, friend)
}

func assertPulseEvent(t *testing.T, ch <-chan sse.Event) {
	t.Helper()
	select {
	case ev := <-ch:
		if ev.Type != "pulse.updated" {
			t.Fatalf("type=%s", ev.Type)
		}
		raw, err := json.Marshal(ev.Data)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || body["week_start"] != "2026-11-02T00:00:00Z" {
			t.Fatalf("data=%s", raw)
		}
		if strings.Contains(string(raw), "title") {
			t.Fatalf("event carries text: %s", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("missing pulse.updated")
	}
}

func assertNoPulse(t *testing.T, ch <-chan sse.Event) {
	t.Helper()
	select {
	case ev := <-ch:
		t.Fatalf("unexpected %s", ev.Type)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestGetPulse_PassesRankAndFriends(t *testing.T) {
	svc := newTestService(nil, nil)
	svc.SetPulseBoard(&fakeBoard{
		rank:    &PulseRank{GroupID: "g1", GroupName: "Salı kortu", Rank: 2, Points: 4},
		friends: &PulseFriends{Names: []string{"Ada Lovelace"}, Others: 0},
	})
	card, err := svc.GetPulse(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if card.Rank == nil || card.Rank.Rank != 2 || card.Rank.Points != 4 || card.Rank.GroupName != "Salı kortu" {
		t.Fatalf("rank=%+v", card.Rank)
	}
	if card.Friends == nil || len(card.Friends.Names) != 1 || card.Friends.Others != 0 {
		t.Fatalf("friends=%+v", card.Friends)
	}
	if card.Goal != nil {
		t.Fatal("default target 0 should hide the goal")
	}
}
