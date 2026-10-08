package social

import (
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

func (h *handler) listGroups(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	groups, err := h.svc.ListGroups(r.Context(), userID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": groups})
}

func (h *handler) createGroup(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Name  string `json:"name"`
		Sport string `json:"sport"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := h.svc.CreateGroup(r.Context(), userID, body.Name, body.Sport)
	if err != nil {
		writeGroupErr(w, err, "failed to create group")
		return
	}
	httpx.JSON(w, http.StatusCreated, g)
}

func (h *handler) joinGroup(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Code string `json:"code"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	g, err := h.svc.JoinGroup(r.Context(), userID, body.Code)
	if err != nil {
		writeGroupErr(w, err, "failed to join group")
		return
	}
	httpx.JSON(w, http.StatusOK, g)
}

func (h *handler) getGroup(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	detail, err := h.svc.GetGroup(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeGroupErr(w, err, "failed to load group")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *handler) getGroupByCode(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	detail, err := h.svc.GetGroupByCode(r.Context(), userID, r.PathValue("code"))
	if err != nil {
		writeGroupErr(w, err, "failed to load group")
		return
	}
	httpx.JSON(w, http.StatusOK, detail)
}

func (h *handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	if err := h.svc.DeleteGroup(r.Context(), userID, r.PathValue("id")); err != nil {
		writeGroupErr(w, err, "failed to delete group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) leaveGroup(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	if err := h.svc.LeaveGroup(r.Context(), userID, r.PathValue("id")); err != nil {
		writeGroupErr(w, err, "failed to leave group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeGroupErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "group not found")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "not allowed")
	case errors.Is(err, ErrConflict):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, fallback)
	}
}
