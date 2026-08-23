# Domain: notify — device tokens + FCM

> Focused notes for the `notify` module. See `../../../CLAUDE.md` for the whole backend.

Registers FCM device tokens and fans out push. When Firebase credentials are missing,
the sender is a no-op (`NoopSender`) so local/dev still boots.

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts `/v1/devices` (Bearer) |
| `handler.go` | Register + unregister |
| `service.go` | Upsert/delete tokens; `SendToUser` fan-out |
| `repo.go` | Postgres `device_tokens` |
| `fcm.go` | FCM HTTP v1 sender; `NewFCMSenderFromEnv` → Noop if unset |
| `*_test.go` | handler, service, fcm env fallback |

## Routes

```
POST   /v1/devices           → {token, platform: android|ios|web} → 201
DELETE /v1/devices/{token}   → 204 (token URL-encoded)
```

## Who sends

Other modules call `notify.Service.SendToUser` (implements `score.PushSender`):

- Live invite / accept (`score`)
- Friend request received / accepted (`social`)

Best-effort: push failure must not fail the HTTP handler.

## Gotchas

- Platform must be `android`, `ios`, or `web`.
- Credentials: `FIREBASE_CREDENTIALS_JSON` or `GOOGLE_APPLICATION_CREDENTIALS` (see `.env.example`). Docker mounts `secrets/firebase-adminsdk.json`.
- Do not log tokens.

## After changes

`cd BeCourtScore && go test ./internal/modules/notify/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
