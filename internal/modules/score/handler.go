package score

import (
	"context"
	"errors"
	"net/http"
	"time"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

// svcFacade is the subset of Service methods used by the HTTP handler,
// so tests can substitute a stub without hitting the database.
type svcFacade interface {
	CreateMatch(ctx context.Context, userID string, in CreateInput) (Match, error)
	ListMyMatches(ctx context.Context, userID string, f ListFilter, limit, offset int) ([]Match, int64, error)
	ListUserMatches(ctx context.Context, viewerID, targetID string, f ListFilter, limit, offset int) ([]Match, int64, error)
	GetMatch(ctx context.Context, viewerID, matchID string) (Match, error)
	DeleteMatch(ctx context.Context, userID, matchID string) error
	StartLiveMatch(ctx context.Context, userID string, in LiveStartInput) (LiveMatch, error)
	GetLiveMatch(ctx context.Context, viewerID, id string) (LiveMatch, error)
	UpdateLiveMatch(ctx context.Context, userID, id string, u LiveScoreUpdate) (LiveMatch, error)
	EndLiveMatch(ctx context.Context, userID, id string, in LiveEndInput) (LiveMatch, error)
	ListMyOpenLiveMatches(ctx context.Context, userID string, limit, offset int) ([]LiveMatch, int64, error)
	CancelLiveMatch(ctx context.Context, userID, id string) (LiveMatch, error)
	AcceptLiveMatch(ctx context.Context, userID, id string) (LiveMatch, error)
	DeclineLiveMatch(ctx context.Context, userID, id string) (LiveMatch, error)
}

type handler struct {
	svc svcFacade
}

// --- POST /v1/matches ---

func (h *handler) createMatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		Sport         string     `json:"sport"`
		PlayerAName   string     `json:"player_a_name"`
		PlayerBName   string     `json:"player_b_name"`
		PlayerAUserID *string    `json:"player_a_user_id"`
		PlayerBUserID *string    `json:"player_b_user_id"`
		SetsA         int        `json:"sets_a"`
		SetsB         int        `json:"sets_b"`
		WinnerSide    string     `json:"winner_side"`
		PlayedAt      *time.Time `json:"played_at"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	m, err := h.svc.CreateMatch(r.Context(), userID, CreateInput{
		Sport:         body.Sport,
		PlayerAName:   body.PlayerAName,
		PlayerBName:   body.PlayerBName,
		PlayerAUserID: body.PlayerAUserID,
		PlayerBUserID: body.PlayerBUserID,
		SetsA:         body.SetsA,
		SetsB:         body.SetsB,
		WinnerSide:    body.WinnerSide,
		PlayedAt:      body.PlayedAt,
	})
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to save match")
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// --- GET /v1/matches ---

func (h *handler) listMyMatches(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	f := ListFilter{
		Query: r.URL.Query().Get("q"),
		Sport: r.URL.Query().Get("sport"),
	}

	matches, total, err := h.svc.ListMyMatches(r.Context(), userID, f, page.PageSize, page.Offset())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list matches")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(matches, page, total))
}

// --- GET /v1/users/{userID}/matches ---

func (h *handler) listUserMatches(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := authkit.UserIDFromCtx(r.Context())
	targetID := r.PathValue("userID")
	page := httpx.ParsePage(r)
	f := ListFilter{
		Query: r.URL.Query().Get("q"),
		Sport: r.URL.Query().Get("sport"),
	}

	matches, total, err := h.svc.ListUserMatches(r.Context(), viewerID, targetID, f, page.PageSize, page.Offset())
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			httpx.Error(w, http.StatusForbidden, "not friends with this user")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to list matches")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(matches, page, total))
}

// --- GET /v1/matches/{id} ---

func (h *handler) getMatch(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := authkit.UserIDFromCtx(r.Context())
	matchID := r.PathValue("id")

	m, err := h.svc.GetMatch(r.Context(), viewerID, matchID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "match not found")
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, "no access to this match")
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to get match")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

// --- DELETE /v1/matches/{id} ---

func (h *handler) deleteMatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	matchID := r.PathValue("id")

	if err := h.svc.DeleteMatch(r.Context(), userID, matchID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "match not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to delete match")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
