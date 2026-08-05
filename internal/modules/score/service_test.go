package score

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeFriends struct {
	ok  bool
	err error
}

func (f *fakeFriends) AreFriends(context.Context, string, string) (bool, error) {
	return f.ok, f.err
}

type fakePush struct {
	calls int
	last  string
}

func (p *fakePush) SendToUser(_ context.Context, userID, _, _ string, _ map[string]string) error {
	p.calls++
	p.last = userID
	return nil
}

type fakeScoreStore struct {
	match     Match
	live      LiveMatch
	matches   []Match
	insertErr error
	getErr    error
	liveErr   error
	ended     bool
	historyID string
}

func (f *fakeScoreStore) insert(_ context.Context, createdBy string, in CreateInput) (Match, error) {
	if f.insertErr != nil {
		return Match{}, f.insertErr
	}
	m := Match{
		ID: "hist-1", Sport: in.Sport, PlayerAName: in.PlayerAName, PlayerBName: in.PlayerBName,
		PlayerAUserID: in.PlayerAUserID, PlayerBUserID: in.PlayerBUserID,
		SetsA: in.SetsA, SetsB: in.SetsB, WinnerSide: in.WinnerSide, CreatedBy: createdBy,
		PlayedAt: time.Now(), CreatedAt: time.Now(),
	}
	f.historyID = m.ID
	return m, nil
}

func (f *fakeScoreStore) listForUser(context.Context, string, ListFilter, int, int) ([]Match, int64, error) {
	return f.matches, int64(len(f.matches)), nil
}

func (f *fakeScoreStore) getByID(context.Context, string) (Match, error) {
	return f.match, f.getErr
}

func (f *fakeScoreStore) deleteByID(context.Context, string, string) error { return nil }

func (f *fakeScoreStore) insertLive(_ context.Context, createdBy string, in LiveStartInput) (LiveMatch, error) {
	if f.liveErr != nil {
		return LiveMatch{}, f.liveErr
	}
	m := LiveMatch{
		ID: "live-1", Sport: in.Sport, Status: "LIVE",
		PlayerAName: in.PlayerAName, PlayerBName: in.PlayerBName,
		PlayerAUserID: in.PlayerAUserID, PlayerBUserID: in.PlayerBUserID,
		SetsToWin: in.SetsToWin, AdvantageEnabled: in.AdvantageEnabled, PointsToWin: in.PointsToWin,
		CreatedBy: createdBy, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.live = m
	return m, nil
}

func (f *fakeScoreStore) getLiveByID(context.Context, string) (LiveMatch, error) {
	if f.liveErr != nil {
		return LiveMatch{}, f.liveErr
	}
	return f.live, nil
}

func (f *fakeScoreStore) updateLiveScore(_ context.Context, _ string, u LiveScoreUpdate) (LiveMatch, error) {
	m := f.live
	if u.ScoreA != nil {
		m.ScoreA = *u.ScoreA
	}
	if u.ScoreB != nil {
		m.ScoreB = *u.ScoreB
	}
	f.live = m
	return m, nil
}

func (f *fakeScoreStore) endLive(_ context.Context, _ string, in LiveEndInput, historyID *string) (LiveMatch, error) {
	f.ended = true
	m := f.live
	m.Status = "ENDED"
	side := in.WinnerSide
	m.WinnerSide = &side
	m.HistoryMatchID = historyID
	f.live = m
	return m, nil
}

func (f *fakeScoreStore) cancelLive(_ context.Context, _ string) (LiveMatch, error) {
	m := f.live
	if m.Status != "IN_PROGRESS" && m.Status != "LIVE" {
		return LiveMatch{}, ErrWrongState
	}
	m.Status = "ENDED"
	f.live = m
	f.ended = true
	return m, nil
}

func (f *fakeScoreStore) listOpenLiveByCreator(_ context.Context, userID string, _, _ int) ([]LiveMatch, int64, error) {
	if f.live.CreatedBy == userID && (f.live.Status == "IN_PROGRESS" || f.live.Status == "LIVE") {
		return []LiveMatch{f.live}, 1, nil
	}
	return nil, 0, nil
}

func newScoreService(store *fakeScoreStore, friends FriendChecker, push PushSender) *Service {
	return &Service{repo: store, friends: friends, push: push}
}

func TestListUserMatches_RequiresFriendship(t *testing.T) {
	store := &fakeScoreStore{matches: []Match{{ID: "m1"}}}
	svc := newScoreService(store, &fakeFriends{ok: false}, nil)

	_, _, err := svc.ListUserMatches(context.Background(), "viewer", "target", ListFilter{}, 20, 0)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}

	svc = newScoreService(store, &fakeFriends{ok: true}, nil)
	got, _, err := svc.ListUserMatches(context.Background(), "viewer", "target", ListFilter{}, 20, 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestGetMatch_FriendOfParticipant(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{match: Match{ID: "m1", CreatedBy: "u9", PlayerAUserID: &opp}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	got, err := svc.GetMatch(context.Background(), "viewer", "m1")
	if err != nil || got.ID != "m1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestStartLiveMatch_FriendshipGate(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{ok: false}, nil)

	_, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", PlayerBUserID: &opp,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}

	push := &fakePush{}
	svc = newScoreService(store, &fakeFriends{ok: true}, push)
	got, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", PlayerBUserID: &opp,
	})
	if err != nil || got.ID != "live-1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if push.calls != 1 || push.last != "u2" {
		t.Fatalf("push calls=%d last=%q", push.calls, push.last)
	}
}

func TestGetLiveMatch_Visibility(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{ID: "live-1", CreatedBy: "u1", PlayerBUserID: &opp}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	if _, err := svc.GetLiveMatch(context.Background(), "stranger", "live-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger err=%v", err)
	}
	if _, err := svc.GetLiveMatch(context.Background(), "u2", "live-1"); err != nil {
		t.Fatalf("participant err=%v", err)
	}
}

func TestEndLiveMatch_ArchivesHistory(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "TENNIS", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B",
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	got, err := svc.EndLiveMatch(context.Background(), "u1", "live-1", LiveEndInput{
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.ended || store.historyID != "hist-1" {
		t.Fatalf("ended=%v history=%q", store.ended, store.historyID)
	}
	if got.HistoryMatchID == nil || *got.HistoryMatchID != "hist-1" {
		t.Fatalf("history id=%v", got.HistoryMatchID)
	}
}

func TestCancelLiveMatch_NoHistory(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "TENNIS", Status: "IN_PROGRESS", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B",
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	got, err := svc.CancelLiveMatch(context.Background(), "u1", "live-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ENDED" || store.historyID != "" {
		t.Fatalf("status=%s history=%q", got.Status, store.historyID)
	}
	if _, err := svc.CancelLiveMatch(context.Background(), "u2", "live-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other user err=%v", err)
	}
}

func TestListMyOpenLiveMatches(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "IN_PROGRESS", CreatedBy: "u1",
	}}
	svc := newScoreService(store, &fakeFriends{}, nil)
	items, total, err := svc.ListMyOpenLiveMatches(context.Background(), "u1", 20, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
	empty, total2, err := svc.ListMyOpenLiveMatches(context.Background(), "other", 20, 0)
	if err != nil || total2 != 0 || len(empty) != 0 {
		t.Fatalf("empty=%v total=%d err=%v", empty, total2, err)
	}
}

func TestCreateMatch_Validation(t *testing.T) {
	svc := newScoreService(&fakeScoreStore{}, &fakeFriends{}, nil)
	_, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "CHESS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}
