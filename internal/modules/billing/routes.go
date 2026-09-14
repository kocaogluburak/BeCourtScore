package billing

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers billing verify + store webhook routes.
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Post("/v1/billing/google/verify", h.verifyGoogle)
		r.Post("/v1/billing/apple/verify", h.verifyApple)
	})

	r.Post("/v1/billing/google/rtdn", h.googleRTDN)
	r.Post("/v1/billing/apple/notifications", h.appleASN)
}
