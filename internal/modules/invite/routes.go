package invite

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers authenticated invite minting and the public resolve route.
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Post("/v1/invites", h.create)
	})
	r.Get("/v1/public/invite/{code}", h.resolvePublic)
}
