// Package score contains match/score management.
// Domain: finished match history + in-progress live shared scoreboards.
// Routes: /v1/matches/*, /v1/live-matches/*, /v1/users/{userID}/matches
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

		r.Post("/v1/live-matches", h.startLiveMatch)
		r.Get("/v1/live-matches", h.listMyOpenLiveMatches)
		r.Get("/v1/live-matches/{id}", h.getLiveMatch)
		r.Patch("/v1/live-matches/{id}", h.updateLiveMatch)
		r.Post("/v1/live-matches/{id}/end", h.endLiveMatch)
		r.Post("/v1/live-matches/{id}/cancel", h.cancelLiveMatch)
	})
}
