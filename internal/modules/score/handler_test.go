package score

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/entitlement"
	"courtscore/internal/platform/httpx"
)

// ── stub service ─────────────────────────────────────────────────────────────

type stubService struct {
	match      Match
	live       LiveMatch
	matches    []Match
	total      int64
	stats      UserStats
	h2h        HeadToHead
	trend         []FitnessTrendPoint
	wrapped       WrappedReport
	wrappedErr    error
	wrappedShare  WrappedShare
	publicWrapped WrappedReport
	gotYear       int
	lastCreate    CreateInput
	lastEnd       LiveEndInput
	createErr     error
	listErr       error
	getErr        error
	pointStats    MatchPointStats
	pointStatsErr error
	deleteErr     error
	liveErr       error
	statsErr      error
	h2hErr        error
	trendErr      error
	rematch    RematchChallenge
	prefill    RematchPrefill
	rematchErr error

	gotLimit  int
	gotOffset int
}

func (s *stubService) CreateMatch(_ context.Context, userID string, in CreateInput) (Match, error) {
	if s.createErr != nil {
		return Match{}, s.createErr
	}
	s.lastCreate = in
	m := s.match
	m.CreatedBy = userID
	m.Sport = in.Sport
	m.AvgHeartRate = in.AvgHeartRate
	m.MaxHeartRate = in.MaxHeartRate
	m.ActiveCalories = in.ActiveCalories
	m.FitnessSource = in.FitnessSource
	m.FitnessSamples = in.FitnessSamples
	return m, nil
}

func (s *stubService) ListMyMatches(_ context.Context, _ string, _ ListFilter, limit, offset int) ([]Match, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	return s.matches, s.total, s.listErr
}

func (s *stubService) ListUserMatches(_ context.Context, _, _ string, _ ListFilter, limit, offset int) ([]Match, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	return s.matches, s.total, s.listErr
}

func (s *stubService) GetMatch(_ context.Context, _, _ string) (Match, error) {
	return s.match, s.getErr
}

func (s *stubService) GetMatchPointStats(_ context.Context, _, _ string) (MatchPointStats, error) {
	if s.pointStatsErr != nil {
		return MatchPointStats{}, s.pointStatsErr
	}
	return s.pointStats, nil
}

func (s *stubService) DeleteMatch(_ context.Context, _, _ string) error { return s.deleteErr }

func (s *stubService) StartLiveMatch(_ context.Context, userID string, in LiveStartInput) (LiveMatch, error) {
	if s.liveErr != nil {
		return LiveMatch{}, s.liveErr
	}
	m := s.live
	m.CreatedBy = userID
	m.Sport = in.Sport
	if in.Status != "" {
		m.Status = in.Status
	} else if in.PlayerAUserID != nil || in.PlayerBUserID != nil {
		m.Status = "PENDING"
	} else {
		m.Status = "IN_PROGRESS"
	}
	return m, nil
}

func (s *stubService) GetLiveMatch(_ context.Context, _, _ string) (LiveMatch, error) {
	return s.live, s.liveErr
}

func (s *stubService) UpdateLiveMatch(_ context.Context, _, _ string, _ LiveScoreUpdate) (LiveMatch, error) {
	return s.live, s.liveErr
}

func (s *stubService) EndLiveMatch(_ context.Context, _, _ string, in LiveEndInput) (LiveMatch, error) {
	s.lastEnd = in
	return s.live, s.liveErr
}

func (s *stubService) GetFitnessTrend(_ context.Context, _, _, _ string) ([]FitnessTrendPoint, error) {
	if s.trendErr != nil {
		return nil, s.trendErr
	}
	return s.trend, nil
}

func (s *stubService) ListMyOpenLiveMatches(_ context.Context, _ string, limit, offset int) ([]LiveMatch, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	if s.liveErr != nil {
		return nil, 0, s.liveErr
	}
	if s.live.ID == "" {
		return nil, 0, nil
	}
	return []LiveMatch{s.live}, 1, nil
}

func (s *stubService) CancelLiveMatch(_ context.Context, _, _ string) (LiveMatch, error) {
	if s.liveErr != nil {
		return LiveMatch{}, s.liveErr
	}
	m := s.live
	m.Status = "ENDED"
	return m, nil
}

func (s *stubService) AcceptLiveMatch(_ context.Context, _, _ string) (LiveMatch, error) {
	if s.liveErr != nil {
		return LiveMatch{}, s.liveErr
	}
	m := s.live
	m.Status = "IN_PROGRESS"
	return m, nil
}

func (s *stubService) DeclineLiveMatch(_ context.Context, _, _ string) (LiveMatch, error) {
	if s.liveErr != nil {
		return LiveMatch{}, s.liveErr
	}
	m := s.live
	m.Status = "ENDED"
	return m, nil
}

func (s *stubService) GetUserStats(_ context.Context, _, _, _ string) (UserStats, error) {
	if s.statsErr != nil {
		return UserStats{}, s.statsErr
	}
	return s.stats, nil
}

func (s *stubService) GetHeadToHead(_ context.Context, _, _, _ string) (HeadToHead, error) {
	if s.h2hErr != nil {
		return HeadToHead{}, s.h2hErr
	}
	return s.h2h, nil
}

func (s *stubService) ShareWrapped(_ context.Context, _ string, year int) (WrappedShare, error) {
	s.gotYear = year
	if s.wrappedErr != nil {
		return WrappedShare{}, s.wrappedErr
	}
	if s.wrappedShare.Token == "" {
		s.wrappedShare = WrappedShare{Token: "tok", URL: "https://court-score.com/wrapped/tok", Year: year}
	}
	return s.wrappedShare, nil
}

func (s *stubService) GetPublicWrapped(_ context.Context, token string) (WrappedReport, error) {
	if s.wrappedErr != nil {
		return WrappedReport{}, s.wrappedErr
	}
	if s.publicWrapped.Year == 0 {
		return WrappedReport{}, ErrNotFound
	}
	_ = token
	return s.publicWrapped, nil
}

func (s *stubService) GetWrapped(_ context.Context, _ string, year int) (WrappedReport, error) {
	s.gotYear = year
	if s.wrappedErr != nil {
		return WrappedReport{}, s.wrappedErr
	}
	if s.wrapped.Year == 0 {
		s.wrapped.Year = year
	}
	return s.wrapped, nil
}

func (s *stubService) ShareLiveMatch(_ context.Context, _, _ string) (string, string, error) {
	if s.liveErr != nil {
		return "", "", s.liveErr
	}
	return "tok", "https://court-score.com/live/s/tok", nil
}

func (s *stubService) GetLiveByShareToken(_ context.Context, _ string) (LiveMatch, error) {
	return s.live, s.liveErr
}

func (s *stubService) CreateRematch(_ context.Context, userID string, in RematchCreateInput) (RematchChallenge, error) {
	if s.rematchErr != nil {
		return RematchChallenge{}, s.rematchErr
	}
	ch := s.rematch
	if ch.ID == "" {
		ch.ID = "rm-1"
	}
	ch.ChallengerID = userID
	ch.OpponentID = in.OpponentID
	ch.Sport = in.Sport
	ch.Status = "pending"
	return ch, nil
}

func (s *stubService) ListIncomingRematches(_ context.Context, _ string, limit, offset int) ([]RematchChallenge, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	if s.rematchErr != nil {
		return nil, 0, s.rematchErr
	}
	if s.rematch.ID == "" {
		return nil, 0, nil
	}
	return []RematchChallenge{s.rematch}, 1, nil
}

func (s *stubService) ListOutgoingRematches(_ context.Context, _ string, limit, offset int) ([]RematchChallenge, int64, error) {
	s.gotLimit, s.gotOffset = limit, offset
	if s.rematchErr != nil {
		return nil, 0, s.rematchErr
	}
	if s.rematch.ID == "" {
		return nil, 0, nil
	}
	return []RematchChallenge{s.rematch}, 1, nil
}

func (s *stubService) AcceptRematch(_ context.Context, _, _ string) (RematchPrefill, error) {
	if s.rematchErr != nil {
		return RematchPrefill{}, s.rematchErr
	}
	return s.prefill, nil
}

func (s *stubService) RejectRematch(_ context.Context, _, _ string) (RematchChallenge, error) {
	if s.rematchErr != nil {
		return RematchChallenge{}, s.rematchErr
	}
	ch := s.rematch
	ch.Status = "rejected"
	return ch, nil
}

func (s *stubService) CancelRematch(_ context.Context, _, _ string) (RematchChallenge, error) {
	if s.rematchErr != nil {
		return RematchChallenge{}, s.rematchErr
	}
	ch := s.rematch
	ch.Status = "cancelled"
	return ch, nil
}

func authedRequest(method, target string, body []byte, userID string) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	ctx := context.WithValue(r.Context(), authkit.UserIDKey, userID)
	return r.WithContext(ctx)
}

func baseMatch() Match {
	return Match{
		ID: "match-1", Sport: "TENNIS",
		PlayerAName: "Ada", PlayerBName: "Grace",
		SetsA: 2, SetsB: 1, WinnerSide: "A", Winner: "Ada",
		CreatedBy: "user-abc", PlayedAt: time.Now(), CreatedAt: time.Now(),
	}
}

// ── POST /v1/matches ─────────────────────────────────────────────────────────

func TestCreateMatch_Returns201(t *testing.T) {
	svc := &stubService{match: baseMatch()}
	h := &handler{svc: svc}

	body, _ := json.Marshal(map[string]any{
		"sport": "TENNIS", "player_a_name": "Ada", "player_b_name": "Grace",
		"sets_a": 2, "sets_b": 1, "winner_side": "A",
	})
	w := httptest.NewRecorder()
	h.createMatch(w, authedRequest(http.MethodPost, "/v1/matches", body, "user-abc"))

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
	var got Match
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CreatedBy != "user-abc" {
		t.Errorf("created_by: got %q, want user-abc", got.CreatedBy)
	}
}

func TestCreateMatch_Returns400OnInvalidInput(t *testing.T) {
	svc := &stubService{createErr: ErrInvalid}
	h := &handler{svc: svc}

	body, _ := json.Marshal(map[string]any{"sport": "CHESS"})
	w := httptest.NewRecorder()
	h.createMatch(w, authedRequest(http.MethodPost, "/v1/matches", body, "user-abc"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestCreateMatch_Returns402OnProRequired(t *testing.T) {
	svc := &stubService{createErr: entitlement.ErrProRequired}
	h := &handler{svc: svc}
	body, _ := json.Marshal(map[string]any{
		"sport": "TENNIS", "player_a_name": "Ada", "player_b_name": "Grace",
		"sets_a": 2, "sets_b": 1, "winner_side": "A",
	})
	w := httptest.NewRecorder()
	h.createMatch(w, authedRequest(http.MethodPost, "/v1/matches", body, "user-abc"))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status: got %d, want 402", w.Code)
	}
}

// ── GET /v1/matches ──────────────────────────────────────────────────────────

func TestListMyMatches_ReturnsPagedEnvelope(t *testing.T) {
	svc := &stubService{matches: []Match{baseMatch()}, total: 45}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listMyMatches(w, authedRequest(http.MethodGet, "/v1/matches?page=2", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if svc.gotLimit != 20 || svc.gotOffset != 20 {
		t.Errorf("pagination: got limit=%d offset=%d, want 20/20", svc.gotLimit, svc.gotOffset)
	}
	var got httpx.Page[Match]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 45 || got.Page != 2 || got.PageSize != 20 || !got.HasMore {
		t.Errorf("envelope: %+v", got)
	}
}

// ── GET /v1/users/{userID}/matches ───────────────────────────────────────────

func TestListUserMatches_Returns403WhenNotFriends(t *testing.T) {
	svc := &stubService{listErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.listUserMatches(w, authedRequest(http.MethodGet, "/v1/users/other/matches", nil, "user-abc"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

// ── GET /v1/matches/{id} ─────────────────────────────────────────────────────

func TestGetMatch_OldMatchWithoutPointLogReturns200(t *testing.T) {
	svc := &stubService{match: baseMatch()}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatch(w, authedRequest(http.MethodGet, "/v1/matches/match-1", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var decoded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["point_log"]; ok {
		t.Fatalf("old match included point_log: %s", w.Body.String())
	}
	stats := Summarize(nil, nil)
	if stats.TotalPoints != 0 || stats.HasServer {
		t.Fatalf("detail stats: %+v", stats)
	}
}

func TestGetMatch_Returns404WhenMissing(t *testing.T) {
	svc := &stubService{getErr: ErrNotFound}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatch(w, authedRequest(http.MethodGet, "/v1/matches/nope", nil, "user-abc"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

func TestGetMatch_Returns403WhenNoAccess(t *testing.T) {
	svc := &stubService{getErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatch(w, authedRequest(http.MethodGet, "/v1/matches/private", nil, "stranger"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestGetMatchPointStats_Returns200ForPro(t *testing.T) {
	svc := &stubService{pointStats: MatchPointStats{AcesA: 2, WinnersB: 1, TaggedPoints: 3}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatchPointStats(w, authedRequest(http.MethodGet, "/v1/matches/match-1/stats", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got MatchPointStats
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AcesA != 2 || got.WinnersB != 1 || got.TaggedPoints != 3 {
		t.Fatalf("stats: %+v", got)
	}
}

func TestGetMatchPointStats_Returns402WhenNotPro(t *testing.T) {
	svc := &stubService{pointStatsErr: entitlement.ErrProRequired}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatchPointStats(w, authedRequest(http.MethodGet, "/v1/matches/match-1/stats", nil, "user-abc"))

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status: got %d, want 402", w.Code)
	}
}

func TestGetMatchPointStats_Returns403WhenNoAccess(t *testing.T) {
	svc := &stubService{pointStatsErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatchPointStats(w, authedRequest(http.MethodGet, "/v1/matches/private/stats", nil, "stranger"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestGetMatchPointStats_Returns404WhenMissing(t *testing.T) {
	svc := &stubService{pointStatsErr: ErrNotFound}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getMatchPointStats(w, authedRequest(http.MethodGet, "/v1/matches/nope/stats", nil, "user-abc"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

// ── DELETE /v1/matches/{id} ──────────────────────────────────────────────────

func TestDeleteMatch_Returns204(t *testing.T) {
	svc := &stubService{}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.deleteMatch(w, authedRequest(http.MethodDelete, "/v1/matches/match-1", nil, "user-abc"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", w.Code)
	}
}

func TestDeleteMatch_Returns404WhenNotOwnerOrMissing(t *testing.T) {
	svc := &stubService{deleteErr: ErrNotFound}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.deleteMatch(w, authedRequest(http.MethodDelete, "/v1/matches/match-1", nil, "other"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

// ── GET /v1/users/{userID}/stats ─────────────────────────────────────────────

func TestGetUserStats_Returns200(t *testing.T) {
	svc := &stubService{stats: UserStats{
		UserID: "user-abc", Matches: 2, Wins: 1, Losses: 1,
		Form: []string{"W", "L"}, Rivals: []RivalSummary{}, BySport: []SportStats{},
	}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getUserStats(w, authedRequest(http.MethodGet, "/v1/users/user-abc/stats", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got UserStats
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Matches != 2 || got.Wins != 1 || len(got.Form) != 2 {
		t.Errorf("stats: %+v", got)
	}
}

func TestGetUserStats_Returns403WhenNotFriends(t *testing.T) {
	svc := &stubService{statsErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getUserStats(w, authedRequest(http.MethodGet, "/v1/users/other/stats", nil, "user-abc"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestGetUserStats_Returns400OnBadSport(t *testing.T) {
	svc := &stubService{statsErr: ErrInvalid}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getUserStats(w, authedRequest(http.MethodGet, "/v1/users/user-abc/stats?sport=CHESS", nil, "user-abc"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

// ── GET /v1/users/{userID}/h2h ───────────────────────────────────────────────

func TestGetHeadToHead_Returns200Empty(t *testing.T) {
	svc := &stubService{h2h: HeadToHead{
		OpponentUserID: "other", Form: []string{},
	}}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getHeadToHead(w, authedRequest(http.MethodGet, "/v1/users/other/h2h", nil, "user-abc"))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var got HeadToHead
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OpponentUserID != "other" || got.Matches != 0 {
		t.Errorf("h2h: %+v", got)
	}
}

func TestGetHeadToHead_Returns400AgainstSelf(t *testing.T) {
	svc := &stubService{h2hErr: ErrInvalid}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getHeadToHead(w, authedRequest(http.MethodGet, "/v1/users/user-abc/h2h", nil, "user-abc"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
}

func TestGetHeadToHead_Returns403WhenNotFriends(t *testing.T) {
	svc := &stubService{h2hErr: ErrForbidden}
	h := &handler{svc: svc}

	w := httptest.NewRecorder()
	h.getHeadToHead(w, authedRequest(http.MethodGet, "/v1/users/stranger/h2h", nil, "user-abc"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
}

func TestCreateMatch_RoundTripsFitnessFields(t *testing.T) {
	svc := &stubService{match: baseMatch()}
	h := &handler{svc: svc}
	src := "apple_watch"
	hr := 141
	body, _ := json.Marshal(map[string]any{
		"sport": "TENNIS", "player_a_name": "Ada", "player_b_name": "Grace",
		"sets_a": 2, "sets_b": 1, "winner_side": "A",
		"avg_heart_rate": 141, "max_heart_rate": 178, "active_calories": 62,
		"fitness_source": src,
		"fitness_samples": []map[string]any{{"t": 30, "hr": 141, "kcal": 10}},
	})
	w := httptest.NewRecorder()
	h.createMatch(w, authedRequest(http.MethodPost, "/v1/matches", body, "user-abc"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if svc.lastCreate.AvgHeartRate == nil || *svc.lastCreate.AvgHeartRate != hr {
		t.Fatalf("create input=%+v", svc.lastCreate)
	}
	var got Match
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.AvgHeartRate == nil || *got.AvgHeartRate != 141 || len(got.FitnessSamples) != 1 {
		t.Fatalf("response=%+v", got)
	}
}

func TestGetFitnessTrend_HandlerStatuses(t *testing.T) {
	h := &handler{svc: &stubService{trendErr: entitlement.ErrProRequired}}
	w := httptest.NewRecorder()
	h.getFitnessTrend(w, authedRequest(http.MethodGet, "/v1/users/user-abc/fitness-trend", nil, "user-abc"))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status=%d want 402", w.Code)
	}

	h = &handler{svc: &stubService{trendErr: ErrForbidden}}
	w = httptest.NewRecorder()
	h.getFitnessTrend(w, authedRequest(http.MethodGet, "/v1/users/other/fitness-trend", nil, "user-abc"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", w.Code)
	}

	h = &handler{svc: &stubService{trend: []FitnessTrendPoint{{MatchID: "m1", AvgHeartRate: intPtr(130)}}}}
	w = httptest.NewRecorder()
	h.getFitnessTrend(w, authedRequest(http.MethodGet, "/v1/users/user-abc/fitness-trend", nil, "user-abc"))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	items, ok := decoded["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items=%v", decoded["items"])
	}
}
