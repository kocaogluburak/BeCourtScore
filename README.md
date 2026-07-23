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
- `friend.request_received` — pushed to the addressee of a new friend request
- `friend.request_accepted` — pushed to the requester when accepted
- `friend.removed` — pushed to the removed side of an unfriend

### Pagination (all list endpoints)

Every list endpoint accepts `?page=` and `?page_size=` (**default 20, max 100**) and returns:

```json
{"items": [...], "page": 1, "page_size": 20, "total": 137, "has_more": true}
```

### Matches

All match endpoints require `Authorization: Bearer <access_token>`.

```
POST /v1/matches
Body: {"sport":"TENNIS|PADEL|SQUASH|PING_PONG",
       "player_a_name","player_b_name",
       "player_a_user_id"?, "player_b_user_id"?,     ← optional registered users
       "sets_a","sets_b","winner_side":"A|B",
       "played_at"?}                                  ← RFC3339, defaults to now
→ 201 match object

GET /v1/matches?page=&page_size=&q=&sport=
→ 200 paginated matches (participant or recorder), newest first

GET /v1/matches/{id}
→ 200 match | 403 (not participant/recorder/friend) | 404

DELETE /v1/matches/{id}                               ← creator only
→ 204 | 404

GET /v1/users/{id}/matches?page=&page_size=
→ 200 paginated matches | 403 unless self or accepted friend
```

The `matches` table is RANGE-partitioned by month on `played_at` (migration `003` creates
partitions from 3 months back to 24 months ahead plus a DEFAULT catch-all). **Ops note:**
create new monthly partitions before the pre-created window runs out; rows past the window
land in `matches_default`, which still works but loses pruning benefits.

### Friends

All endpoints require Bearer. Friendship flow: request → accept/reject; single row per user pair.

```
GET  /v1/users/search?q=<min 2 chars>&page=&page_size=
→ 200 paginated users with "friendship_status": none|pending_sent|pending_received|accepted

GET  /v1/users/{userID}
→ 200 public profile (no email) | 403 unless self or accepted friend

GET  /v1/friends?page=&page_size=          → accepted friends
DELETE /v1/friends/{userID}                → unfriend (204)

GET  /v1/friends/requests?page=&page_size= → incoming pending requests
POST /v1/friends/requests                  → {"user_id":"..."} (201 | 409 duplicate | 404)
POST /v1/friends/requests/{id}/accept      → 200 (addressee only)
POST /v1/friends/requests/{id}/reject      → 200 (addressee only)
```

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
    score/            → matches + history (/v1/matches/*, /v1/users/{id}/matches)
    social/           → friendships + user search (/v1/friends/*, /v1/users/*)
    tournament/       → planned: /v1/tournaments/* (stub)
  migrate/            → embedded SQL migration runner
```

Cross-module dependencies go through small interfaces: `score.FriendChecker` is implemented by
`social.Service` and wired in `cmd/api/main.go`.

### Adding a new auth provider

1. Implement `identity.IdentityProvider` interface in a new file (e.g. `internal/modules/identity/apple.go`).
2. Register it in `cmd/api/main.go`:
   ```go
   identitySvc.RegisterProvider("apple", identity.NewAppleProvider(...))
   ```
3. Clients call `POST /v1/auth/apple` — same request/response shape.

### Adding a new domain

1. Implement handlers/service/repo inside `internal/modules/<name>/` (see `score/` or `social/`).
2. Mount it in `internal/httpapi/router.go` and wire the service in `cmd/api/main.go`.
3. Add numbered migrations in `internal/migrate/sql/`.
4. Paginate every list endpoint with `httpx.ParsePage` / `httpx.NewPage` (default 20, max 100).

## Client ↔ Backend Integration

### Production URL

```
https://api.court-score.com
```

Her iki client da bu URL'i sabit olarak kullanır:
- **Android** — `BuildConfig.API_BASE_URL` (`local.properties` / `-P` Gradle property üzerinden)
- **iOS** — `Config.apiBaseURL` (`CourtScore/Config.swift`)

### End-to-end auth akışı

```
Client                                    Backend
──────                                    ───────
1. Google Sign-In SDK (Credential Mgr /
   GIDSignIn) → Google ID token
   
2. POST /v1/auth/google                →  idTokenPayload → google.tokeninfo
   {"id_token": "<Google ID token>"}       ↓
                                           users + auth_identities upsert
                                       ←  {access_token (JWT, 15 min),
                                           refresh_token (opaque, 30 gün),
                                           is_new_user,
                                           user}
3. is_new_user=true → profile ekranı
   PATCH /v1/me {"nickname", ...}      →  users güncellenir
                                       ←  SSE: user.updated event

4. Token 401 verince:
   POST /v1/auth/refresh               →  refresh_tokens tablosu kontrol
   {"refresh_token": "..."}            ←  yeni access_token + refresh_token (rotation)

5. Uygulama kapatılınca / logout:
   POST /v1/auth/logout                →  refresh_tokens satırı silinir
   {"refresh_token": "..."}
```

### Token depolama

| Platform | access_token | refresh_token |
|---|---|---|
| Android | `EncryptedSharedPreferences` | `EncryptedSharedPreferences` |
| iOS | Keychain | Keychain |

### Google client ID'leri

`GOOGLE_CLIENT_IDS` değişkeni üç ID'yi virgülle içerir — BE her ikisini de geçerli audience olarak kabul eder:

| Client | ID | Kullanım |
|---|---|---|
| Android debug | `620640306728-mlgnptjcl2qsoh6seha3p100r73pruol` | Debug build + emülatör |
| Android release | `620640306728-et295k0iid88m0u6o6sk91fjivvkukrj` | Play Store build |
| Web / BE | `620640306728-k6rggqmdjlkh7bb241hjpbd753q6fdeo` | `serverClientId` — clientların kullandığı |

Android ve iOS clientlarda `serverClientId` olarak **Web client ID** kullanılır; bu, Google'ın token audience'ını belirler ve BE tarafında doğrulanan değerdir.

### SSE akışı

Authenticate olmuş client `GET /v1/events` açar (`Authorization: Bearer <access_token>`). BE, o kullanıcı ID'sine yayın yapılan event'leri iletir:

```
data: {"type":"connected","payload":{}}

data: {"type":"heartbeat","payload":{}}          ← 25s'de bir

data: {"type":"user.updated","payload":{...user}} ← PATCH /v1/me sonrası
```

- **Android** — OkHttp SSE (`SseClient.kt`), `callbackFlow` ile Flow'a dönüştürülür
- **iOS** — URLSession bytes API (`SSEClient.swift`), async/await

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
