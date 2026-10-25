package achievements

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers Bearer-protected achievement + goal routes.
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/v1/achievements/badges", h.catalog)
		r.Get("/v1/users/{userID}/badges", h.listUserBadges)
		r.Get("/v1/me/goal", h.getGoal)
		r.Put("/v1/me/goal", h.putGoal)
	})
}
