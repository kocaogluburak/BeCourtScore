package httpapi

import (
	"net/http"
	"strings"

	"courtscore/internal/modules/identity"
	"courtscore/internal/modules/notify"
	"courtscore/internal/modules/score"
	"courtscore/internal/modules/social"
	"courtscore/internal/modules/tournament"
	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/config"
	"courtscore/internal/platform/httpx"
	"courtscore/internal/platform/sse"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// New builds and returns the root HTTP handler with all routes mounted.
func New(
	cfg config.Config,
	identitySvc *identity.Service,
	scoreSvc *score.Service,
	socialSvc *social.Service,
	tournamentSvc *tournament.Service,
	notifySvc *notify.Service,
	hub *sse.Hub,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.CORSOrigins))

	// Health
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	authMW := authkit.Middleware(cfg.JWTSecret)

	identity.Mount(r, identitySvc, hub, authMW)
	score.Mount(r, scoreSvc, authMW)
	social.Mount(r, socialSvc, hub, authMW)
	tournament.Mount(r, tournamentSvc, authMW)
	notify.Mount(r, notifySvc, authMW)

	return r
}

func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	allowAll := len(origins) == 1 && origins[0] == "*"
	originSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll || originSet[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if strings.ToUpper(r.Method) == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
