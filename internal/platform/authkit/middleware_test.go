package authkit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddleware_MissingBearer(t *testing.T) {
	mw := Middleware([]byte("secret"))
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
	if called {
		t.Fatal("next should not run")
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	mw := Middleware([]byte("secret"))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestMiddleware_ValidTokenInjectsUserID(t *testing.T) {
	secret := []byte("secret")
	token, err := IssueAccessToken("user-42", secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	mw := Middleware(secret)
	var gotUser string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserIDFromCtx(r.Context())
		if !ok {
			t.Fatal("missing user id")
		}
		gotUser = id
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
	if gotUser != "user-42" {
		t.Fatalf("user=%q", gotUser)
	}
}
