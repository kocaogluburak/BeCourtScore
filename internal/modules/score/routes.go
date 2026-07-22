// Package score will contain match/score management.
// Domain: track live match scores, store results, query history.
// Routes: /v1/matches/*
// Status: planned — not implemented in v1.
package score

import "github.com/go-chi/chi/v5"

// Mount registers score routes. Currently a no-op placeholder.
func Mount(_ chi.Router) {}
