package equipment

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers Bearer-protected /v1/me/equipment routes and the catalog read route.
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/v1/equipment/catalog", h.catalog)
		r.Get("/v1/me/equipment", h.list)
		r.Post("/v1/me/equipment", h.create)
		r.Get("/v1/me/equipment/win-rate", h.winRate) // static segment wins over {id}
		r.Get("/v1/me/equipment/{id}", h.get)
		r.Patch("/v1/me/equipment/{id}", h.update)
		r.Delete("/v1/me/equipment/{id}", h.del)
		r.Post("/v1/me/equipment/{id}/activate", h.activate)
		r.Post("/v1/me/equipment/{id}/reset", h.reset)
		r.Post("/v1/me/equipment/{id}/archive", h.archive)
		r.Post("/v1/me/equipment/{id}/renew", h.renew)
	})
}
