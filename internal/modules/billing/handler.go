package billing

import (
	"context"
	"errors"
	"net/http"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

type svcFacade interface {
	VerifyGoogle(ctx context.Context, userID, purchaseToken, productID string) error
	VerifyApple(ctx context.Context, userID, signedTransaction string) error
	HandleGoogleRTDN(ctx context.Context, dataB64 string) error
	HandleAppleASN(ctx context.Context, signedPayload string) error
}

type handler struct {
	svc svcFacade
}

func (h *handler) verifyGoogle(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		PurchaseToken string `json:"purchase_token"`
		ProductID     string `json:"product_id"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.PurchaseToken == "" || body.ProductID == "" {
		httpx.Error(w, http.StatusBadRequest, "purchase_token and product_id required")
		return
	}
	if err := h.svc.VerifyGoogle(r.Context(), userID, body.PurchaseToken, body.ProductID); err != nil {
		writeVerifyErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) verifyApple(w http.ResponseWriter, r *http.Request) {
	userID, _ := authkit.UserIDFromCtx(r.Context())
	var body struct {
		SignedTransaction string `json:"signed_transaction"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.SignedTransaction == "" {
		httpx.Error(w, http.StatusBadRequest, "signed_transaction required")
		return
	}
	if err := h.svc.VerifyApple(r.Context(), userID, body.SignedTransaction); err != nil {
		writeVerifyErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) googleRTDN(w http.ResponseWriter, r *http.Request) {
	var envelope struct {
		Message struct {
			Data string `json:"data"`
		} `json:"message"`
	}
	if err := httpx.Decode(r, &envelope); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid rtdn body")
		return
	}
	if err := h.svc.HandleGoogleRTDN(r.Context(), envelope.Message.Data); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid rtdn payload")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) appleASN(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SignedPayload string `json:"signedPayload"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.SignedPayload == "" {
		httpx.Error(w, http.StatusBadRequest, "signedPayload required")
		return
	}
	if err := h.svc.HandleAppleASN(r.Context(), body.SignedPayload); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid apple notification")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeVerifyErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, errVerifierUnavailable):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	default:
		httpx.Error(w, http.StatusBadGateway, "store verification failed")
	}
}
