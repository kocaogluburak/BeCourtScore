package score

import (
	"context"
	"errors"
	"fmt"
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
	data   map[string]string
}

func (p *fakePush) SendToUser(_ context.Context, userID, _, body string, data map[string]string) error {
	p.calls++
	p.last = userID
	p.users = append(p.users, userID)
	p.bodies = append(p.bodies, body)
	p.data = data
	if data != nil {
		p.typ = data["type"]
	}
	return nil
}

type fakeScoreStore struct {
	match           Match
	live            LiveMatch
	matches         []Match
	insertErr       error
	getErr          error
	liveErr         error
	ended           bool
	historyID       string
	openExists      bool
	ratings         map[string]lockedRating
	history         []ratingHistoryRow
	seq             int
	nights          SocialNightStats
	socialH2H       SocialH2H
	ratedDoubles    map[string]bool
	rematches       []RematchChallenge
	rematchConflict bool
	summaries       map[string]UserSummary
}

func (f *fakeScoreStore) insert(_ context.Context, createdBy string, in CreateInput) (Match, error) {
	if f.insertErr != nil {
		return Match{}, f.insertErr
	}
	scores := in.SetScores
	if scores == nil {
		scores = []SetScore{}
	}
	f.seq++
	id := "hist-1"
	if f.seq > 1 {
		id = fmt.Sprintf("hist-%d", f.seq)
	}
	m := Match{
		ID: id, Sport: in.Sport, PlayerAName: in.PlayerAName, PlayerBName: in.PlayerBName,
		PlayerAUserID: in.PlayerAUserID, PlayerBUserID: in.PlayerBUserID,
		PlayerA2Name: in.PlayerA2Name, PlayerB2Name: in.PlayerB2Name,
		PlayerA2UserID: in.PlayerA2UserID, PlayerB2UserID: in.PlayerB2UserID,
		SetsA: in.SetsA, SetsB: in.SetsB, SetScores: scores, WinnerSide: in.WinnerSide, CreatedBy: createdBy,
		PlayedAt: time.Now(), CreatedAt: time.Now(),
		PointLog: in.PointLog, DurationSeconds: in.DurationSeconds,
		AvgHeartRate: in.AvgHeartRate, MaxHeartRate: in.MaxHeartRate,
		ActiveCalories: in.ActiveCalories, FitnessSource: in.FitnessSource,
		FitnessSamples: in.FitnessSamples,
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

func (f *fakeScoreStore) listParticipantMatchesInRange(_ context.Context, userID string, from, to time.Time) ([]Match, error) {
	out := []Match{}
	for _, m := range f.matches {
		if !matchHasUser(m, userID) {
			continue
		}
		if m.PlayedAt.Before(from) || !m.PlayedAt.Before(to) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeScoreStore) firstMatchPerSport(_ context.Context, userID string) ([]SportFirst, error) {
	earliest := map[string]time.Time{}
	for _, m := range f.matches {
		if !matchHasUser(m, userID) {
			continue
		}
		if prev, ok := earliest[m.Sport]; !ok || m.PlayedAt.Before(prev) {
			earliest[m.Sport] = m.PlayedAt
		}
	}
	out := []SportFirst{}
	for sport, t := range earliest {
		out = append(out, SportFirst{Sport: sport, PlayedAt: t})
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

func (f *fakeScoreStore) applySinglesElo(_ context.Context, sport, winnerID, loserID, matchID string, playedAt time.Time) error {
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
	if playedAt.IsZero() {
		playedAt = time.Now().UTC()
	}
	f.history = append(f.history,
		ratingHistoryRow{UserID: firstID, Sport: sport, Rating: nextFirst, Counted: first.counted + 1, MatchID: matchID, PlayedAt: playedAt, Discipline: DisciplineSingles},
		ratingHistoryRow{UserID: secondID, Sport: sport, Rating: nextSecond, Counted: second.counted + 1, MatchID: matchID, PlayedAt: playedAt, Discipline: DisciplineSingles},
	)
	return nil
}

func doublesRatingKey(userID, sport string) string {
	return ratingKey(userID, sport) + "\x00" + DisciplineDoubles
}

func (f *fakeScoreStore) applyDoublesElo(_ context.Context, sport string, sideA, sideB [2]string, aWon bool, matchID, tournamentMatchID string, playedAt time.Time) error {
	if f.ratedDoubles == nil {
		f.ratedDoubles = map[string]bool{}
	}
	key := matchID
	if tournamentMatchID != "" {
		key = "t:" + tournamentMatchID
	}
	if key != "" && f.ratedDoubles[key] {
		return nil
	}
	if key != "" {
		f.ratedDoubles[key] = true
	}
	if f.ratings == nil {
		f.ratings = map[string]lockedRating{}
	}
	var aRows, bRows [2]lockedRating
	for i := 0; i < 2; i++ {
		aRows[i] = f.ratings[doublesRatingKey(sideA[i], sport)]
		if aRows[i].rating == 0 {
			aRows[i].rating = initialRating
		}
		bRows[i] = f.ratings[doublesRatingKey(sideB[i], sport)]
		if bRows[i].rating == 0 {
			bRows[i].rating = initialRating
		}
	}
	nextA, nextB := nextDoublesRatings(aRows, bRows, aWon)
	if playedAt.IsZero() {
		playedAt = time.Now().UTC()
	}
	for i := 0; i < 2; i++ {
		f.ratings[doublesRatingKey(sideA[i], sport)] = lockedRating{rating: nextA[i], counted: aRows[i].counted + 1}
		f.ratings[doublesRatingKey(sideB[i], sport)] = lockedRating{rating: nextB[i], counted: bRows[i].counted + 1}
		f.history = append(f.history,
			ratingHistoryRow{UserID: sideA[i], Sport: sport, Rating: nextA[i], Counted: aRows[i].counted + 1, MatchID: matchID, PlayedAt: playedAt, Discipline: DisciplineDoubles},
			ratingHistoryRow{UserID: sideB[i], Sport: sport, Rating: nextB[i], Counted: bRows[i].counted + 1, MatchID: matchID, PlayedAt: playedAt, Discipline: DisciplineDoubles},
		)
	}
	return nil
}

func (f *fakeScoreStore) listUserSocialNights(context.Context, string, string) (SocialNightStats, error) {
	return f.nights, nil
}

func (f *fakeScoreStore) listSocialH2H(context.Context, string, string, string) (SocialH2H, error) {
	return f.socialH2H, nil
}

func (f *fakeScoreStore) ratingPeakInRange(_ context.Context, userID string, from, to time.Time) (*WrappedElo, error) {
	var rows []WrappedElo
	for _, h := range f.history {
		if h.UserID != userID || !isSinglesDiscipline(h.Discipline) {
			continue
		}
		if h.PlayedAt.Before(from) || !h.PlayedAt.Before(to) {
			continue
		}
		t := h.PlayedAt
		rows = append(rows, WrappedElo{Sport: h.Sport, Rating: h.Rating, PlayedAt: &t})
	}
	return pickEloPeak(rows), nil
}

func (f *fakeScoreStore) listRatings(_ context.Context, userID string) ([]UserRating, error) {
	out := []UserRating{}
	prefix := userID + "\x00"
	for key, row := range f.ratings {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		rest := key[len(prefix):]
		sport, discipline, found := strings.Cut(rest, "\x00")
		if !found {
			discipline = DisciplineSingles
		}
		out = append(out, UserRating{Sport: sport, Discipline: discipline, Rating: row.rating, MatchesCounted: row.counted})
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

func (f *fakeScoreStore) insertRematch(_ context.Context, challengerID string, in RematchCreateInput) (RematchChallenge, error) {
	if f.rematchConflict {
		return RematchChallenge{}, ErrConflict
	}
	adv := true
	if in.AdvantageEnabled != nil {
		adv = *in.AdvantageEnabled
	}
	ch := RematchChallenge{
		ID: "rm-1", ChallengerID: challengerID, OpponentID: in.OpponentID,
		Sport: in.Sport, SetsToWin: in.SetsToWin, AdvantageEnabled: adv,
		PointsToWin: in.PointsToWin, SetRule: in.SetRule, FinalSet: in.FinalSet,
		SourceMatchID: in.SourceMatchID, Status: "pending",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.rematches = append(f.rematches, ch)
	return ch, nil
}

func (f *fakeScoreStore) getRematch(_ context.Context, id string) (RematchChallenge, error) {
	for _, ch := range f.rematches {
		if ch.ID == id {
			return ch, nil
		}
	}
	return RematchChallenge{}, ErrNotFound
}

func (f *fakeScoreStore) listIncomingRematches(_ context.Context, opponentID string, _, _ int) ([]RematchChallenge, int64, error) {
	out := []RematchChallenge{}
	for _, ch := range f.rematches {
		if ch.OpponentID == opponentID && ch.Status == "pending" {
			out = append(out, ch)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeScoreStore) listOutgoingRematches(_ context.Context, challengerID string, _, _ int) ([]RematchChallenge, int64, error) {
	out := []RematchChallenge{}
	for _, ch := range f.rematches {
		if ch.ChallengerID == challengerID && ch.Status == "pending" {
			out = append(out, ch)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeScoreStore) updateRematchStatus(_ context.Context, id, fromStatus, toStatus string) (RematchChallenge, error) {
	for i, ch := range f.rematches {
		if ch.ID == id {
			if ch.Status != fromStatus {
				return RematchChallenge{}, ErrWrongState
			}
			ch.Status = toStatus
			ch.UpdatedAt = time.Now()
			f.rematches[i] = ch
			return ch, nil
		}
	}
	return RematchChallenge{}, ErrNotFound
}

func (f *fakeScoreStore) expirePendingRematches(_ context.Context, olderThan time.Time) error {
	for i, ch := range f.rematches {
		if ch.Status == "pending" && !ch.CreatedAt.IsZero() && ch.CreatedAt.Before(olderThan) {
			ch.Status = "expired"
			f.rematches[i] = ch
		}
	}
	return nil
}

func (f *fakeScoreStore) userSummary(_ context.Context, id string) (UserSummary, error) {
	if f.summaries != nil {
		if u, ok := f.summaries[id]; ok {
			return u, nil
		}
	}
	return UserSummary{ID: id}, nil
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

func TestGetMatchPointStats_ProParticipant(t *testing.T) {
	dur := 40
	store := &fakeScoreStore{match: Match{
		ID: "m1", CreatedBy: "u1",
		PointLog: []PointEvent{
			{Side: "A", Outcome: "ace", OutcomeSide: "A"},
			{Side: "B"},
		},
		DurationSeconds: &dur,
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{isPro: true})

	got, err := svc.GetMatchPointStats(context.Background(), "u1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AcesA != 1 || got.TaggedPoints != 1 || got.TotalPoints != 2 || got.DurationSeconds != 40 {
		t.Fatalf("%+v", got)
	}
}

func TestGetMatchPointStats_FreeUserEvenWithinQuota(t *testing.T) {
	store := &fakeScoreStore{match: Match{ID: "m1", CreatedBy: "u1"}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{isPro: false})

	_, err := svc.GetMatchPointStats(context.Background(), "u1", "m1")
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetMatchPointStats_StrangerForbidden(t *testing.T) {
	store := &fakeScoreStore{match: Match{ID: "m1", CreatedBy: "u1"}}
	svc := newScoreService(store, &fakeFriends{ok: false}, nil)
	svc.SetGate(&fakeGate{isPro: true})

	_, err := svc.GetMatchPointStats(context.Background(), "stranger", "m1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetMatchPointStats_Missing(t *testing.T) {
	store := &fakeScoreStore{getErr: ErrNotFound}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{isPro: true})

	_, err := svc.GetMatchPointStats(context.Background(), "u1", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
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

func TestGetHeadToHead_UsesAllowHeadToHeadGate(t *testing.T) {
	me, opp := "u1", "u2"
	store := &fakeScoreStore{
		matches: []Match{{
			Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp,
			PlayerAName: "Me", PlayerBName: "Opp", SetsA: 2, SetsB: 0, WinnerSide: "A",
			PlayedAt: time.Now(),
		}},
	}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	// Simulates 10th free cloud match: AllowStats/AllowCloudSync locked (err=ErrProRequired),
	// while AllowHeadToHead remains open (hasH2H=true, h2hErr=nil).
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired, hasH2H: true, h2hErr: nil})
	if _, err := svc.GetUserStats(context.Background(), me, me, ""); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("stats should remain locked at 10: %v", err)
	}
	got, err := svc.GetHeadToHead(context.Background(), me, opp, "")
	if err != nil || got.Wins != 1 || got.Losses != 0 {
		t.Fatalf("10th match h2h should succeed with new match: got=%+v err=%v", got, err)
	}

	// Simulates >10 matches without Pro: AllowHeadToHead returns ErrProRequired.
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired, hasH2H: true, h2hErr: entitlement.ErrProRequired})
	if _, err := svc.GetHeadToHead(context.Background(), me, opp, ""); !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("11+ without Pro should lock h2h: %v", err)
	}
}

type fakeGate struct {
	err    error
	h2hErr error
	isPro  bool
	hasH2H bool
}

func (g *fakeGate) AllowCloudSync(context.Context, string) error { return g.err }
func (g *fakeGate) AllowStats(context.Context, string) error     { return g.err }
func (g *fakeGate) AllowHeadToHead(context.Context, string) error {
	if g.hasH2H {
		return g.h2hErr
	}
	return g.err
}
func (g *fakeGate) AllowAdvancedStats(context.Context, string) error {
	if g.isPro {
		return nil
	}
	if g.err != nil {
		return g.err
	}
	return entitlement.ErrProRequired
}
func (g *fakeGate) AllowPublicLive(context.Context, string) error { return g.err }
func (g *fakeGate) AllowLargeTournament(context.Context, string, int) error {
	return g.err
}
func (g *fakeGate) AllowLargeLeague(context.Context, string, int) error {
	return g.err
}
func (g *fakeGate) AllowSeasonArchive(context.Context, string) error { return g.err }
func (g *fakeGate) AllowEquipment(context.Context, string) error {
	if g.isPro {
		return nil
	}
	if g.err != nil {
		return g.err
	}
	return entitlement.ErrProRequired
}
func (g *fakeGate) MaxParticipants(context.Context, string) (int, error) {
	return 8, nil
}
func (g *fakeGate) IsPro(context.Context, string) (bool, error) { return g.isPro, nil }

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

func TestCreateMatch_PickleballAcceptedAndRated(t *testing.T) {
	a, b := "ua", "ub"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{}, nil)
	_, err := svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "PICKLEBALL", PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: &a, PlayerBUserID: &b,
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	})
	if err != nil {
		t.Fatalf("pickleball should be accepted: %v", err)
	}
	if store.ratings[ratingKey(a, "PICKLEBALL")].rating != 1516 ||
		store.ratings[ratingKey(b, "PICKLEBALL")].rating != 1484 {
		t.Fatalf("ratings=%v", store.ratings)
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

func TestCreateMatch_RatesRegisteredDoublesWithoutSingles(t *testing.T) {
	a, b, c, d := "ua", "ub", "uc", "ud"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{}, nil)
	if _, err := svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "PADEL", PlayerAName: "A", PlayerBName: "C",
		PlayerAUserID: &a, PlayerBUserID: &c,
		PlayerA2Name: spName("B"), PlayerB2Name: spName("D"),
		PlayerA2UserID: &b, PlayerB2UserID: &d,
		SetsA: 2, SetsB: 1, WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if store.ratings[ratingKey(a, "PADEL")].rating != 0 {
		t.Fatal("doubles moved singles rating")
	}
	if store.ratings[doublesRatingKey(a, "PADEL")].rating != 1516 || store.ratings[doublesRatingKey(c, "PADEL")].rating != 1484 {
		t.Fatalf("doubles=%v", store.ratings)
	}
	if err := svc.RateDoublesFixtures(context.Background(), "PADEL", []DoublesFixture{{
		TournamentMatchID: "tm1",
		SideA:             [2]string{a, b},
		SideB:             [2]string{c, d},
		AWon:              false,
	}}); err != nil {
		t.Fatal(err)
	}
	after := store.ratings[doublesRatingKey(a, "PADEL")].rating
	if err := svc.RateDoublesFixtures(context.Background(), "PADEL", []DoublesFixture{{
		TournamentMatchID: "tm1",
		SideA:             [2]string{a, b},
		SideB:             [2]string{c, d},
		AWon:              true,
	}}); err != nil {
		t.Fatal(err)
	}
	if store.ratings[doublesRatingKey(a, "PADEL")].rating != after {
		t.Fatal("replayed social round moved rating again")
	}
}

func spName(s string) *string { return &s }

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
		if s.Sport == "TENNIS" && s.DoublesRating != initialRating {
			t.Fatalf("doubles default=%d", s.DoublesRating)
		}
	}
	if got.DoublesRating != initialRating {
		t.Fatalf("doubles=%d", got.DoublesRating)
	}
}

func TestGetUserStats_GuestStaysAndSocialNightsAttach(t *testing.T) {
	me := "u1"
	store := &fakeScoreStore{
		matches: []Match{{
			Sport: "PADEL", PlayerAUserID: &me, PlayerAName: "Me", PlayerBName: "Guest",
			SetsA: 2, SetsB: 0, WinnerSide: "A", PlayedAt: time.Now(),
		}},
		nights: SocialNightStats{Nights: 2, BestRank: 1},
	}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err := svc.GetUserStats(context.Background(), me, me, "PADEL")
	if err != nil {
		t.Fatal(err)
	}
	if got.Matches != 1 || got.Wins != 1 || len(got.Rivals) != 0 {
		t.Fatalf("guest career=%+v", got)
	}
	if got.SocialNights != 2 || got.SocialBestRank != 1 {
		t.Fatalf("nights=%d best=%d", got.SocialNights, got.SocialBestRank)
	}
}

func TestGetHeadToHead_SocialRoundsStayOffSets(t *testing.T) {
	me, opp := "u1", "u2"
	store := &fakeScoreStore{
		matches: []Match{{
			Sport: "PADEL", PlayerAUserID: &me, PlayerBUserID: &opp,
			PlayerAName: "Me", PlayerBName: "Opp", SetsA: 2, SetsB: 1, WinnerSide: "A",
			PlayedAt: time.Now(),
		}},
		socialH2H: SocialH2H{Rounds: 4, Wins: 3, Losses: 1},
	}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	got, err := svc.GetHeadToHead(context.Background(), me, opp, "PADEL")
	if err != nil {
		t.Fatal(err)
	}
	if got.Matches != 1 || got.SetDiff != 1 {
		t.Fatalf("sets moved: %+v", got)
	}
	if got.SocialRounds != 4 || got.SocialWins != 3 || got.SocialLosses != 1 {
		t.Fatalf("social=%d %d %d", got.SocialRounds, got.SocialWins, got.SocialLosses)
	}
}

func TestCreateMatch_PersistsAndClampsFitness(t *testing.T) {
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	src := "wear_os"
	samples := []FitnessSample{{T: 30, HR: intPtr(142), Kcal: intPtr(12)}}
	_, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		AvgHeartRate: intPtr(138), MaxHeartRate: intPtr(176), ActiveCalories: intPtr(84),
		FitnessSource: &src, FitnessSamples: samples,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := store.match
	if m.AvgHeartRate == nil || *m.AvgHeartRate != 138 || m.MaxHeartRate == nil || *m.MaxHeartRate != 176 {
		t.Fatalf("hr summary=%+v", m)
	}
	if m.ActiveCalories == nil || *m.ActiveCalories != 84 || m.FitnessSource == nil || *m.FitnessSource != "wear_os" {
		t.Fatalf("kcal/source=%+v", m)
	}
	if len(m.FitnessSamples) != 1 || m.FitnessSamples[0].T != 30 {
		t.Fatalf("samples=%+v", m.FitnessSamples)
	}

	_, err = svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		AvgHeartRate: intPtr(-1), MaxHeartRate: intPtr(400), ActiveCalories: intPtr(-8),
		FitnessSource:  strPtr("fitbit"),
		FitnessSamples: append(samples, make([]FitnessSample, maxFitnessSamples)...),
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.match.AvgHeartRate != nil || store.match.MaxHeartRate != nil || store.match.ActiveCalories != nil || store.match.FitnessSource != nil {
		t.Fatalf("invalid fitness should drop: %+v", store.match)
	}
	if len(store.match.FitnessSamples) > maxFitnessSamples {
		t.Fatalf("samples not capped: %d", len(store.match.FitnessSamples))
	}
}

func TestListMyMatches_StripsFitnessSamplesUnlessPro(t *testing.T) {
	src := "wear_os"
	match := Match{
		ID: "m1", CreatedBy: "u1",
		AvgHeartRate: intPtr(140), ActiveCalories: intPtr(50),
		FitnessSource:  &src,
		FitnessSamples: []FitnessSample{{T: 10, HR: intPtr(138)}},
	}
	store := &fakeScoreStore{matches: []Match{match}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{isPro: false})
	got, _, err := svc.ListMyMatches(context.Background(), "u1", ListFilter{}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AvgHeartRate == nil || got[0].FitnessSamples != nil {
		t.Fatalf("free list should keep summary and strip samples: %+v", got)
	}

	// listForUser returns the fake's backing slice; restore samples for the Pro check.
	store.matches[0].FitnessSamples = []FitnessSample{{T: 10, HR: intPtr(138)}}
	svc.SetGate(&fakeGate{isPro: true})
	got, _, err = svc.ListMyMatches(context.Background(), "u1", ListFilter{}, 20, 0)
	if err != nil || len(got) != 1 || len(got[0].FitnessSamples) != 1 {
		t.Fatalf("pro list should keep samples: %+v err=%v", got, err)
	}
}

func TestGetMatch_StripsFitnessSamplesUnlessPro(t *testing.T) {
	src := "apple_watch"
	store := &fakeScoreStore{match: Match{
		ID: "m1", CreatedBy: "u1",
		AvgHeartRate: intPtr(140), MaxHeartRate: intPtr(180), ActiveCalories: intPtr(70),
		FitnessSource:  &src,
		FitnessSamples: []FitnessSample{{T: 10, HR: intPtr(138)}},
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{isPro: false})
	got, err := svc.GetMatch(context.Background(), "u1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.AvgHeartRate == nil || *got.AvgHeartRate != 140 || got.ActiveCalories == nil {
		t.Fatalf("summary should stay free: %+v", got)
	}
	if got.FitnessSamples != nil {
		t.Fatalf("free viewer should not see samples: %+v", got.FitnessSamples)
	}

	svc.SetGate(&fakeGate{isPro: true})
	got, err = svc.GetMatch(context.Background(), "u1", "m1")
	if err != nil || len(got.FitnessSamples) != 1 {
		t.Fatalf("pro should keep samples: %+v err=%v", got.FitnessSamples, err)
	}
}

func TestGetFitnessTrend_GatesAndFilters(t *testing.T) {
	me, opp := "u1", "u2"
	store := &fakeScoreStore{matches: []Match{
		{ID: "m1", Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp, PlayedAt: time.Now(), AvgHeartRate: intPtr(130), ActiveCalories: intPtr(40)},
		{ID: "m2", Sport: "TENNIS", PlayerAUserID: &me, PlayerBUserID: &opp, PlayedAt: time.Now()},
	}}
	svc := newScoreService(store, &fakeFriends{ok: false}, nil)
	_, err := svc.GetFitnessTrend(context.Background(), "stranger", me, "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-friend err=%v", err)
	}

	svc = newScoreService(store, &fakeFriends{ok: true}, nil)
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired})
	_, err = svc.GetFitnessTrend(context.Background(), me, me, "")
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("free err=%v", err)
	}

	svc.SetGate(&fakeGate{isPro: true})
	got, err := svc.GetFitnessTrend(context.Background(), me, me, "TENNIS")
	if err != nil || len(got) != 1 || got[0].MatchID != "m1" {
		t.Fatalf("trend=%+v err=%v", got, err)
	}
}

func TestEndLiveMatch_PersistsFitness(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{ID: "live-1", CreatedBy: "u1", Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B"}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	src := "apple_watch"
	_, err := svc.EndLiveMatch(context.Background(), "u1", "live-1", LiveEndInput{
		WinnerSide: "A", AvgHeartRate: intPtr(125), MaxHeartRate: intPtr(168),
		ActiveCalories: intPtr(55), FitnessSource: &src,
		FitnessSamples: []FitnessSample{{T: 45, HR: intPtr(125)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.match.AvgHeartRate == nil || *store.match.AvgHeartRate != 125 || store.match.FitnessSource == nil {
		t.Fatalf("history fitness=%+v", store.match)
	}
}

type recActivitySink struct {
	recorded []Match
	deleted  []string
}

func (r *recActivitySink) OnMatchRecorded(_ context.Context, m Match) {
	r.recorded = append(r.recorded, m)
}
func (r *recActivitySink) OnMatchDeleted(_ context.Context, matchID string) {
	r.deleted = append(r.deleted, matchID)
}

func TestActivitySink_CreateEndAndDelete(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "TENNIS", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B",
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	sink := &recActivitySink{}
	svc.SetActivitySink(sink)

	if _, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.recorded) != 1 || sink.recorded[0].ID != "hist-1" {
		t.Fatalf("create recorded=%+v", sink.recorded)
	}

	if _, err := svc.EndLiveMatch(context.Background(), "u1", "live-1", LiveEndInput{
		SetsA: 2, SetsB: 0, WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.recorded) != 2 {
		t.Fatalf("end recorded=%d", len(sink.recorded))
	}

	if err := svc.DeleteMatch(context.Background(), "u1", "hist-1"); err != nil {
		t.Fatal(err)
	}
	if len(sink.deleted) != 1 || sink.deleted[0] != "hist-1" {
		t.Fatalf("deleted=%v", sink.deleted)
	}
}

type recAccruer struct {
	userIDs []string
	seconds []int
}

func (r *recAccruer) AccrueMatchDuration(_ context.Context, userID string, seconds int) {
	r.userIDs = append(r.userIDs, userID)
	r.seconds = append(r.seconds, seconds)
}

func TestUsageAccruer_CreateAndEndSkipNil(t *testing.T) {
	store := &fakeScoreStore{live: LiveMatch{
		ID: "live-1", Sport: "TENNIS", CreatedBy: "u1",
		PlayerAName: "A", PlayerBName: "B",
	}}
	svc := newScoreService(store, &fakeFriends{ok: true}, nil)
	acc := &recAccruer{}
	svc.SetAccruer(acc)

	if _, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if len(acc.seconds) != 0 {
		t.Fatalf("nil duration should skip: %v", acc.seconds)
	}

	zero := 0
	if _, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		DurationSeconds: &zero,
	}); err != nil {
		t.Fatal(err)
	}
	if len(acc.seconds) != 0 {
		t.Fatalf("zero duration should skip: %v", acc.seconds)
	}

	dur := 90
	if _, err := svc.CreateMatch(context.Background(), "u1", CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B", WinnerSide: "A",
		DurationSeconds: &dur,
	}); err != nil {
		t.Fatal(err)
	}
	if len(acc.seconds) != 1 || acc.seconds[0] != 90 || acc.userIDs[0] != "u1" {
		t.Fatalf("create accrue=%v users=%v", acc.seconds, acc.userIDs)
	}

	endDur := 40
	if _, err := svc.EndLiveMatch(context.Background(), "u1", "live-1", LiveEndInput{
		SetsA: 2, SetsB: 0, WinnerSide: "A", DurationSeconds: &endDur,
	}); err != nil {
		t.Fatal(err)
	}
	if len(acc.seconds) != 2 || acc.seconds[1] != 40 {
		t.Fatalf("end accrue=%v", acc.seconds)
	}
}
