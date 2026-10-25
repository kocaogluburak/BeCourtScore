package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"courtscore/internal/platform/httpx"
)

type stubFeed struct {
	items    []ActivityDTO
	total    int64
	comments []CommentDTO
	result   ReactionResult
	comment  CommentDTO
	settings FeedSettings
	err      error
}

func (s *stubFeed) ListFeed(context.Context, string, int, int) ([]ActivityDTO, int64, error) {
	return s.items, s.total, s.err
}
func (s *stubFeed) AddReaction(context.Context, string, string, string) (ReactionResult, error) {
	return s.result, s.err
}
func (s *stubFeed) RemoveReaction(context.Context, string, string) error { return s.err }
func (s *stubFeed) ListComments(context.Context, string, string, int, int) ([]CommentDTO, int64, error) {
	return s.comments, s.total, s.err
}
func (s *stubFeed) AddComment(context.Context, string, string, string) (CommentDTO, error) {
	return s.comment, s.err
}
func (s *stubFeed) DeleteComment(context.Context, string, string, string) error { return s.err }
func (s *stubFeed) GetSettings(context.Context, string) (FeedSettings, error) {
	return s.settings, s.err
}
func (s *stubFeed) SetSettings(_ context.Context, _ string, opt bool) (FeedSettings, error) {
	s.settings.FeedOptOut = opt
	return s.settings, s.err
}

func TestListFeed_PageEnvelope(t *testing.T) {
	h := &feedHandler{svc: &stubFeed{
		items: []ActivityDTO{{ID: "a1", Type: activityTypeMatch, CreatedAt: time.Now()}},
		total: 1,
	}}
	w := httptest.NewRecorder()
	h.listFeed(w, authedRequest(http.MethodGet, "/v1/feed?page=1&page_size=20", nil, "u1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var page httpx.Page[ActivityDTO]
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 20 || page.Total != 1 || len(page.Items) != 1 || page.HasMore {
		t.Fatalf("page=%+v", page)
	}
}

func TestAddReaction_CreatedAndForbidden(t *testing.T) {
	h := &feedHandler{svc: &stubFeed{result: ReactionResult{ActivityID: "a1", ReactionCount: 2, ViewerHasKudos: true}}}
	req := authedRequest(http.MethodPost, "/v1/feed/a1/reactions", []byte(`{"type":"kudos"}`), "u1")
	req.SetPathValue("activityID", "a1")
	w := httptest.NewRecorder()
	h.addReaction(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}

	h.svc = &stubFeed{err: ErrForbidden}
	req = authedRequest(http.MethodPost, "/v1/feed/a1/reactions", []byte(`{}`), "u9")
	req.SetPathValue("activityID", "a1")
	w = httptest.NewRecorder()
	h.addReaction(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestRemoveReaction_NoContent(t *testing.T) {
	h := &feedHandler{svc: &stubFeed{}}
	req := authedRequest(http.MethodDelete, "/v1/feed/a1/reactions", nil, "u1")
	req.SetPathValue("activityID", "a1")
	w := httptest.NewRecorder()
	h.removeReaction(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestComments_CreateListDelete(t *testing.T) {
	h := &feedHandler{svc: &stubFeed{
		comment:  CommentDTO{ID: "c1", ActivityID: "a1", Body: "gg"},
		comments: []CommentDTO{{ID: "c1", Body: "gg"}},
		total:    1,
	}}
	req := authedRequest(http.MethodPost, "/v1/feed/a1/comments", []byte(`{"body":"gg"}`), "u1")
	req.SetPathValue("activityID", "a1")
	w := httptest.NewRecorder()
	h.addComment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}

	req = authedRequest(http.MethodGet, "/v1/feed/a1/comments?page=1", nil, "u1")
	req.SetPathValue("activityID", "a1")
	w = httptest.NewRecorder()
	h.listComments(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d", w.Code)
	}
	var page httpx.Page[CommentDTO]
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}

	req = authedRequest(http.MethodDelete, "/v1/feed/a1/comments/c1", nil, "u1")
	req.SetPathValue("activityID", "a1")
	req.SetPathValue("commentID", "c1")
	w = httptest.NewRecorder()
	h.deleteComment(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", w.Code)
	}

	h.svc = &stubFeed{err: ErrInvalid}
	req = authedRequest(http.MethodPost, "/v1/feed/a1/comments", []byte(`{"body":""}`), "u1")
	req.SetPathValue("activityID", "a1")
	w = httptest.NewRecorder()
	h.addComment(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty comment status=%d", w.Code)
	}

	h.svc = &stubFeed{err: ErrRateLimited}
	req = authedRequest(http.MethodPost, "/v1/feed/a1/comments", []byte(`{"body":"spam"}`), "u1")
	req.SetPathValue("activityID", "a1")
	w = httptest.NewRecorder()
	h.addComment(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit status=%d", w.Code)
	}
}

func TestFeedSettings_GetAndPatch(t *testing.T) {
	h := &feedHandler{svc: &stubFeed{settings: FeedSettings{FeedOptOut: false}}}
	w := httptest.NewRecorder()
	h.getSettings(w, authedRequest(http.MethodGet, "/v1/me/feed-settings", nil, "u1"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"feed_opt_out":false`) {
		t.Fatalf("get=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.patchSettings(w, authedRequest(http.MethodPatch, "/v1/me/feed-settings", []byte(`{"feed_opt_out":true}`), "u1"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"feed_opt_out":true`) {
		t.Fatalf("patch=%d %s", w.Code, w.Body.String())
	}
}
