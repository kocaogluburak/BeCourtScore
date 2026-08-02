package notify

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

type svcFacade interface {
	Register(ctx context.Context, userID, token, platform string) (DeviceToken, error)
	Unregister(ctx context.Context, userID, token string) error
}

type handler struct {
	svc svcFacade
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	d, err := h.svc.Register(r.Context(), userID, body.Token, body.Platform)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to register device")
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *handler) unregister(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	token, err := url.PathUnescape(r.PathValue("token"))
	if err != nil || token == "" {
		httpx.Error(w, http.StatusBadRequest, "token required")
		return
	}
	if err := h.svc.Unregister(r.Context(), userID, token); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "device token not found")
			return
		}
		if errors.Is(err, ErrInvalid) {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to unregister device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
