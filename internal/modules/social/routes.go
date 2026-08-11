// Package social contains friendships and public user lookups.
// Domain: friend requests (pending/accepted/rejected), friend lists,
// user search, friend profile visibility.
// Routes: /v1/friends/*, /v1/users/search, /v1/users/{userID}
package social

import (
	"net/http"

	"courtscore/internal/platform/sse"
	"github.com/go-chi/chi/v5"
)

// Mount registers social routes onto the provided router.
// All routes require an authenticated user (authMW). hub pushes
// friend.* events to counterpart users; push is best-effort FCM.
func Mount(r chi.Router, svc svcFacade, hub *sse.Hub, push PushSender, authMW func(http.Handler) http.Handler) {
	h := &handler{svc: svc, hub: hub, push: push}

	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/v1/users/search", h.searchUsers)
		r.Get("/v1/users/{userID}", h.getUserProfile)

		r.Get("/v1/friends", h.listFriends)
		r.Delete("/v1/friends/{userID}", h.unfriend)
		r.Get("/v1/friends/requests", h.listRequests)
		r.Post("/v1/friends/requests", h.sendRequest)
		r.Post("/v1/friends/requests/{id}/accept", h.acceptRequest)
		r.Post("/v1/friends/requests/{id}/reject", h.rejectRequest)
	})
}
