package authkit

import (
	"context"
	"net/http"
	"strings"

	"courtscore/internal/platform/httpx"
)

type ctxKey struct{}

// UserIDKey is the context key for the authenticated user ID.
var UserIDKey = ctxKey{}

// UserIDFromCtx retrieves the authenticated user ID from the request context.
func UserIDFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(UserIDKey).(string)
	return v, ok && v != ""
}

// Middleware returns an HTTP middleware that validates the Bearer JWT and
// injects the userID into the request context. Requests without a valid token
// receive 401.
func Middleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			token, found := strings.CutPrefix(auth, "Bearer ")
			if !found || token == "" {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			userID, err := ParseAccessToken(token, secret)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
