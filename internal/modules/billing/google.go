package billing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"
)

const playPackageName = "com.court.score"

// PlayVerifier talks to the Google Play Developer API.
type PlayVerifier struct {
	svc *androidpublisher.Service
}

// newPlayVerifierFromEnv loads the Play Developer API client.
// A nil verifier with a nil error means the credentials are unset.
func newPlayVerifierFromEnv() (*PlayVerifier, error) {
	raw := strings.TrimSpace(os.Getenv("PLAY_SERVICE_ACCOUNT_JSON_CONTENT"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("PLAY_SERVICE_ACCOUNT_JSON"))
		if path == "" {
			return nil, nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	svc, err := androidpublisher.NewService(context.Background(), option.WithCredentialsJSON([]byte(raw)))
	if err != nil {
		return nil, err
	}
	return &PlayVerifier{svc: svc}, nil
}

func (p *PlayVerifier) VerifyGoogle(ctx context.Context, purchaseToken, productID string) (VerifiedSub, error) {
	if p == nil || p.svc == nil {
		return VerifiedSub{}, errVerifierUnavailable
	}
	if purchaseToken == "" || productID == "" {
		return VerifiedSub{}, fmt.Errorf("%w: missing purchase token", ErrInvalid)
	}
	got, err := p.svc.Purchases.Subscriptionsv2.Get(playPackageName, purchaseToken).Context(ctx).Do()
	if err != nil {
		return VerifiedSub{}, err
	}
	return googlePurchaseToSub(got, purchaseToken, productID)
}

func googlePurchaseToSub(got *androidpublisher.SubscriptionPurchaseV2, token, productID string) (VerifiedSub, error) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	if len(got.LineItems) > 0 && got.LineItems[0].ExpiryTime != "" {
		if t, err := time.Parse(time.RFC3339, got.LineItems[0].ExpiryTime); err == nil {
			expires = t
		}
		if got.LineItems[0].ProductId != "" {
			productID = got.LineItems[0].ProductId
		}
	}
	status := "expired"
	switch got.SubscriptionState {
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_CANCELED":
		if expires.After(time.Now()) {
			status = "active"
		}
	case "SUBSCRIPTION_STATE_IN_GRACE_PERIOD":
		status = "grace"
	case "SUBSCRIPTION_STATE_EXPIRED", "SUBSCRIPTION_STATE_EXPIRED_IN_DUNNING":
		status = "expired"
	case "SUBSCRIPTION_STATE_REVOKED":
		status = "revoked"
	default:
		if expires.After(time.Now()) {
			status = "active"
		}
	}
	txn := got.LatestOrderId
	if txn == "" {
		txn = token
	}
	var grace *time.Time
	if status == "grace" {
		g := expires.Add(7 * 24 * time.Hour)
		grace = &g
	}
	return VerifiedSub{
		Store:         "google",
		ProductID:     productID,
		OriginalTxnID: txn,
		ExpiresAt:     expires,
		GraceUntil:    grace,
		Status:        status,
		PurchaseToken: token,
	}, nil
}

func ParseGoogleRTDN(dataB64 string) (purchaseToken, productID string, err error) {
	raw, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(dataB64)
		if err != nil {
			return "", "", err
		}
	}
	var payload struct {
		SubscriptionNotification *struct {
			PurchaseToken  string `json:"purchaseToken"`
			SubscriptionID string `json:"subscriptionId"`
		} `json:"subscriptionNotification"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", err
	}
	if payload.SubscriptionNotification == nil {
		return "", "", nil
	}
	return payload.SubscriptionNotification.PurchaseToken, payload.SubscriptionNotification.SubscriptionID, nil
}
