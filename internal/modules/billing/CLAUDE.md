# Domain: billing — Pro subscriptions

> Focused notes for the `billing` module. See `../../../CLAUDE.md` for the whole backend.

Entitlement snapshot (match count + store subscriptions) and Play/App Store verify + webhooks.
Score/tournament call `entitlement.Gate`; identity nests `entitlement` on `/v1/me` and auth.

## Files

| File | Role |
|---|---|
| `service.go` | Snapshot, gates, ApplyVerified |
| `repo.go` | `subscriptions` + finished-match COUNT |
| `handler.go` / `routes.go` | verify + RTDN/ASN |
| `google.go` / `apple.go` | Play Developer API + StoreKit JWS |

## Routes

```
POST /v1/billing/google/verify        Bearer { purchase_token, product_id }
POST /v1/billing/apple/verify         Bearer { signed_transaction }
POST /v1/billing/google/rtdn          public Play RTDN
POST /v1/billing/apple/notifications  public ASN v2
```

Products: `pro_monthly`, `pro_yearly`. Free quota: 10 finished cloud matches, then Pro for cloud write / stats / public live / tournament max>8.

Play verify needs `PLAY_SERVICE_ACCOUNT_JSON` (Compose: `/run/secrets/play.json`). Startup logs `billing: Play verifier ready` or `billing: Play verifier unavailable`. Missing creds make `POST /v1/billing/google/verify` return 400 `store verifier unavailable`.

## After changes

`cd BeCourtScore && go test ./internal/modules/billing/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
