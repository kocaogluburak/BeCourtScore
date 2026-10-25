package score

import (
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

func writeRematchErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "rematch not found")
	case errors.Is(err, ErrConflict), errors.Is(err, ErrWrongState):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, fallback)
	}
}

func (h *handler) createRematch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		OpponentID       string  `json:"opponent_id"`
		Sport            string  `json:"sport"`
		SourceMatchID    *string `json:"source_match_id"`
		SetsToWin        int     `json:"sets_to_win"`
		AdvantageEnabled *bool   `json:"advantage_enabled"`
		PointsToWin      int     `json:"points_to_win"`
		SetRule          *string `json:"set_rule"`
		FinalSet         *string `json:"final_set"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ch, err := h.svc.CreateRematch(r.Context(), userID, RematchCreateInput{
		OpponentID:       body.OpponentID,
		Sport:            body.Sport,
		SourceMatchID:    body.SourceMatchID,
		SetsToWin:        body.SetsToWin,
		AdvantageEnabled: body.AdvantageEnabled,
		PointsToWin:      body.PointsToWin,
		SetRule:          body.SetRule,
		FinalSet:         body.FinalSet,
	})
	if err != nil {
		writeRematchErr(w, err, "failed to create rematch")
		return
	}
	httpx.JSON(w, http.StatusCreated, ch)
}

func (h *handler) listIncomingRematches(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListIncomingRematches(r.Context(), userID, page.PageSize, page.Offset())
	if err != nil {
		writeRematchErr(w, err, "failed to list rematches")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *handler) listOutgoingRematches(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListOutgoingRematches(r.Context(), userID, page.PageSize, page.Offset())
	if err != nil {
		writeRematchErr(w, err, "failed to list rematches")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *handler) acceptRematch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	prefill, err := h.svc.AcceptRematch(r.Context(), userID, id)
	if err != nil {
		writeRematchErr(w, err, "failed to accept rematch")
		return
	}
	httpx.JSON(w, http.StatusOK, prefill)
}

func (h *handler) rejectRematch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	ch, err := h.svc.RejectRematch(r.Context(), userID, id)
	if err != nil {
		writeRematchErr(w, err, "failed to reject rematch")
		return
	}
	httpx.JSON(w, http.StatusOK, ch)
}

func (h *handler) cancelRematch(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	id := r.PathValue("id")
	ch, err := h.svc.CancelRematch(r.Context(), userID, id)
	if err != nil {
		writeRematchErr(w, err, "failed to cancel rematch")
		return
	}
	httpx.JSON(w, http.StatusOK, ch)
}
