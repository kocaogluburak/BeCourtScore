package social

import (
	"context"
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

// feedFacade is the subset of FeedService used by HTTP handlers.
type feedFacade interface {
	ListFeed(ctx context.Context, viewerID string, limit, offset int) ([]ActivityDTO, int64, error)
	AddReaction(ctx context.Context, callerID, activityID, typ string) (ReactionResult, error)
	RemoveReaction(ctx context.Context, callerID, activityID string) error
	ListComments(ctx context.Context, callerID, activityID string, limit, offset int) ([]CommentDTO, int64, error)
	AddComment(ctx context.Context, callerID, activityID, body string) (CommentDTO, error)
	DeleteComment(ctx context.Context, callerID, activityID, commentID string) error
	GetSettings(ctx context.Context, userID string) (FeedSettings, error)
	SetSettings(ctx context.Context, userID string, optOut bool) (FeedSettings, error)
}

type feedHandler struct {
	svc feedFacade
}

func writeFeedErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "forbidden")
	default:
		httpx.Error(w, http.StatusInternalServerError, fallback)
	}
}

func (h *feedHandler) listFeed(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListFeed(r.Context(), userID, page.PageSize, page.Offset())
	if err != nil {
		writeFeedErr(w, err, "failed to list feed")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *feedHandler) addReaction(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Type string `json:"type"`
	}
	_ = httpx.Decode(r, &body)
	if body.Type == "" {
		body.Type = reactionKudos
	}
	res, err := h.svc.AddReaction(r.Context(), userID, r.PathValue("activityID"), body.Type)
	if err != nil {
		writeFeedErr(w, err, "failed to add reaction")
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *feedHandler) removeReaction(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	if err := h.svc.RemoveReaction(r.Context(), userID, r.PathValue("activityID")); err != nil {
		writeFeedErr(w, err, "failed to remove reaction")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *feedHandler) listComments(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListComments(r.Context(), userID, r.PathValue("activityID"), page.PageSize, page.Offset())
	if err != nil {
		writeFeedErr(w, err, "failed to list comments")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *feedHandler) addComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Body string `json:"body"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	c, err := h.svc.AddComment(r.Context(), userID, r.PathValue("activityID"), body.Body)
	if err != nil {
		writeFeedErr(w, err, "failed to add comment")
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

func (h *feedHandler) deleteComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	if err := h.svc.DeleteComment(r.Context(), userID, r.PathValue("activityID"), r.PathValue("commentID")); err != nil {
		writeFeedErr(w, err, "failed to delete comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *feedHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	s, err := h.svc.GetSettings(r.Context(), userID)
	if err != nil {
		writeFeedErr(w, err, "failed to get feed settings")
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *feedHandler) patchSettings(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		FeedOptOut *bool `json:"feed_opt_out"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.FeedOptOut == nil {
		httpx.Error(w, http.StatusBadRequest, "feed_opt_out required")
		return
	}
	s, err := h.svc.SetSettings(r.Context(), userID, *body.FeedOptOut)
	if err != nil {
		writeFeedErr(w, err, "failed to update feed settings")
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}
