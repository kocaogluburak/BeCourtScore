package equipment

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func passAuth(next http.Handler) http.Handler { return next }

// The static win-rate / catalog paths must not be swallowed by the {id} wildcard,
// and archive / renew must be routed.
func TestMount_RoutesStaticSegmentsBeforeID(t *testing.T) {
	stub := &stubEquip{}
	r := chi.NewRouter()
	Mount(r, stub, passAuth)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/v1/me/equipment/win-rate?sport=TENNIS", http.StatusOK},
		{http.MethodGet, "/v1/equipment/catalog", http.StatusOK},
		{http.MethodPost, "/v1/me/equipment/e1/archive", http.StatusOK},
		{http.MethodPost, "/v1/me/equipment/e1/renew", http.StatusCreated},
		{http.MethodPost, "/v1/me/equipment/e1/activate", http.StatusOK},
		{http.MethodGet, "/v1/me/equipment", http.StatusOK},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%s %s => %d want %d (%s)", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}

	// win-rate must hit winRate (enough_data key), not get (Usage shape).
	stub.one = Usage{ID: "should-not-be-returned"}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/me/equipment/win-rate?sport=TENNIS", nil))
	if got := w.Body.String(); got == "" || strings.Contains(got, "should-not-be-returned") || !strings.Contains(got, "enough_data") {
		t.Fatalf("win-rate routed to {id}: %s", got)
	}
}
