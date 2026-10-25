package achievements

import (
	"context"
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

type svcFacade interface {
	Catalog() []CatalogEntry
	ListUserBadges(ctx context.Context, viewerID, targetID string, limit, offset int) ([]UserBadge, int64, error)
	GetGoal(ctx context.Context, userID string) (GoalState, error)
	SetGoal(ctx context.Context, userID string, weeklyTarget int, reminderOptIn bool) (GoalState, error)
}

type handler struct {
	svc svcFacade
}

func (h *handler) catalog(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, h.svc.Catalog())
}

func (h *handler) listUserBadges(w http.ResponseWriter, r *http.Request) {
	viewerID, _ := authkit.UserIDFromCtx(r.Context())
	targetID := r.PathValue("userID")
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListUserBadges(r.Context(), viewerID, targetID, page.PageSize, page.Offset())
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			httpx.Error(w, http.StatusForbidden, "not friends with this user")
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to list badges")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *handler) getGoal(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	state, err := h.svc.GetGoal(r.Context(), userID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to get goal")
		return
	}
	httpx.JSON(w, http.StatusOK, state)
}

func (h *handler) putGoal(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		WeeklyTarget  *int  `json:"weekly_target"`
		ReminderOptIn *bool `json:"reminder_opt_in"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target := 0
	if body.WeeklyTarget != nil {
		target = *body.WeeklyTarget
	}
	optIn := true
	if body.ReminderOptIn != nil {
		optIn = *body.ReminderOptIn
	}
	state, err := h.svc.SetGoal(r.Context(), userID, target, optIn)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to set goal")
		return
	}
	httpx.JSON(w, http.StatusOK, state)
}
