package social

import (
	"context"
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
	"courtscore/internal/platform/sse"
)

// svcFacade is the subset of Service methods used by the HTTP handler,
// so tests can substitute a stub without hitting the database.
type svcFacade interface {
	SearchUsers(ctx context.Context, viewerID, query string, limit, offset int) ([]SearchResult, int64, error)
	GetUserProfile(ctx context.Context, viewerID, targetID string) (UserSummary, error)
	SendRequest(ctx context.Context, requesterID, addresseeID string) (Friendship, error)
	AcceptRequest(ctx context.Context, userID, requestID string) (Friendship, error)
	RejectRequest(ctx context.Context, userID, requestID string) (Friendship, error)
	ListFriends(ctx context.Context, userID string, limit, offset int) ([]UserSummary, int64, error)
	ListIncomingRequests(ctx context.Context, userID string, limit, offset int) ([]IncomingRequest, int64, error)
	Unfriend(ctx context.Context, userID, otherID string) error
}

type handler struct {
	svc svcFacade
	hub *sse.Hub
}

// --- GET /v1/users/search?q= ---

func (h *handler) searchUsers(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)

	results, total, err := h.svc.SearchUsers(r.Context(), viewerID, r.URL.Query().Get("q"), page.PageSize, page.Offset())
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "search failed")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(results, page, total))
}

// --- GET /v1/users/{userID} ---

func (h *handler) getUserProfile(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := authkit.UserIDFromCtx(r.Context())
	targetID := r.PathValue("userID")

	profile, err := h.svc.GetUserProfile(r.Context(), viewerID, targetID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "user not found")
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, "not friends with this user")
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to get profile")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}

// --- GET /v1/friends ---

func (h *handler) listFriends(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)

	friends, total, err := h.svc.ListFriends(r.Context(), userID, page.PageSize, page.Offset())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list friends")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(friends, page, total))
}

// --- GET /v1/friends/requests ---

func (h *handler) listRequests(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)

	reqs, total, err := h.svc.ListIncomingRequests(r.Context(), userID, page.PageSize, page.Offset())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list requests")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(reqs, page, total))
}

// --- POST /v1/friends/requests ---

func (h *handler) sendRequest(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		UserID string `json:"user_id"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.UserID == "" {
		httpx.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	f, err := h.svc.SendRequest(r.Context(), userID, body.UserID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "user not found")
		case errors.Is(err, ErrConflict):
			httpx.Error(w, http.StatusConflict, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to send request")
		}
		return
	}

	if h.hub != nil {
		h.hub.Publish(f.AddresseeID, sse.Event{Type: "friend.request_received", Data: f})
	}
	httpx.JSON(w, http.StatusCreated, f)
}

// --- POST /v1/friends/requests/{id}/accept ---

func (h *handler) acceptRequest(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	f, err := h.svc.AcceptRequest(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "request not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to accept request")
		return
	}

	if h.hub != nil {
		h.hub.Publish(f.RequesterID, sse.Event{Type: "friend.request_accepted", Data: f})
	}
	httpx.JSON(w, http.StatusOK, f)
}

// --- POST /v1/friends/requests/{id}/reject ---

func (h *handler) rejectRequest(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	f, err := h.svc.RejectRequest(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "request not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to reject request")
		return
	}
	httpx.JSON(w, http.StatusOK, f)
}

// --- DELETE /v1/friends/{userID} ---

func (h *handler) unfriend(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	otherID := r.PathValue("userID")

	if err := h.svc.Unfriend(r.Context(), userID, otherID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "friendship not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to unfriend")
		return
	}

	if h.hub != nil {
		h.hub.Publish(otherID, sse.Event{Type: "friend.removed", Data: map[string]string{"user_id": userID}})
	}
	w.WriteHeader(http.StatusNoContent)
}
