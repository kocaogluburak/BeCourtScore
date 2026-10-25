package score

import (
	"errors"
	"net/http"
	"strconv"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

// GET /v1/me/wrapped?year=YYYY — self only, FREE (no AllowStats).
func (h *handler) getMyWrapped(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	raw := r.URL.Query().Get("year")
	if raw == "" {
		httpx.Error(w, http.StatusBadRequest, "year is required")
		return
	}
	year, err := strconv.Atoi(raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid year")
		return
	}

	report, err := h.svc.GetWrapped(r.Context(), userID, year)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to get wrapped")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, report)
}

// POST /v1/me/wrapped/share?year=YYYY — mint a public share token (FREE).
func (h *handler) shareMyWrapped(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	raw := r.URL.Query().Get("year")
	if raw == "" {
		httpx.Error(w, http.StatusBadRequest, "year is required")
		return
	}
	year, err := strconv.Atoi(raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid year")
		return
	}
	out, err := h.svc.ShareWrapped(r.Context(), userID, year)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Error(w, http.StatusBadRequest, err.Error())
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to share wrapped")
		}
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

// GET /v1/public/wrapped/{token} — frozen snapshot, no auth (FREE).
func (h *handler) getPublicWrapped(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		httpx.Error(w, http.StatusNotFound, "wrapped not found")
		return
	}
	report, err := h.svc.GetPublicWrapped(r.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Error(w, http.StatusNotFound, "wrapped not found")
		default:
			httpx.Error(w, http.StatusInternalServerError, "failed to get wrapped")
		}
		return
	}
	httpx.JSON(w, http.StatusOK, report)
}
