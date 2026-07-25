# Domain: identity — auth + profile

> Focused notes for the `identity` module. See `../../../CLAUDE.md` for the whole backend.

Handles authentication (Google Sign-In today), JWT sessions, refresh-token rotation,
the current-user profile (`/v1/me`), and mounts the SSE stream (`/v1/events`).

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts routes; public `POST /v1/auth/{provider}` + `refresh`, protected `logout`/`me`, SSE `/v1/events` |
| `handler.go` | HTTP handlers; request/response DTOs, validation, error mapping |
| `service.go` | Business logic: verify provider token, upsert user, issue/rotate tokens |
| `provider.go` | `IdentityProvider` interface (`Verify(idToken) → external identity`) |
| `google.go` | Google provider impl — verifies against `GOOGLE_CLIENT_IDS` |
| `repo.go` | Postgres: `users`, `auth_identities`, `refresh_tokens` |
| `*_test.go` | `handler_test.go`, `google_test.go` — part of the 15-test suite |

## Routes

```
POST  /v1/auth/{provider}   → verify id_token, upsert user, return {access_token, refresh_token, user, is_new_user}
POST  /v1/auth/refresh      → rotate token pair (refresh in body)
POST  /v1/auth/logout       → revoke refresh token (Bearer + refresh body)
GET   /v1/me                → current profile (Bearer)
PATCH /v1/me                → update nickname/name/surname/profile_icon → pushes SSE user.updated
GET   /v1/events            → per-user SSE stream (Bearer)
```

## Flow

1. Client sends provider `id_token` → `service` calls the matching `IdentityProvider.Verify`.
2. On success, upsert `users` + `auth_identities`; issue access JWT (~15m) + refresh (stored hashed).
3. `refresh` rotates the pair; `logout` revokes.
4. `PATCH /v1/me` publishes `user.updated` on the SSE hub so other sessions update live.

## Adding a new auth provider

1. Implement `identity.IdentityProvider` (see `provider.go` / `google.go`).
2. Register it in `cmd/api/main.go` keyed by the `{provider}` path segment.
3. Add its client ID/secret to config + `.env.example`.
4. Add a `*_test.go` case (verify success + rejection).

## Gotchas

- JWT signing uses `platform/authkit`; secret from `JWT_SECRET`. Never log tokens.
- Refresh tokens are stored **hashed** — compare hashes, never plaintext.
- SSE is owned here (hub handler mounted in `routes.go`) but the hub itself lives in `platform/sse`.

## After changes

`cd BeCourtScore && go test ./internal/modules/identity/...` then `go test ./...` (15 tests must pass).
