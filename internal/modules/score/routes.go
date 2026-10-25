// Package score contains match/score management.
// Domain: finished match history + in-progress live shared scoreboards.
// Routes: /v1/matches/*, /v1/live-matches/*, /v1/users/{userID}/matches|stats|h2h
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
		r.Get("/v1/matches/{id}/stats", h.getMatchPointStats)
		r.Delete("/v1/matches/{id}", h.deleteMatch)
		r.Get("/v1/users/{userID}/matches", h.listUserMatches)
		r.Get("/v1/users/{userID}/stats", h.getUserStats)
		r.Get("/v1/users/{userID}/h2h", h.getHeadToHead)
		r.Get("/v1/users/{userID}/fitness-trend", h.getFitnessTrend)
		r.Get("/v1/me/wrapped", h.getMyWrapped)

		r.Post("/v1/live-matches", h.startLiveMatch)
		r.Get("/v1/live-matches", h.listMyOpenLiveMatches)
		r.Get("/v1/live-matches/{id}", h.getLiveMatch)
		r.Patch("/v1/live-matches/{id}", h.updateLiveMatch)
		r.Post("/v1/live-matches/{id}/end", h.endLiveMatch)
		r.Post("/v1/live-matches/{id}/cancel", h.cancelLiveMatch)
		r.Post("/v1/live-matches/{id}/accept", h.acceptLiveMatch)
		r.Post("/v1/live-matches/{id}/decline", h.declineLiveMatch)
		r.Post("/v1/live-matches/{id}/share", h.shareLiveMatch)

		r.Post("/v1/rematches", h.createRematch)
		r.Get("/v1/rematches/incoming", h.listIncomingRematches)
		r.Get("/v1/rematches/outgoing", h.listOutgoingRematches)
		r.Post("/v1/rematches/{id}/accept", h.acceptRematch)
		r.Post("/v1/rematches/{id}/reject", h.rejectRematch)
		r.Post("/v1/rematches/{id}/cancel", h.cancelRematch)
	})

	r.Get("/v1/public/live/{token}", h.getPublicLiveMatch)
}
