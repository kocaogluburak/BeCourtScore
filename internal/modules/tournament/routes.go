// Package tournament contains tournament/ladder management.
// Domain: brackets (single-elimination), round-robin, participants, draws,
// results with opponent confirmation, and live bracket events over SSE.
// Routes: /v1/tournaments/*
package tournament

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Mount registers tournament routes onto the provided router.
// All routes require an authenticated user (authMW).
func Mount(r chi.Router, svc svcFacade, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc}

	r.Group(func(r chi.Router) {
		r.Use(authMW)

		r.Post("/v1/tournaments", h.create)
		r.Get("/v1/tournaments/mine", h.listMine)
		r.Get("/v1/tournaments/{ref}", h.getBySlug)
		r.Delete("/v1/tournaments/{ref}", h.delete)
		r.Post("/v1/tournaments/{ref}/join", h.join)
		r.Get("/v1/tournaments/{ref}/participants", h.participants)
		r.Delete("/v1/tournaments/{ref}/participants/me", h.leave)
		r.Post("/v1/tournaments/{ref}/lock", h.lock)
		r.Post("/v1/tournaments/{ref}/draw", h.draw)
		r.Get("/v1/tournaments/{ref}/bracket", h.bracket)

		r.Post("/v1/tournaments/matches/{matchId}/result", h.result)
		r.Post("/v1/tournaments/matches/{matchId}/confirm", h.confirm)
	})
}
