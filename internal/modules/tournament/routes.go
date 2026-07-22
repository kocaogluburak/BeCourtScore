// Package tournament will contain tournament/ladder management.
// Domain: brackets, participants, scheduling, leaderboards.
// Routes: /v1/tournaments/*
// Status: planned — not implemented in v1.
package tournament

import "github.com/go-chi/chi/v5"

// Mount registers tournament routes. Currently a no-op placeholder.
func Mount(_ chi.Router) {}
