package identity

import (
	"net/http"

	"courtscore/internal/platform/sse"
	"github.com/go-chi/chi/v5"
)

// Mount registers all identity routes onto the provided router.
// authMW is the Bearer-JWT middleware; hub is used to push user.updated events.
func Mount(r chi.Router, svc *Service, hub *sse.Hub, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc, hub: hub}

	// Public auth routes
	r.Post("/v1/auth/{provider}", h.authWithProvider)
	r.Post("/v1/auth/refresh", h.refresh)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Post("/v1/auth/logout", h.logout)
		r.Get("/v1/me", h.getMe)
		r.Patch("/v1/me", h.patchMe)
	})

	// SSE stream — also protected
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/v1/events", hub.Handler())
	})
}
