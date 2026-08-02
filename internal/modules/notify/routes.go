// Package notify handles device token registration and FCM push delivery.
package notify

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers /v1/devices routes.
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Post("/v1/devices", h.register)
		r.Delete("/v1/devices/{token}", h.unregister)
	})
}
