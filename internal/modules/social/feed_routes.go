package social

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountFeed registers /v1/feed/* and /v1/me/feed-settings under authMW.
func MountFeed(r chi.Router, svc feedFacade, authMW func(http.Handler) http.Handler) {
	if svc == nil {
		return
	}
	h := &feedHandler{svc: svc}
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/v1/feed", h.listFeed)
		r.Post("/v1/feed/{activityID}/reactions", h.addReaction)
		r.Delete("/v1/feed/{activityID}/reactions", h.removeReaction)
		r.Get("/v1/feed/{activityID}/comments", h.listComments)
		r.Post("/v1/feed/{activityID}/comments", h.addComment)
		r.Delete("/v1/feed/{activityID}/comments/{commentID}", h.deleteComment)
		r.Get("/v1/me/feed-settings", h.getSettings)
		r.Patch("/v1/me/feed-settings", h.patchSettings)
	})
}
