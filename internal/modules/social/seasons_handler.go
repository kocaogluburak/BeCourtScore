package social

import (
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

func (h *handler) listSeasons(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	page := httpx.ParsePage(r)
	items, total, err := h.svc.ListSeasons(r.Context(), userID, r.PathValue("id"), page.PageSize, page.Offset())
	if err != nil {
		writeGroupErr(w, err, "failed to list seasons")
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.NewPage(items, page, total))
}

func (h *handler) getCurrentSeason(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	detail, err := h.svc.GetCurrentSeason(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeGroupErr(w, err, "failed to load season")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *handler) getSeason(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	detail, err := h.svc.GetSeason(r.Context(), userID, r.PathValue("id"), r.PathValue("seasonId"))
	if err != nil {
		writeGroupErr(w, err, "failed to load season")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *handler) startSeason(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Weeks             int  `json:"weeks"`
		DivisionCount     int  `json:"division_count"`
		CountSocialNights bool `json:"count_social_nights"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	detail, err := h.svc.StartSeason(r.Context(), userID, r.PathValue("id"), SeasonConfig{
		Weeks:             body.Weeks,
		DivisionCount:     body.DivisionCount,
		CountSocialNights: body.CountSocialNights,
	})
	if err != nil {
		writeGroupErr(w, err, "failed to start season")
		return
	}
	httpx.JSON(w, http.StatusCreated, detail)
}

func (h *handler) finalizeSeason(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	detail, err := h.svc.FinalizeSeason(r.Context(), userID, r.PathValue("id"), r.PathValue("seasonId"))
	if err != nil {
		writeGroupErr(w, err, "failed to finalize season")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}
