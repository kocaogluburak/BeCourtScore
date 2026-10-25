package score

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"courtscore/internal/platform/entitlement"
)

func wrapMatch(id, sport, me, opp, winner string, setsA, setsB int, played time.Time, dur *int, oppName string) Match {
	return Match{
		ID: id, Sport: sport,
		PlayerAUserID: &me, PlayerBUserID: &opp,
		PlayerAName: "Me", PlayerBName: oppName,
		SetsA: setsA, SetsB: setsB, WinnerSide: winner,
		PlayedAt: played, DurationSeconds: dur,
	}
}

func TestWrappedYearWindow_Boundaries(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	from, to, err := wrappedYearWindow(2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("from=%s", from)
	}
	if !to.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("to=%s", to)
	}
	if _, _, err := wrappedYearWindow(2022, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("2022 err=%v", err)
	}
	if _, _, err := wrappedYearWindow(2027, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("future year err=%v", err)
	}
	if _, _, err := wrappedYearWindow(2023, now); err != nil {
		t.Fatalf("2023 should be allowed: %v", err)
	}
}

func TestAggregateWrapped_YearInReview(t *testing.T) {
	me, alex, sam := "me", "alex", "sam"
	y := 2026
	jan := time.Date(2026, 1, 4, 10, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC)
	sep := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	nov := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	dec := time.Date(2026, 12, 20, 9, 0, 0, 0, time.UTC)
	long := 9240
	short := 3600

	// Newest first, matching listParticipantMatchesInRange.
	matches := []Match{
		wrapMatch("m6", "SQUASH", me, sam, "A", 2, 0, dec, &short, "Sam"),
		wrapMatch("m5", "TENNIS", me, alex, "A", 2, 1, nov, &short, "Alex"),
		wrapMatch("m4", "TENNIS", me, alex, "A", 2, 0, sep, &short, "Alex"),
		wrapMatch("m3", "PADEL", me, sam, "B", 1, 2, jul, &long, "Sam"),
		wrapMatch("m2", "TENNIS", me, alex, "A", 2, 1, jul.Add(-24*time.Hour), &short, "Alex"),
		wrapMatch("m1", "TENNIS", me, alex, "A", 2, 0, jan, &short, "Alex"),
	}
	firsts := []SportFirst{
		{Sport: "TENNIS", PlayedAt: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
		{Sport: "PADEL", PlayedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)},
		{Sport: "SQUASH", PlayedAt: dec},
	}
	peakAt := sep
	peak := &WrappedElo{Sport: "TENNIS", Rating: 1712, PlayedAt: &peakAt}
	ratings := []UserRating{{Sport: "TENNIS", Rating: 1690, MatchesCounted: 8}}

	got := aggregateWrapped(me, y, matches, firsts, ratings, peak, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))

	if !got.HasEnoughData || got.TotalMatches != 6 || got.Wins != 5 || got.Losses != 1 {
		t.Fatalf("totals=%+v", got)
	}
	if got.WinRate < 0.83 || got.WinRate > 0.84 {
		t.Fatalf("win_rate=%v", got.WinRate)
	}
	if got.FavoriteSport != "TENNIS" || got.SportsPlayedCount != 3 {
		t.Fatalf("favorite=%s sports=%d by=%+v", got.FavoriteSport, got.SportsPlayedCount, got.BySport)
	}
	if got.BiggestRival == nil || got.BiggestRival.UserID != alex || got.BiggestRival.Matches != 4 {
		t.Fatalf("rival=%+v", got.BiggestRival)
	}
	if got.LongestMatch == nil || got.LongestMatch.MatchID != "m3" || got.LongestMatch.DurationSeconds != 9240 {
		t.Fatalf("longest=%+v", got.LongestMatch)
	}
	if got.LongestMatch.OpponentName != "Sam" || got.LongestMatch.Sport != "PADEL" {
		t.Fatalf("longest meta=%+v", got.LongestMatch)
	}
	if got.BusiestMonth == nil || got.BusiestMonth.Month != 7 || got.BusiestMonth.Matches != 2 {
		t.Fatalf("busiest=%+v monthly=%v", got.BusiestMonth, got.MonthlyMatches)
	}
	if len(got.MonthlyMatches) != 12 || got.MonthlyMatches[0] != 1 || got.MonthlyMatches[10] != 1 {
		t.Fatalf("monthly=%v", got.MonthlyMatches)
	}
	if got.LongestWinStreak != 3 { // m1,m2 then L then W,W,W — wait: chrono m1W m2W m3L m4W m5W m6W → longest 3
		t.Fatalf("longest streak=%d", got.LongestWinStreak)
	}
	if got.CurrentStreak == nil || got.CurrentStreak.Type != "W" || got.CurrentStreak.Length != 3 {
		t.Fatalf("current=%+v", got.CurrentStreak)
	}
	if got.SetsFor != (2+2+1+2+2+2) || got.SetsAgainst != (0+1+2+0+1+0) {
		t.Fatalf("sets %d-%d", got.SetsFor, got.SetsAgainst)
	}
	if len(got.NewSportsTried) != 1 || got.NewSportsTried[0] != "SQUASH" {
		t.Fatalf("new sports=%v", got.NewSportsTried)
	}
	if got.FirstMatchOfYear == nil || got.FirstMatchOfYear.OpponentName != "Alex" {
		t.Fatalf("first=%+v", got.FirstMatchOfYear)
	}
	if got.EloPeak == nil || got.EloPeak.Rating != 1712 {
		t.Fatalf("peak=%+v", got.EloPeak)
	}
	if got.EloYearEnd == nil || got.EloYearEnd.Sport != "TENNIS" || got.EloYearEnd.Rating != 1690 {
		t.Fatalf("year-end=%+v", got.EloYearEnd)
	}
}

func TestAggregateWrapped_NotEnoughDataAndEmpty(t *testing.T) {
	me, opp := "me", "opp"
	jan := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	empty := aggregateWrapped(me, 2026, nil, nil, nil, nil, jan)
	if empty.HasEnoughData || empty.TotalMatches != 0 || empty.WinRate != 0 {
		t.Fatalf("empty=%+v", empty)
	}
	if empty.BiggestRival != nil || empty.LongestMatch != nil || empty.EloPeak != nil {
		t.Fatalf("empty extras=%+v", empty)
	}
	if len(empty.MonthlyMatches) != 12 || empty.NewSportsTried == nil {
		t.Fatalf("slices should be non-nil")
	}

	var few []Match
	for i := 0; i < 4; i++ {
		few = append(few, wrapMatch("m", "TENNIS", me, opp, "A", 2, 0, jan.Add(time.Duration(i)*time.Hour), nil, "Opp"))
	}
	got := aggregateWrapped(me, 2026, few, nil, nil, nil, jan)
	if got.HasEnoughData || got.TotalMatches != 4 {
		t.Fatalf("threshold=%+v", got)
	}
}

func TestAggregateWrapped_YearBoundaryExclusion(t *testing.T) {
	me, opp := "me", "opp"
	// Aggregation assumes the repo already filtered; this checks histogram uses UTC month
	// and that a Dec 31 UTC match is month 12.
	dec31 := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	got := aggregateWrapped(me, 2026, []Match{
		wrapMatch("m", "TENNIS", me, opp, "A", 2, 0, dec31, nil, "Opp"),
	}, nil, nil, nil, dec31)
	if got.MonthlyMatches[11] != 1 || got.BusiestMonth == nil || got.BusiestMonth.Month != 12 {
		t.Fatalf("dec31 month=%v busiest=%+v", got.MonthlyMatches, got.BusiestMonth)
	}
}

func TestAggregateWrapped_DoublesDurationAndRival(t *testing.T) {
	me, partner, opp1, opp2 := "me", "p", "o1", "o2"
	partnerName := "Pat"
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	dur := 5000
	m := Match{
		ID: "d1", Sport: "PADEL",
		PlayerAUserID: &me, PlayerA2UserID: &partner, PlayerA2Name: &partnerName,
		PlayerBUserID: &opp1, PlayerB2UserID: &opp2,
		PlayerAName: "Me", PlayerBName: "O1", PlayerB2Name: func() *string { s := "O2"; return &s }(),
		SetsA: 2, SetsB: 1, WinnerSide: "A",
		PlayedAt: jul, DurationSeconds: &dur,
	}
	got := aggregateWrapped(me, 2026, []Match{m}, nil, nil, nil, jul)
	if got.LongestMatch == nil || got.LongestMatch.DurationSeconds != 5000 {
		t.Fatalf("doubles duration=%+v", got.LongestMatch)
	}
	if got.BiggestRival == nil || got.BiggestRival.UserID != opp1 {
		t.Fatalf("doubles rival=%+v", got.BiggestRival)
	}
	if got.LongestMatch.OpponentName != "O1 / O2" {
		t.Fatalf("opp name=%q", got.LongestMatch.OpponentName)
	}
}

func TestAggregateWrapped_RecorderOnlyIgnored(t *testing.T) {
	me := "me"
	jan := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	got := aggregateWrapped(me, 2026, []Match{{
		ID: "rec", Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B",
		WinnerSide: "A", CreatedBy: me, SetsA: 2, SetsB: 0, PlayedAt: jan,
	}}, nil, nil, nil, jan)
	if got.TotalMatches != 0 || got.FirstMatchOfYear != nil {
		t.Fatalf("recorder counted: %+v", got)
	}
}

func TestAggregateWrapped_EloYearEndFallbackWhenPeakNil(t *testing.T) {
	me, opp := "me", "opp"
	jan := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	matches := []Match{wrapMatch("m", "TENNIS", me, opp, "A", 2, 0, jan, nil, "Opp")}
	got := aggregateWrapped(me, 2026, matches, nil, []UserRating{{Sport: "TENNIS", Rating: 1540}}, nil, jan)
	if got.EloPeak != nil {
		t.Fatalf("peak should be nil: %+v", got.EloPeak)
	}
	if got.EloYearEnd == nil || got.EloYearEnd.Rating != 1540 {
		t.Fatalf("year-end=%+v", got.EloYearEnd)
	}
}

func TestPickEloPeak_MaxInWindow(t *testing.T) {
	t1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got := pickEloPeak([]WrappedElo{
		{Sport: "TENNIS", Rating: 1600, PlayedAt: &t1},
		{Sport: "PADEL", Rating: 1712, PlayedAt: &t2},
		{Sport: "TENNIS", Rating: 1680, PlayedAt: &t2},
	})
	if got == nil || got.Rating != 1712 || got.Sport != "PADEL" {
		t.Fatalf("peak=%+v", got)
	}
	if pickEloPeak(nil) != nil {
		t.Fatal("empty peak")
	}
}

func TestGetMyWrapped_Handler(t *testing.T) {
	svc := &stubService{wrapped: WrappedReport{
		UserID: "user-abc", Year: 2026, HasEnoughData: true, TotalMatches: 8,
		MonthlyMatches: []int{1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0},
		NewSportsTried: []string{},
		BySport:        []SportStats{},
	}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMyWrapped(w, authedRequest(http.MethodGet, "/v1/me/wrapped?year=2026", nil, "user-abc"))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got WrappedReport
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TotalMatches != 8 || svc.gotYear != 2026 {
		t.Fatalf("got=%+v year=%d", got, svc.gotYear)
	}

	w = httptest.NewRecorder()
	h.getMyWrapped(w, authedRequest(http.MethodGet, "/v1/me/wrapped", nil, "user-abc"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing year status=%d", w.Code)
	}

	w = httptest.NewRecorder()
	h.getMyWrapped(w, authedRequest(http.MethodGet, "/v1/me/wrapped?year=abc", nil, "user-abc"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid year status=%d", w.Code)
	}

	svc.wrappedErr = ErrInvalid
	w = httptest.NewRecorder()
	h.getMyWrapped(w, authedRequest(http.MethodGet, "/v1/me/wrapped?year=1999", nil, "user-abc"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("out of range status=%d", w.Code)
	}
}

func TestShareAndPublicWrapped_Handler(t *testing.T) {
	svc := &stubService{
		wrappedShare:  WrappedShare{Token: "abc", URL: "https://court-score.com/wrapped/abc", Year: 2026},
		publicWrapped: WrappedReport{UserID: "u1", Year: 2026, HasEnoughData: true, TotalMatches: 8},
	}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.shareMyWrapped(w, authedRequest(http.MethodPost, "/v1/me/wrapped/share?year=2026", nil, "user-abc"))
	if w.Code != http.StatusCreated {
		t.Fatalf("share status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"token":"abc"`) {
		t.Fatalf("body=%s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/public/wrapped/abc", nil)
	req.SetPathValue("token", "abc")
	h.getPublicWrapped(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"total_matches":8`) {
		t.Fatalf("public status=%d body=%s", w.Code, w.Body.String())
	}

	svc.wrappedErr = ErrNotFound
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/public/wrapped/missing", nil)
	req.SetPathValue("token", "missing")
	h.getPublicWrapped(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", w.Code)
	}
}

func TestGetWrapped_FreeEvenWhenStatsDenied(t *testing.T) {
	me, opp := "u1", "u2"
	var matches []Match
	for i := 0; i < 5; i++ {
		matches = append(matches, wrapMatch(
			"m"+string(rune('1'+i)), "TENNIS", me, opp, "A", 2, 0,
			time.Date(2026, 4, 1+i, 0, 0, 0, 0, time.UTC), nil, "Opp",
		))
	}
	store := &fakeScoreStore{matches: matches}
	svc := newScoreService(store, &fakeFriends{}, nil)
	svc.SetGate(&fakeGate{err: entitlement.ErrProRequired})

	got, err := svc.GetWrapped(context.Background(), me, 2026)
	if err != nil {
		t.Fatalf("wrapped should be free: %v", err)
	}
	if !got.HasEnoughData || got.TotalMatches != 5 {
		t.Fatalf("got=%+v", got)
	}

	_, err = svc.GetUserStats(context.Background(), me, me, "")
	if !errors.Is(err, entitlement.ErrProRequired) {
		t.Fatalf("stats should still be gated: %v", err)
	}

	_, err = svc.GetWrapped(context.Background(), me, 2019)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad year err=%v", err)
	}
}

func TestGetWrapped_YearRangePrunesAndPeak(t *testing.T) {
	me, opp := "u1", "u2"
	inYear := wrapMatch("in", "TENNIS", me, opp, "A", 2, 0, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), nil, "Opp")
	outYear := wrapMatch("out", "TENNIS", me, opp, "A", 2, 0, time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC), nil, "Opp")
	janNext := wrapMatch("next", "TENNIS", me, opp, "A", 2, 0, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), nil, "Opp")
	store := &fakeScoreStore{
		matches: []Match{inYear, outYear, janNext},
		history: []ratingHistoryRow{
			{UserID: me, Sport: "TENNIS", Rating: 1600, PlayedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
			{UserID: me, Sport: "TENNIS", Rating: 1720, PlayedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
			{UserID: me, Sport: "TENNIS", Rating: 1800, PlayedAt: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)},
		},
		ratings: map[string]lockedRating{ratingKey(me, "TENNIS"): {rating: 1690, counted: 4}},
	}
	svc := newScoreService(store, &fakeFriends{}, nil)
	got, err := svc.GetWrapped(context.Background(), me, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalMatches != 1 {
		t.Fatalf("year prune matches=%d", got.TotalMatches)
	}
	if got.EloPeak == nil || got.EloPeak.Rating != 1720 {
		t.Fatalf("peak=%+v", got.EloPeak)
	}
	if got.EloYearEnd == nil || got.EloYearEnd.Rating != 1690 {
		t.Fatalf("year-end=%+v", got.EloYearEnd)
	}
}

func TestApplySinglesElo_AppendsHistoryPerPlayer(t *testing.T) {
	a, b := "u-win", "u-lose"
	store := &fakeScoreStore{}
	svc := newScoreService(store, &fakeFriends{}, nil)
	played := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	_, err := svc.CreateMatch(context.Background(), a, CreateInput{
		Sport: "TENNIS", PlayerAName: "A", PlayerBName: "B",
		PlayerAUserID: &a, PlayerBUserID: &b,
		SetsA: 2, SetsB: 0, WinnerSide: "A", PlayedAt: &played,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.history) != 2 {
		t.Fatalf("history rows=%d want 2", len(store.history))
	}
	seen := map[string]int{}
	for _, h := range store.history {
		seen[h.UserID] = h.Rating
		if h.Sport != "TENNIS" || h.PlayedAt.IsZero() {
			t.Fatalf("row=%+v", h)
		}
	}
	if seen[a] != 1516 || seen[b] != 1484 {
		t.Fatalf("history ratings=%v", seen)
	}
}

