package achievements

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"courtscore/internal/platform/authkit"
)

type stubAchService struct {
	catalog []CatalogEntry
	badges  []UserBadge
	total   int64
	goal    GoalState
	err     error
}

func (s *stubAchService) Catalog() []CatalogEntry { return s.catalog }
func (s *stubAchService) ListUserBadges(context.Context, string, string, int, int) ([]UserBadge, int64, error) {
	return s.badges, s.total, s.err
}
func (s *stubAchService) GetGoal(context.Context, string) (GoalState, error) { return s.goal, s.err }
func (s *stubAchService) SetGoal(_ context.Context, _ string, weeklyTarget int, reminderOptIn bool) (GoalState, error) {
	if s.err != nil {
		return GoalState{}, s.err
	}
	s.goal.WeeklyTarget = weeklyTarget
	s.goal.ReminderOptIn = reminderOptIn
	return s.goal, nil
}

func withUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, userID))
}

func TestCatalog_ReturnsCodes(t *testing.T) {
	h := &handler{svc: &stubAchService{catalog: Catalog()}}
	r := httptest.NewRequest(http.MethodGet, "/v1/achievements/badges", nil)
	w := httptest.NewRecorder()
	h.catalog(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var got []CatalogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 || got[0].Code != BadgeFirstMatch {
		t.Fatalf("got=%+v", got)
	}
}

func TestListUserBadges_PageEnvelopeAnd403(t *testing.T) {
	h := &handler{svc: &stubAchService{
		badges: []UserBadge{{BadgeCode: BadgeFirstMatch, UnlockedAt: time.Now()}},
		total:  1,
	}}
	r := withUser(httptest.NewRequest(http.MethodGet, "/v1/users/u2/badges?page=1&page_size=20", nil), "u1")
	r.SetPathValue("userID", "u2")
	w := httptest.NewRecorder()
	h.listUserBadges(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var page struct {
		Items    []UserBadge `json:"items"`
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
		Total    int64       `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 20 || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}

	h = &handler{svc: &stubAchService{err: ErrForbidden}}
	r = withUser(httptest.NewRequest(http.MethodGet, "/v1/users/u2/badges", nil), "u1")
	r.SetPathValue("userID", "u2")
	w = httptest.NewRecorder()
	h.listUserBadges(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestPutGoal_Validates(t *testing.T) {
	h := &handler{svc: &stubAchService{err: ErrInvalid}}
	body := []byte(`{"weekly_target":99}`)
	r := withUser(httptest.NewRequest(http.MethodPut, "/v1/me/goal", bytes.NewReader(body)), "u1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.putGoal(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}

	h = &handler{svc: &stubAchService{goal: GoalState{}}}
	body = []byte(`{"weekly_target":3,"reminder_opt_in":true}`)
	r = withUser(httptest.NewRequest(http.MethodPut, "/v1/me/goal", bytes.NewReader(body)), "u1")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.putGoal(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetGoal_OK(t *testing.T) {
	h := &handler{svc: &stubAchService{goal: GoalState{WeeklyTarget: 2, ReminderOptIn: true}}}
	r := withUser(httptest.NewRequest(http.MethodGet, "/v1/me/goal", nil), "u1")
	w := httptest.NewRecorder()
	h.getGoal(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}
