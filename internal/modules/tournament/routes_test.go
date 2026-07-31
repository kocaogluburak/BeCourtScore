package tournament

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"courtscore/internal/platform/authkit"

	"github.com/go-chi/chi/v5"
)

// TestMount_MineIsNotCapturedByRef ensures the static /mine route wins over
// GET /v1/tournaments/{ref}. If /mine fell through to getBySlug, an empty
// organizer would see 404 "not found" instead of an empty page.
func TestMount_MineIsNotCapturedByRef(t *testing.T) {
	svc := &stubService{
		tournaments: nil,
		total:       0,
		getErr:      ErrNotFound, // would 404 if {ref} matched "mine"
	}

	r := chi.NewRouter()
	Mount(r, svc, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), authkit.UserIDKey, "u1")
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tournaments/mine", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}
