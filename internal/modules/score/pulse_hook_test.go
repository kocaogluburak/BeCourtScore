package score

import (
	"context"
	"testing"
)

type recPulse struct {
	calls [][]string
}

func (r *recPulse) OnMatchPulse(_ context.Context, ids []string) {
	r.calls = append(r.calls, append([]string{}, ids...))
}

type panicPulse struct{}

func (panicPulse) OnMatchPulse(context.Context, []string) { panic("pulse") }

func TestCreateMatch_NilPulseHookStillReturnsMatch(t *testing.T) {
	svc := newScoreService(&fakeScoreStore{}, &fakeFriends{ok: true}, nil)
	got, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		PlayerAUserID: strPtr("u1"), PlayerBUserID: strPtr("u2"),
	})
	if err != nil || got.ID == "" {
		t.Fatalf("match=%+v err=%v", got, err)
	}
}

func TestPulseHook_CreateEndDeleteAndPanic(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "TENNIS", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: strPtr("u1"), PlayerBUserID: strPtr("u2"),
	}}
	hook := &recPulse{}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetPulseHook(hook)

	if _, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		PlayerAUserID: strPtr("u1"), PlayerBUserID: strPtr("u2"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EndLiveMatch(context.Background(), "u1", "live-1", LiveEndInput{
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMatch(context.Background(), "u1", "hist-1"); err != nil {
		t.Fatal(err)
	}
	if len(hook.calls) != 3 {
		t.Fatalf("calls=%d", len(hook.calls))
	}
	for _, ids := range hook.calls {
		if len(ids) != 2 || ids[0] != "u1" || ids[1] != "u2" {
			t.Fatalf("ids=%v", ids)
		}
	}

	svc.SetPulseHook(panicPulse{})
	if _, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
	}); err != nil {
		t.Fatalf("panic hook failed the write: %v", err)
	}
}
