package score

import (
	"context"
	"errors"
	"testing"
	"time"

	"courtscore/internal/platform/sse"
)

func nick(s string) *string { return &s }

func TestCreateRematch_RequiresAcceptedFriend(t *testing.T) {
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{ok: false}, nil)
	_, err := svc.CreateRematch(context.Background(), "u1", RematchCreateInput{
		OpponentID: "u2", Sport: "TENNIS",
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

func TestCreateRematch_RejectsSelf(t *testing.T) {
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	_, err := svc.CreateRematch(context.Background(), "u1", RematchCreateInput{
		OpponentID: "u1", Sport: "TENNIS",
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestCreateRematch_DuplicatePendingConflict(t *testing.T) {
	store := &fakeScoreStore{rematchConflict: true}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	_, err := svc.CreateRematch(context.Background(), "u1", RematchCreateInput{
		OpponentID: "u2", Sport: "TENNIS",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

func TestCreateRematch_PushesAndPublishes(t *testing.T) {
	store := &fakeScoreStore{
		summaries: map[string]UserSummary{
			"u1": {ID: "u1", Nickname: nick("Ada")},
			"u2": {ID: "u2", Nickname: nick("Grace")},
		},
	}
	push := &fakePush{}
	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("u2")
	defer unsub()
	svc := newScoreService(store, &fakeFriends{ok: true}, push)
	svc.hub = hub
	got, err := svc.CreateRematch(context.Background(), "u1", RematchCreateInput{
		OpponentID: "u2", Sport: "TENNIS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "pending" || got.OpponentID != "u2" {
		t.Fatalf("challenge: %+v", got)
	}
	if push.calls != 1 || push.last != "u2" || push.typ != "rematch.received" {
		t.Fatalf("push: calls=%d last=%s typ=%s", push.calls, push.last, push.typ)
	}
	select {
	case ev := <-ch:
		if ev.Type != "rematch.received" {
			t.Fatalf("sse type=%s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("expected rematch.received SSE")
	}
}

func TestAcceptRematch_OpponentOnlyPrefill(t *testing.T) {
	store := &fakeScoreStore{rematches: []RematchChallenge{{
		ID: "rm-1", ChallengerID: "u1", OpponentID: "u2", Sport: "PADEL",
		SetsToWin: 2, AdvantageEnabled: true, PointsToWin: 11, Status: "pending",
		CreatedAt: time.Now(),
	}}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	if _, err := svc.AcceptRematch(context.Background(), "u1", "rm-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("challenger accept: %v", err)
	}
	prefill, err := svc.AcceptRematch(context.Background(), "u2", "rm-1")
	if err != nil {
		t.Fatal(err)
	}
	if prefill.Sport != "PADEL" || prefill.Opponent.ID != "u1" || prefill.LiveMatchID != nil {
		t.Fatalf("prefill: %+v", prefill)
	}
	if store.rematches[0].Status != "accepted" {
		t.Fatalf("status=%s", store.rematches[0].Status)
	}
}

func TestAcceptRematch_PublishesChallengerFacingPrefill(t *testing.T) {
	store := &fakeScoreStore{
		rematches: []RematchChallenge{{
			ID: "rm-1", ChallengerID: "u1", OpponentID: "u2", Sport: "PADEL",
			SetsToWin: 2, AdvantageEnabled: true, PointsToWin: 11, Status: "pending",
			CreatedAt: time.Now(),
		}},
		summaries: map[string]UserSummary{
			"u1": {ID: "u1", Nickname: nick("Ada")},
			"u2": {ID: "u2", Nickname: nick("Grace")},
		},
	}
	push := &fakePush{}
	hub := sse.NewHub()
	ch, unsub := hub.Subscribe("u1")
	defer unsub()
	svc := newScoreService(store, &fakeFriends{ok: true}, push)
	svc.hub = hub
	if _, err := svc.AcceptRematch(context.Background(), "u2", "rm-1"); err != nil {
		t.Fatal(err)
	}
	if push.last != "u1" || push.typ != "rematch.accepted" {
		t.Fatalf("push last=%s typ=%s", push.last, push.typ)
	}
	if push.data["opponent_id"] != "u2" || push.data["sport"] != "PADEL" {
		t.Fatalf("push extras: %+v", push.data)
	}
	select {
	case ev := <-ch:
		if ev.Type != "rematch.accepted" {
			t.Fatalf("sse type=%s", ev.Type)
		}
		prefill, ok := ev.Data.(RematchPrefill)
		if !ok {
			t.Fatalf("sse data %T", ev.Data)
		}
		if prefill.Opponent.ID != "u2" {
			t.Fatalf("challenger SSE opponent=%s want u2 (acceptor)", prefill.Opponent.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected rematch.accepted SSE")
	}
}

func TestAcceptRematch_WrongState(t *testing.T) {
	store := &fakeScoreStore{rematches: []RematchChallenge{{
		ID: "rm-1", ChallengerID: "u1", OpponentID: "u2", Sport: "TENNIS",
		Status: "rejected", CreatedAt: time.Now(),
	}}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	if _, err := svc.AcceptRematch(context.Background(), "u2", "rm-1"); !errors.Is(err, ErrWrongState) {
		t.Fatalf("got %v, want ErrWrongState", err)
	}
}

func TestRejectAndCancelRematch(t *testing.T) {
	store := &fakeScoreStore{rematches: []RematchChallenge{{
		ID: "rm-1", ChallengerID: "u1", OpponentID: "u2", Sport: "TENNIS",
		Status: "pending", CreatedAt: time.Now(),
	}}}
	push := &fakePush{}
	svc := newScoreService(store, &fakeFriends{ok: true}, push)
	if _, err := svc.RejectRematch(context.Background(), "u1", "rm-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("challenger reject: %v", err)
	}
	got, err := svc.RejectRematch(context.Background(), "u2", "rm-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "rejected" {
		t.Fatalf("status=%s", got.Status)
	}
	if push.typ != "rematch.rejected" {
		t.Fatalf("push type=%s", push.typ)
	}

	store.rematches = []RematchChallenge{{
		ID: "rm-2", ChallengerID: "u1", OpponentID: "u2", Sport: "TENNIS",
		Status: "pending", CreatedAt: time.Now(),
	}}
	if _, err := svc.CancelRematch(context.Background(), "u2", "rm-2"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("opponent cancel: %v", err)
	}
	cancelled, err := svc.CancelRematch(context.Background(), "u1", "rm-2")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("status=%s", cancelled.Status)
	}
	if push.typ != "rematch.cancelled" {
		t.Fatalf("push type=%s", push.typ)
	}
}

func TestRematchExpiry48h(t *testing.T) {
	old := time.Now().Add(-49 * time.Hour)
	fresh := time.Now()
	store := &fakeScoreStore{rematches: []RematchChallenge{
		{ID: "old", ChallengerID: "u1", OpponentID: "u2", Sport: "TENNIS", Status: "pending", CreatedAt: old},
		{ID: "new", ChallengerID: "u1", OpponentID: "u3", Sport: "TENNIS", Status: "pending", CreatedAt: fresh},
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	items, total, err := svc.ListIncomingRematches(context.Background(), "u2", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("expired still listed: items=%d total=%d", len(items), total)
	}
	still, _, err := svc.ListOutgoingRematches(context.Background(), "u1", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(still) != 1 || still[0].ID != "new" {
		t.Fatalf("fresh challenge missing: %+v", still)
	}
}

func TestCreateRematch_CopiesRulesFromSourceMatch(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{
		match: Match{
			ID: "m-1", Sport: "SQUASH", CreatedBy: "u1",
			PlayerAName: "Ada", PlayerBName: "Grace",
			PlayerAUserID: strPtr("u1"), PlayerBUserID: &opp,
			SetRule: "BEST_OF_3", FinalSet: "TIEBREAK",
		},
	}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	src := "m-1"
	got, err := svc.CreateRematch(context.Background(), "u1", RematchCreateInput{
		OpponentID: "u2", SourceMatchID: &src,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sport != "SQUASH" || got.SetRule == nil || *got.SetRule != "BEST_OF_3" {
		t.Fatalf("rules not copied: %+v", got)
	}
}
