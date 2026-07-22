package identity

import (
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
	"courtscore/internal/platform/sse"
)

type handler struct {
	svc *Service
	hub *sse.Hub
}

// --- auth: POST /v1/auth/{provider} ---

func (h *handler) authWithProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")

	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.IDToken == "" {
		httpx.Error(w, http.StatusBadRequest, "id_token required")
		return
	}

	session, user, isNewUser, err := h.svc.AuthWithProvider(r.Context(), provider, body.IDToken)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "authentication failed")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token":  session.AccessToken,
		"refresh_token": session.RefreshToken,
		"expires_in":    session.ExpiresIn,
		"is_new_user":   isNewUser,
		"user":          user,
	})
}

// --- auth: POST /v1/auth/refresh ---

func (h *handler) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.RefreshToken == "" {
		httpx.Error(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	session, user, err := h.svc.RefreshSession(r.Context(), body.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidToken) {
			httpx.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "refresh failed")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token":  session.AccessToken,
		"refresh_token": session.RefreshToken,
		"expires_in":    session.ExpiresIn,
		"user":          user,
	})
}

// --- auth: POST /v1/auth/logout ---

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.RefreshToken == "" {
		httpx.Error(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	if err := h.svc.RevokeSession(r.Context(), body.RefreshToken); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "logout failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- profile: GET /v1/me ---

func (h *handler) getMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	user, err := h.svc.GetUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

// --- profile: PATCH /v1/me ---

func (h *handler) patchMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())

	var body struct {
		Nickname    *string `json:"nickname"`
		Name        *string `json:"name"`
		Surname     *string `json:"surname"`
		ProfileIcon *string `json:"profile_icon"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.svc.UpdateUser(r.Context(), userID, UpdateInput{
		Nickname:    body.Nickname,
		Name:        body.Name,
		Surname:     body.Surname,
		ProfileIcon: body.ProfileIcon,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "update failed")
		return
	}

	if h.hub != nil {
		h.hub.Publish(userID, sse.Event{Type: "user.updated", Data: user})
	}

	httpx.JSON(w, http.StatusOK, user)
}
