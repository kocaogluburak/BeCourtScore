package invite

import (
	"context"
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

type svcFacade interface {
	Create(ctx context.Context, userID string, in CreateInput) (Created, error)
	ResolvePublic(ctx context.Context, code string) (PublicInvite, error)
}

type handler struct {
	svc svcFacade
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Source        string  `json:"source"`
		Sport         *string `json:"sport"`
		SourceMatchID *string `json:"source_match_id"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.Create(r.Context(), userID, CreateInput{
		Source: body.Source, Sport: body.Sport, SourceMatchID: body.SourceMatchID,
	})
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to create invite")
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *handler) resolvePublic(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	out, err := h.svc.ResolvePublic(r.Context(), code)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "invite not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to resolve invite")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
