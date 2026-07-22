# CourtScore — Backend

Go modular monolith for CourtScore. Single binary, PostgreSQL, provider-agnostic auth (v1: Google Sign-In), JWT sessions, and SSE.

## Requirements

- Go 1.25+
- Docker + Docker Compose (for local Postgres)

## Local setup

```bash
cp .env.example .env
# Edit .env: set JWT_SECRET and GOOGLE_CLIENT_IDS
docker compose up -d postgres
go run ./cmd/api
```

Server starts on `http://localhost:8080`.

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `PORT` | No (default: 8080) | HTTP listen port |
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `JWT_SECRET` | Yes | HS256 signing key — min 32 chars, keep secret |
| `GOOGLE_CLIENT_IDS` | Yes | Comma-separated Google OAuth client IDs (Android + iOS + Web) |
| `CORS_ORIGINS` | No (default: `*`) | Comma-separated allowed CORS origins |

Generate a JWT secret: `openssl rand -hex 32`

## Google Cloud setup

1. Create a project at [console.cloud.google.com](https://console.cloud.google.com).
2. Enable the **People API**.
3. Create OAuth 2.0 credentials:
   - **Android**: package `com.court.score`, SHA-1 fingerprint
   - **iOS**: bundle ID `com.merveburak.courtscore`
   - **Web** (for testing via curl/Postman)
4. Add all client IDs to `GOOGLE_CLIENT_IDS`.

## API

### Health

```
GET /health
→ 200 {"status":"ok"}
```

### Auth

```
POST /v1/auth/google
Body: {"id_token": "<Google ID token from mobile SDK>"}
→ 200 {"access_token","refresh_token","expires_in","is_new_user","user"}
```

```
POST /v1/auth/refresh
Body: {"refresh_token": "..."}
→ 200 {"access_token","refresh_token","expires_in","user"}
```

```
POST /v1/auth/logout          (requires Bearer)
Body: {"refresh_token": "..."}
→ 204
```

### Profile

All profile endpoints require `Authorization: Bearer <access_token>`.

```
GET /v1/me
→ 200 user object

PATCH /v1/me
Body (all fields optional): {"nickname","name","surname","profile_icon"}
→ 200 updated user object
```

### SSE

```
GET /v1/events                (requires Bearer)
Content-Type: text/event-stream
```

Event types:
- `connected` — sent immediately on connection
- `heartbeat` — every 25s to keep connection alive
- `user.updated` — sent after a successful PATCH /v1/me

## Architecture

```
cmd/api/              → composition root (main.go)
internal/
  platform/
    config/           → env-based config
    db/               → pgxpool connect
    httpx/            → JSON helpers
    authkit/          → JWT issue/parse, Bearer middleware
    sse/              → SSE hub (subscribe/publish per userID)
  modules/
    identity/         → auth + profile (v1 implemented)
    score/            → planned: /v1/matches/* (stub)
    tournament/       → planned: /v1/tournaments/* (stub)
  migrate/            → embedded SQL migration runner
```

### Adding a new auth provider

1. Implement `identity.IdentityProvider` interface in a new file (e.g. `internal/modules/identity/apple.go`).
2. Register it in `cmd/api/main.go`:
   ```go
   identitySvc.RegisterProvider("apple", identity.NewAppleProvider(...))
   ```
3. Clients call `POST /v1/auth/apple` — same request/response shape.

### Adding a new domain (e.g. score)

1. Implement handlers/service/repo inside `internal/modules/score/`.
2. Call `score.Mount(r)` in `internal/httpapi/router.go`.
3. Add migrations prefixed `score_` in `internal/migrate/sql/`.

## Tests

```bash
go test ./...
```

## Docker

```bash
docker compose up        # Postgres + API
docker compose up -d     # background
docker compose down
```
