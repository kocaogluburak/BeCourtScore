# CourtScore Backend — Agent Notes

Go modular monolith. Single binary, PostgreSQL, Google Sign-In (v1), JWT sessions, SSE.

## Focused domain docs (read the one you're touching)

When working inside a single domain, open its nested `CLAUDE.md` first — it has the routes,
data model, and gotchas for just that area, so you can focus without loading the whole backend.

| Domain | Focused doc |
|--------|-------------|
| Auth + profile + SSE mount | `internal/modules/identity/CLAUDE.md` |
| Matches + history | `internal/modules/score/CLAUDE.md` |
| Friendships + user search | `internal/modules/social/CLAUDE.md` |
| Tournaments (brackets/draws) | `internal/modules/tournament/CLAUDE.md` |
| Shared infra (authkit/sse/httpx/db/config) | `internal/platform/CLAUDE.md` |

This root file stays the source of truth for cross-cutting concerns (deploy, env, CORS, pagination).

Base URLs:

| Environment | URL |
|-------------|-----|
| Local | `http://localhost:8080` |
| Production | `https://api.court-score.com` |

---

## How clients talk to the BE

Clients (Android / iOS / web) never talk to Postgres. They only call the HTTP API.

```
Mobile / Web client
    │
    │  HTTPS  (prod)  or  HTTP localhost:8080 (dev)
    ▼
Cloudflare  (api.court-score.com — proxied)
    │
    ▼
Caddy on GCP VM  (:443 / :80 → reverse_proxy)
    │
    ▼
Go API container  (127.0.0.1:8080)
    │
    ▼
Postgres container  (127.0.0.1:5432, not public)
```

### Auth flow (Google)

1. Client signs in with Google SDK → gets a Google `id_token`.
2. Client → `POST /v1/auth/google` with `{"id_token":"..."}`.
3. BE verifies the token against `GOOGLE_CLIENT_IDS`, upserts `users` + `auth_identities`, issues:
   - `access_token` (JWT, short-lived ~15m)
   - `refresh_token` (stored hashed in `refresh_tokens`)
4. Client sends `Authorization: Bearer <access_token>` on protected routes.
5. When access expires → `POST /v1/auth/refresh` with refresh token → new pair.
6. Logout → `POST /v1/auth/logout` (Bearer + refresh body) revokes refresh token.

### Typical client session

```
POST /v1/auth/google     → tokens + user (+ is_new_user)
GET  /v1/me              → profile (Bearer)
PATCH /v1/me             → update profile (Bearer) → may push SSE user.updated
GET  /v1/events          → SSE stream (Bearer): connected, heartbeat, user.updated, friend.*
POST /v1/auth/refresh    → rotate tokens
POST /v1/auth/logout     → revoke
GET  /health             → liveness (no auth)
```

### Matches API (score module, Bearer required)

```
POST   /v1/matches               → record a finished match (201)
GET    /v1/matches               → own history, paginated; ?q= name search, ?sport= filter
GET    /v1/matches/{id}          → detail (participant, recorder, or their accepted friend)
DELETE /v1/matches/{id}          → only the creator (204)
GET    /v1/users/{id}/matches    → a friend's history (403 unless self/accepted friend)
```

Match body: `sport` (TENNIS|PADEL|SQUASH|PING_PONG), `player_a_name`, `player_b_name`,
optional `player_a_user_id` / `player_b_user_id` (registered users; guests stay name-only),
`sets_a`, `sets_b`, `winner_side` (A|B), optional `played_at` (RFC3339, defaults to now).

### Friends API (social module, Bearer required)

```
GET    /v1/users/search?q=              → users by nickname/name prefix or exact email (min 2 chars)
GET    /v1/users/{userID}               → public profile (self or accepted friend; no email)
GET    /v1/friends                      → accepted friends, paginated
DELETE /v1/friends/{userID}             → unfriend (204)
GET    /v1/friends/requests             → incoming pending requests, paginated
POST   /v1/friends/requests             → {"user_id": "..."} send request (409 if pending/friends)
POST   /v1/friends/requests/{id}/accept → addressee only
POST   /v1/friends/requests/{id}/reject → addressee only
```

Friendship model: single row per pair, `status ∈ pending|accepted|rejected` (a rejected pair can
be re-requested). Search results carry `friendship_status`: `none|pending_sent|pending_received|accepted`.
SSE pushes: `friend.request_received` (to addressee), `friend.request_accepted` (to requester),
`friend.removed` (to the removed side).

### Pagination (mandatory for every list endpoint)

- Query params: `?page=1&page_size=20` — **default page_size 20, max 100** (`httpx.ParsePage`).
- Response envelope (`httpx.Page[T]`):

```json
{"items": [...], "page": 1, "page_size": 20, "total": 137, "has_more": true}
```

Any new list endpoint MUST use this helper and envelope — never return unbounded arrays.

Mobile clients usually have no browser Origin; CORS mainly matters for web. `CORS_ORIGINS=*` is fine for API-only mobile; tighten for browser frontends later.

---

## Architecture (code)

```
cmd/api/                 composition root
internal/
  platform/
    config/              env config
    db/                  pgxpool
    httpx/               JSON helpers + pagination (ParsePage, Page[T])
    authkit/             JWT + Bearer middleware
    sse/                 per-user SSE hub
  modules/
    identity/            auth + profile (implemented)
    score/               matches + history → /v1/matches/*, /v1/users/{id}/matches
    social/              friendships + user search → /v1/friends/*, /v1/users/*
    tournament/          stub → /v1/tournaments/*
  migrate/               embedded SQL migrations
  httpapi/               chi router, CORS, mounts modules
```

Conventions:

- New auth provider → implement `identity.IdentityProvider`, register in `cmd/api/main.go`.
- New domain → handlers/service/repo under `internal/modules/<name>/`, mount in `httpapi/router.go`, SQL under `internal/migrate/sql/`.
- Cross-module deps go through small interfaces (e.g. `score.FriendChecker` implemented by `social.Service`, wired in `main.go`).
- Every list endpoint is paginated via `httpx.ParsePage` / `httpx.NewPage` (default 20, max 100).
- Never commit `.env`. Use `.env.example` as the template.

### Matches table performance

`matches` is **RANGE-partitioned by month on `played_at`** (migration 003 creates 3 months back
through 24 months ahead plus a DEFAULT catch-all — extend with new partitions periodically).
Parent-level indexes propagate to all partitions: `(created_by, played_at DESC)`, partial indexes
on `player_a_user_id` / `player_b_user_id`, and `(sport, played_at DESC)`. History queries always
order by `played_at DESC` and filter by user columns, so they hit these indexes and prune
partitions by date. `friendships` has a unique index on the unordered pair (`LEAST/GREATEST`)
plus partial indexes for accepted/pending lookups.

---

## Local development

```bash
cp .env.example .env
# set JWT_SECRET, GOOGLE_CLIENT_IDS
docker compose up -d postgres          # DB only
go run ./cmd/api                       # API on :8080

# or full stack in Docker:
docker compose up --build -d
```

Local `DATABASE_URL` uses `localhost:5432`. Inside Compose, API overrides to `postgres:5432`.

---

## Production (GCP VM + Cloudflare)

### Stack on the VM

| Piece | Role |
|-------|------|
| Docker Compose | `courtscore_api` + `courtscore_db` |
| Go API | listens `:8080` inside container; published as host `8080` |
| Postgres | bound to `127.0.0.1:5432` only (not public) |
| Caddy | TLS + reverse proxy `api.court-score.com` → `127.0.0.1:8080` |
| Cloudflare | DNS + edge proxy for `api.court-score.com` |

### DNS / TLS

- Cloudflare A record: `api` → VM static IP (proxied / orange cloud).
- GCP firewall: allow `tcp:80`, `tcp:443` (and SSH). Do **not** expose Postgres publicly.
- Caddyfile (only site block needed):

```caddy
api.court-score.com {
	reverse_proxy 127.0.0.1:8080
}
```

- Prefer Cloudflare SSL mode **Full** or **Full (strict)** when Caddy has a valid cert.
- Do **not** run the Go app on `:443`; keep TLS at Caddy/Cloudflare.

### Deploy / ops commands (on VM)

```bash
cd ~/BeCourtScore   # or clone path
git pull
# ensure .env exists with prod JWT_SECRET + GOOGLE_CLIENT_IDS
docker-compose up --build -d    # older VMs may need hyphenated binary

sudo systemctl status caddy
curl -s https://api.court-score.com/health
```

Notes:

- Older Compose rejects `env_file: { path, required }` — use `env_file: [.env]`.
- `curl -I` / HEAD on `/health` returns **405** (`Allow: GET`). Use `curl -s https://api.court-score.com/health` instead.
- If Caddy fails with `address already in use` on `:80`, stop competing nginx/apache.

### DB access for debugging

Postgres is localhost-only on the VM.

```bash
# on VM
docker exec -it courtscore_db psql -U courtscore -d courtscore
# useful:
# SELECT * FROM users ORDER BY created_at DESC;
# SELECT * FROM auth_identities ORDER BY created_at DESC;
```

From a laptop, use SSH tunnel (do not open `5432` to the internet):

```bash
ssh -L 5432:127.0.0.1:5432 USER@VM_IP
# then psql / GUI → localhost:5432  user/pass/db: courtscore
```

Default Compose DB creds are `courtscore` / `courtscore` — change for long-lived prod.

---

## Env vars (summary)

| Variable | Required | Notes |
|----------|----------|--------|
| `PORT` | no | default `8080` |
| `DATABASE_URL` | yes | Compose API service overrides host to `postgres` |
| `JWT_SECRET` | yes | `openssl rand -hex 32`; different for local vs prod |
| `GOOGLE_CLIENT_IDS` | yes | Android + iOS + Web client IDs, comma-separated |
| `CORS_ORIGINS` | no | default `*` |

---

## What not to do

- Don’t put secrets in git (`.env` is gitignored).
- Don’t bind Postgres to `0.0.0.0` / public IP.
- Don’t move the Go process onto `:443`; use Caddy (or nginx) as reverse proxy.
- Don’t assume `make up-d` works on the VM — Makefile may still reference a removed Compose profile; prefer `docker compose` / `docker-compose` directly.
