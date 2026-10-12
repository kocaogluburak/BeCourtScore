package score

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
)

type fakeFriends struct {
	ok  bool
	err error
}

func (f *fakeFriends) AreFriends(context.Context, string, string) (bool, error) {
	return f.ok, f.err
}

type fakePush struct {
	calls  int
	last   string
	typ    string
	users  []string
	bodies []string
}

func (p *fakePush) SendToUser(_ context.Context, userID, _, body string, data map[string]string) error {
	p.calls++
	p.last = userID
	p.users = append(p.users, userID)
	p.bodies = append(p.bodies, body)
	if data != nil {
		p.typ = data["type"]
	}
	return nil
}

type fakeScoreStore struct {
	match      Match
	live       LiveMatch
	matches    []Match
	insertErr  error
	getErr     error
	liveErr    error
	ended      bool
	historyID  string
	openExists bool
	ratings    map[string]lockedRating
}

func (f *fakeScoreStore) insert(_ context.Context, createdBy string, in CreateInput) (Match, error) {
	if f.insertErr != nil {
		return Match{}, f.insertErr
	}
	scores := in.SetScores
	if scores == nil {
		scores = []SetScore{}
	}
	m := Match{
		ID: "hist-1", Sport: in.Sport, PlayerAName: in.PlayerAName, PlayerBName: in.PlayerBName,
		PlayerAUserID: in.PlayerAUserID, PlayerBUserID: in.PlayerBUserID,
		PlayerA2Name: in.PlayerA2Name, PlayerB2Name: in.PlayerB2Name,
		PlayerA2UserID: in.PlayerA2UserID, PlayerB2UserID: in.PlayerB2UserID,
		SetsA: in.SetsA, SetsB: in.SetsB, SetScores: scores, WinnerSide: in.WinnerSide, CreatedBy: createdBy,
		PlayedAt: time.Now(), CreatedAt: time.Now(),
		PointLog: in.PointLog, DurationSeconds: in.DurationSeconds,
	}
	f.match = m
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

func (f *fakeScoreStore) listParticipantMatches(_ context.Context, userID, sport string) ([]Match, error) {
	out := []Match{}
	for _, m := range f.matches {
		if sport != "" && m.Sport != sport {
			continue
		}
		if matchHasUser(m, userID) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeScoreStore) listH2HMatches(_ context.Context, userA, userB, sport string) ([]Match, error) {
	out := []Match{}
	for _, m := range f.matches {
		if sport != "" && m.Sport != sport {
			continue
		}
		onA := idEq(m.PlayerAUserID, userA) || idEq(m.PlayerA2UserID, userA)
		onB := idEq(m.PlayerBUserID, userB) || idEq(m.PlayerB2UserID, userB)
		swapA := idEq(m.PlayerAUserID, userB) || idEq(m.PlayerA2UserID, userB)
		swapB := idEq(m.PlayerBUserID, userA) || idEq(m.PlayerB2UserID, userA)
		if (onA && onB) || (swapA && swapB) {
			out = append(out, m)
		}
	}
	return out, nil
}

func ratingKey(userID, sport string) string { return userID + "\x00" + sport }

func (f *fakeScoreStore) applySinglesElo(_ context.Context, sport, winnerID, loserID string) error {
	if f.ratings == nil {
		f.ratings = map[string]lockedRating{}
	}
	firstID, secondID := winnerID, loserID
	firstWon := true
	if loserID < winnerID {
		firstID, secondID = loserID, winnerID
		firstWon = false
	}
	first := f.ratings[ratingKey(firstID, sport)]
	second := f.ratings[ratingKey(secondID, sport)]
	if first.rating == 0 {
		first.rating = initialRating
	}
	if second.rating == 0 {
		second.rating = initialRating
	}
	nextFirst, nextSecond := nextRatings(first.rating, second.rating, first.counted, second.counted, firstWon)
	f.ratings[ratingKey(firstID, sport)] = lockedRating{rating: nextFirst, counted: first.counted + 1}
	f.ratings[ratingKey(secondID, sport)] = lockedRating{rating: nextSecond, counted: second.counted + 1}
	return nil
}

func (f *fakeScoreStore) listRatings(_ context.Context, userID string) ([]UserRating, error) {
	out := []UserRating{}
	prefix := userID + "\x00"
	for key, row := range f.ratings {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		out = append(out, UserRating{Sport: key[len(prefix):], Rating: row.rating, MatchesCounted: row.counted})
	}
	return out, nil
}

func (f *fakeScoreStore) insertLive(_ context.Context, createdBy string, in LiveStartInput) (LiveMatch, error) {
	if f.liveErr != nil {
		return LiveMatch{}, f.liveErr
	}
	status := in.Status
	if status == "" {
		status = "IN_PROGRESS"
	}
	m := LiveMatch{
		ID: "live-1", Sport: in.Sport, Status: status,
		PlayerAName: in.PlayerAName, PlayerBName: in.PlayerBName,
		PlayerAUserID: in.PlayerAUserID, PlayerBUserID: in.PlayerBUserID,
		PlayerA2Name: in.PlayerA2Name, PlayerB2Name: in.PlayerB2Name,
		PlayerA2UserID: in.PlayerA2UserID, PlayerB2UserID: in.PlayerB2UserID,
		SetsToWin: in.SetsToWin, SetRule: in.SetRule, FinalSet: in.FinalSet,
		AdvantageEnabled: in.AdvantageEnabled, PointsToWin: in.PointsToWin,
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

func (f *fakeScoreStore) hasOpenLiveBetween(context.Context, string, string) (bool, error) {
	return f.openExists, nil
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
	if m.Status != "IN_PROGRESS" && m.Status != "PENDING" {
		return LiveMatch{}, ErrWrongState
	}
	m.Status = "ENDED"
	f.live = m
	f.ended = true
	return m, nil
}

func (f *fakeScoreStore) acceptLive(_ context.Context, _ string) (LiveMatch, error) {
	m := f.live
	if m.Status != "PENDING" {
		return LiveMatch{}, ErrWrongState
	}
	m.Status = "IN_PROGRESS"
	f.live = m
	return m, nil
}

func (f *fakeScoreStore) listOpenLiveForParticipant(_ context.Context, userID string, _, _ int) ([]LiveMatch, int64, error) {
	open := f.live.Status == "IN_PROGRESS" || f.live.Status == "PENDING"
	if !open {
		return nil, 0, nil
	}
	if f.live.CreatedBy == userID || (liveUserInSlots(userID, f.live) && !containsID(f.live.DeclinedUserIDs, userID)) {
		return []LiveMatch{f.live}, 1, nil
	}
	return nil, 0, nil
}

func (f *fakeScoreStore) expireStaleLive(_ context.Context, olderThan time.Time) ([]LiveMatch, error) {
	open := f.live.Status == "IN_PROGRESS" || f.live.Status == "PENDING"
	// Zero UpdatedAt means the fixture never set activity time — treat as fresh.
	if !open || f.live.UpdatedAt.IsZero() || !f.live.UpdatedAt.Before(olderThan) {
		return nil, nil
	}
	m := f.live
	m.Status = "ENDED"
	f.live = m
	f.ended = true
	f.openExists = false
	return []LiveMatch{m}, nil
}

func (f *fakeScoreStore) setDeclined(_ context.Context, _ string, userIDs []string) (LiveMatch, error) {
	m := f.live
	if m.Status != "PENDING" {
		return LiveMatch{}, ErrWrongState
	}
	m.DeclinedUserIDs = append([]string{}, userIDs...)
	f.live = m
	return m, nil
}

func (f *fakeScoreStore) setShareToken(_ context.Context, _ string, token string) (LiveMatch, error) {
	m := f.live
	m.ShareToken = &token
	f.live = m
	return m, nil
}

func (f *fakeScoreStore) getLiveByShareToken(_ context.Context, token string) (LiveMatch, error) {
	if f.live.ShareToken != nil && *f.live.ShareToken == token {
		return f.live, nil
	}
	return LiveMatch{}, ErrNotFound
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
	if got.Status != "PENDING" {
		t.Fatalf("status=%q want PENDING", got.Status)
	}
	if push.calls != 1 || push.last != "u2" || push.typ != "match.invite" {
		t.Fatalf("push calls=%d last=%q typ=%q", push.calls, push.last, push.typ)
	}
}

func TestStartLiveMatch_GuestIsInProgress(t *testing.T) {
	store := &fakeScoreStore{}
	push := &fakePush{}
	svc := newScoreService(store, &fakeFriends{ok: true}, push)
	got, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "Guest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "IN_PROGRESS" {
		t.Fatalf("status=%q", got.Status)
	}
	if push.calls != 0 {
		t.Fatalf("push calls=%d", push.calls)
	}
}

func TestStartLiveMatch_DuplicateOpenConflict(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{openExists: true}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	_, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", PlayerBUserID: &opp,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
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

func TestCancelLiveMatch_Pending(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "PENDING", CreatedBy: "u1",
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err := svc.CancelLiveMatch(context.Background(), "u1", "live-1")
	if err != nil || got.Status != "ENDED" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestCancelLiveMatch_OpponentInProgress(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "TENNIS", Status: "IN_PROGRESS", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B", PlayerBUserID: &opp,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	got, err := svc.CancelLiveMatch(context.Background(), "u2", "live-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ENDED" || store.historyID != "" {
		t.Fatalf("status=%s history=%q", got.Status, store.historyID)
	}
}

func TestCancelLiveMatch_OpponentPendingForbidden(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "PENDING", CreatedBy: "u1", PlayerBUserID: &opp,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	if _, err := svc.CancelLiveMatch(context.Background(), "u2", "live-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
}

func TestCancelLiveMatch_StrangerForbidden(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "IN_PROGRESS", CreatedBy: "u1", PlayerBUserID: &opp,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	if _, err := svc.CancelLiveMatch(context.Background(), "u3", "live-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
}

func TestExpireStaleLive_ListClearsOpen(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "IN_PROGRESS", CreatedBy: "u1", PlayerBUserID: &opp,
		UpdatedAt: time.Now().Add(-25 * time.Hour),
	}, openExists: true}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	items, total, err := svc.ListMyOpenLiveMatches(context.Background(), "u2", 20, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
	if store.live.Status != "ENDED" {
		t.Fatalf("status=%s", store.live.Status)
	}
}

func TestExpireStaleLive_StartAllowsNewAfterStale(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-old", Status: "IN_PROGRESS", CreatedBy: "u1", PlayerBUserID: &opp,
		UpdatedAt: time.Now().Add(-25 * time.Hour),
	}, openExists: true}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	got, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", PlayerBUserID: &opp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "PENDING" {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestExpireStaleLive_FreshNotExpired(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "IN_PROGRESS", CreatedBy: "u1", PlayerBUserID: &opp,
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	items, total, err := svc.ListMyOpenLiveMatches(context.Background(), "u1", 20, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
	if store.live.Status != "IN_PROGRESS" {
		t.Fatalf("status=%s", store.live.Status)
	}
}

func TestListMyOpenLiveMatches(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "PENDING", CreatedBy: "u1", PlayerBUserID: &opp,
	}}
	svc := newScoreService(store, &fakeFriends{}, nil)
	items, total, err := svc.ListMyOpenLiveMatches(context.Background(), "u1", 20, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
	asOpp, total2, err := svc.ListMyOpenLiveMatches(context.Background(), "u2", 20, 0)
	if err != nil || total2 != 1 || len(asOpp) != 1 {
		t.Fatalf("opp items=%v total=%d err=%v", asOpp, total2, err)
	}
	empty, total3, err := svc.ListMyOpenLiveMatches(context.Background(), "other", 20, 0)
	if err != nil || total3 != 0 || len(empty) != 0 {
		t.Fatalf("empty=%v total=%d err=%v", empty, total3, err)
	}
}

func TestAcceptLiveMatch(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "PENDING", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B", PlayerBUserID: &opp,
	}}
	push := &fakePush{}
	svc := newScoreService(store, &fakeFriends{ok: true}, push)

	if _, err := svc.AcceptLiveMatch(context.Background(), "u1", "live-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("creator accept err=%v", err)
	}
	got, err := svc.AcceptLiveMatch(context.Background(), "u2", "live-1")
	if err != nil || got.Status != "IN_PROGRESS" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if push.calls != 1 || push.last != "u1" || push.typ != "match.accepted" {
		t.Fatalf("push calls=%d last=%q typ=%q", push.calls, push.last, push.typ)
	}
}

func TestDeclineLiveMatch(t *testing.T) {
	opp := "u2"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "PENDING", CreatedBy: "u1", PlayerBUserID: &opp,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err := svc.DeclineLiveMatch(context.Background(), "u2", "live-1")
	if err != nil || got.Status != "ENDED" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	store.live.Status = "IN_PROGRESS"
	if _, err := svc.DeclineLiveMatch(context.Background(), "u2", "live-1"); !errors.Is(err, ErrWrongState) {
		t.Fatalf("err=%v", err)
	}
}

func TestStartLiveMatch_DoublesInvitesEveryRegisteredPlayer(t *testing.T) {
	partner, opp, opp2 := "p", "o1", "o2"
	partnerName, opp2Name := "Partner", "Opp2"
	store := &fakeScoreStore{}
	push := &fakePush{}
	svc := newScoreService(store, &fakeFriends{ok: true}, push)
	got, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "PADEL", PlayerAName: "A", PlayerBName: "B",
		PlayerA2Name: &partnerName, PlayerA2UserID: &partner,
		PlayerBUserID: &opp, PlayerB2Name: &opp2Name, PlayerB2UserID: &opp2,
	})
	if err != nil || got.Status != "PENDING" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if push.calls != 3 || push.typ != "match.invite" {
		t.Fatalf("push calls=%d users=%v typ=%q", push.calls, push.users, push.typ)
	}
	for _, id := range push.users {
		if id == "u1" {
			t.Fatalf("starter was invited: %v", push.users)
		}
	}
	for _, body := range push.bodies {
		if body != "A / Partner vs B / Opp2 — accept to watch the score" {
			t.Fatalf("body=%q", body)
		}
		if strings.Contains(body, "null") || strings.Contains(body, " /  /") {
			t.Fatalf("broken label in %q", body)
		}
	}
}

func TestStartLiveMatch_PartnerNotFriendSendsNoInvite(t *testing.T) {
	partner := "p"
	name := "Partner"
	store := &fakeScoreStore{}
	push := &fakePush{}
	svc := newScoreService(store, &fakeFriends{ok: false}, push)
	_, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "PADEL", PlayerAName: "A", PlayerBName: "B",
		PlayerA2Name: &name, PlayerA2UserID: &partner,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
	if push.calls != 0 {
		t.Fatalf("partial invite calls=%d", push.calls)
	}
}

func TestStartLiveMatch_GuestOpponentIsNotInvited(t *testing.T) {
	partner := "p"
	name := "Cara"
	store := &fakeScoreStore{}
	push := &fakePush{}
	svc := newScoreService(store, &fakeFriends{ok: true}, push)
	got, err := svc.StartLiveMatch(context.Background(), "u1", LiveStartInput{
		Sport: "PADEL", PlayerAName: "Ada", PlayerBName: "Guest",
		PlayerA2Name: &name, PlayerA2UserID: &partner,
	})
	if err != nil || got.Status != "PENDING" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(push.users) != 1 || push.users[0] != "p" {
		t.Fatalf("users=%v", push.users)
	}
	if push.bodies[0] != "Ada / Cara vs Guest — accept to watch the score" {
		t.Fatalf("body=%q", push.bodies[0])
	}
}

func TestDeclineLiveMatch_OneOfManyKeepsMatch(t *testing.T) {
	partner, opp := "p", "o1"
	partnerName := "Partner"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Status: "PENDING", CreatedBy: "u1",
		PlayerA2Name: &partnerName, PlayerA2UserID: &partner, PlayerBUserID: &opp,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err := svc.DeclineLiveMatch(context.Background(), "p", "live-1")
	if err != nil || got.Status != "PENDING" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if !containsID(store.live.DeclinedUserIDs, "p") {
		t.Fatalf("declined=%v", store.live.DeclinedUserIDs)
	}
	ended, err := svc.DeclineLiveMatch(context.Background(), "o1", "live-1")
	if err != nil || ended.Status != "ENDED" {
		t.Fatalf("got=%+v err=%v", ended, err)
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

func TestCreateMatch_PersistsSetScores(t *testing.T) {
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{}, nil)
	got, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B",
		SetsA: 2, SetsB: 1, WinnerSide: "A",
		SetScores: []SetScore{{A: 6, B: 4}, {A: 3, B: 6}, {A: 7, B: 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SetScores) != 3 || got.SetScores[0].A != 6 || got.SetScores[2].B != 5 {
		t.Fatalf("set_scores=%+v", got.SetScores)
	}
	if len(store.match.SetScores) != 3 {
		t.Fatalf("store set_scores=%+v", store.match.SetScores)
	}
}

func TestCreateMatch_RejectsBadSetScores(t *testing.T) {
	svc := newScoreService(&fakeScoreStore{}, &fakeFriends{}, nil)
	_, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B",
		SetsA: 2, SetsB: 0, WinnerSide: "A",
		SetScores: []SetScore{{A: -1, B: 4}},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetUserStats_SelfAndFriendGate(t *testing.T) {
	me, opp := "u1", "u2"
	store := &fakeScoreStore{matches: []Match{
		{Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp,
			PlayerAName: "Me", PlayerBName: "Opp", SetsA: 2, SetsB: 0, WinnerSide: "A",
			PlayedAt: time.Now()},
	}}
	svc := newScoreService(store, &fakeFriends{ok: false}, nil)

	got, err := svc.GetUserStats(context.Background(), me, me, "")
	if err != nil || got.Matches != 1 || got.Wins != 1 {
		t.Fatalf("self stats=%+v err=%v", got, err)
	}

	_, err = svc.GetUserStats(context.Background(), "stranger", me, "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger err=%v", err)
	}

	svc = newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err = svc.GetUserStats(context.Background(), "friend", me, "TENNIS")
	if err != nil || got.Matches != 1 {
		t.Fatalf("friend stats=%+v err=%v", got, err)
	}
}

func TestGetHeadToHead_SelfRejectedAndEmptyOK(t *testing.T) {
	me, opp := "u1", "u2"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)

	_, err := svc.GetHeadToHead(context.Background(), me, me, "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("self h2h err=%v", err)
	}

	got, err := svc.GetHeadToHead(context.Background(), me, opp, "")
	if err != nil || got.Matches != 0 || got.OpponentUserID != opp {
		t.Fatalf("empty h2h=%+v err=%v", got, err)
	}

	svc = newScoreService(store, &fakeFriends{ok: false}, nil)
	_, err = svc.GetHeadToHead(context.Background(), me, opp, "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-friend err=%v", err)
	}
}

type fakeGate struct {
	err error
}

func (g *fakeGate) AllowCloudSync(context.Context, string) error  { return g.err }
func (g *fakeGate) AllowStats(context.Context, string) error      { return g.err }
func (g *fakeGate) AllowPublicLive(context.Context, string) error { return g.err }
func (g *fakeGate) AllowLargeTournament(context.Context, string, int) error {
	return g.err
}
func (g *fakeGate) MaxParticipants(context.Context, string) (int, error) {
	return 8, nil
}

func TestCreateMatch_CapsPointLog(t *testing.T) {
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	points := make([]PointEvent, maxPointLog+25)
	for i := range points {
		points[i] = PointEvent{Side: "A", AtMs: int64(i)}
	}
	dur := 90
	_, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		PointLog: points, DurationSeconds: &dur,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.match.PointLog) != maxPointLog || store.match.DurationSeconds == nil || *store.match.DurationSeconds != 90 {
		t.Fatalf("stored log=%d duration=%v", len(store.match.PointLog), store.match.DurationSeconds)
	}
}

func TestCreateMatch_ProRequired(t *testing.T) {
	svc := newScoreService(&fakeScoreStore{}, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired})
	_, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
	})
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestEndLiveMatch_SkipsHistoryWhenProRequired(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{ID: "live-1", CreatedBy: "u1", Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B"}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired})
	got, err := svc.EndLiveMatch(context.Background(), "u1", "live-1", LiveEndInput{WinnerSide: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Archived || store.historyID != "" {
		t.Fatalf("should not archive: %+v history=%s", got, store.historyID)
	}
}

func TestShareLiveMatch_ProRequired(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{ID: "live-1", CreatedBy: "u1"}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired})
	_, _, err := svc.ShareLiveMatch(context.Background(), "u1", "live-1")
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateMatch_RatesRegisteredSingles(t *testing.T) {
	a, b := "ua", "ub"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{}, nil)
	_, err := svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: &a, PlayerBUserID: &b,
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.ratings[ratingKey(a, "TENNIS")].rating != 1516 || store.ratings[ratingKey(b, "TENNIS")].rating != 1484 {
		t.Fatalf("ratings=%v", store.ratings)
	}

	_, err = svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "PADEL", PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: &a, PlayerBUserID: &b,
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.ratings[ratingKey(a, "TENNIS")].rating != 1516 {
		t.Fatal("padel result changed tennis rating")
	}
	if store.ratings[ratingKey(a, "PADEL")].rating != 1516 {
		t.Fatalf("padel=%d", store.ratings[ratingKey(a, "PADEL")].rating)
	}
}

func TestCreateMatch_GuestAndDoublesSkipRating(t *testing.T) {
	a := "ua"
	partner := "Bea"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{}, nil)
	if _, err := svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "Guest",
		PlayerAUserID: &a, SetsA: 2, SetsB: 0, WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	b := "ub"
	if _, err := svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: &a, PlayerBUserID: &b, PlayerA2Name: &partner,
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if len(store.ratings) != 0 {
		t.Fatalf("ratings=%v", store.ratings)
	}
}

func TestEndLiveMatch_RatesArchivedSingles(t *testing.T) {
	a, b := "ua", "ub"
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "SQUASH", CreatedBy: a,
		PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: &a, PlayerBUserID: &b,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	if _, err := svc.EndLiveMatch(context.Background(), a, "live-1", LiveEndInput{
		SetsA: 3, SetsB: 1, WinnerSide: "B",
	}); err != nil {
		t.Fatal(err)
	}
	if store.ratings[ratingKey(b, "SQUASH")].rating != 1516 || store.ratings[ratingKey(a, "SQUASH")].rating != 1484 {
		t.Fatalf("ratings=%v", store.ratings)
	}
}

func TestGetUserStats_IncludesSportRating(t *testing.T) {
	me, opp := "u1", "u2"
	store := &fakeScoreStore{
		matches: []Match{{
			Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp,
			PlayerAName: "Me", PlayerBName: "Opp", SetsA: 2, SetsB: 0, WinnerSide: "A",
			PlayedAt: time.Now(),
		}},
		ratings: map[string]lockedRating{
			ratingKey(me, "TENNIS"): {rating: 1516, counted: 1},
			ratingKey(me, "PADEL"):  {rating: 1400, counted: 3},
		},
	}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err := svc.GetUserStats(context.Background(), me, me, "TENNIS")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rating != 1516 {
		t.Fatalf("rating=%d", got.Rating)
	}
	for _, s := range got.BySport {
		if s.Sport == "PADEL" {
			t.Fatalf("padel should not be in by_sport without matches: %+v", got.BySport)
		}
		if s.Sport == "TENNIS" && s.Rating != 1516 {
			t.Fatalf("tennis by_sport=%+v", s)
		}
	}
}
