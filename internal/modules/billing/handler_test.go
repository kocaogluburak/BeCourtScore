package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"courtscore/internal/platform/authkit"
)

type stubBilling struct {
	googleErr error
	appleErr  error
	rtdnErr   error
	asnErr    error
}

func (s *stubBilling) VerifyGoogle(context.Context, string, string, string) error {
	return s.googleErr
}
func (s *stubBilling) VerifyApple(context.Context, string, string) error { return s.appleErr }
func (s *stubBilling) HandleGoogleRTDN(context.Context, string) error    { return s.rtdnErr }
func (s *stubBilling) HandleAppleASN(context.Context, string) error      { return s.asnErr }

func TestVerifyGoogle_NoContent(t *testing.T) {
	h := &handler{svc: &stubBilling{}}
	body, _ := json.Marshal(map[string]string{"purchase_token": "t", "product_id": ProductMonthly})
	r := httptest.NewRequest(http.MethodPost, "/v1/billing/google/verify", bytes.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "u1"))
	w := httptest.NewRecorder()
	h.verifyGoogle(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d", w.Code)
	}
}

func TestVerifyGoogle_MissingBody(t *testing.T) {
	h := &handler{svc: &stubBilling{}}
	r := httptest.NewRequest(http.MethodPost, "/v1/billing/google/verify", bytes.NewReader([]byte(`{}`)))
	r = r.WithContext(context.WithValue(r.Context(), authkit.UserIDKey, "u1"))
	w := httptest.NewRecorder()
	h.verifyGoogle(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
}

func TestGoogleRTDN_NoContent(t *testing.T) {
	h := &handler{svc: &stubBilling{}}
	body, _ := json.Marshal(map[string]any{"message": map[string]string{"data": "e30="}})
	r := httptest.NewRequest(http.MethodPost, "/v1/billing/google/rtdn", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.googleRTDN(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}
