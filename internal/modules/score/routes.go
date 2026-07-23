// Package score contains match/score management.
// Domain: store finished match results, query history.
// Routes: /v1/matches/*, /v1/users/{userID}/matches
package score

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers score routes onto the provided router.
// All routes require an authenticated user (authMW).
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Post("/v1/matches", h.createMatch)
		r.Get("/v1/matches", h.listMyMatches)
		r.Get("/v1/matches/{id}", h.getMatch)
		r.Delete("/v1/matches/{id}", h.deleteMatch)
		r.Get("/v1/users/{userID}/matches", h.listUserMatches)
	})
}
