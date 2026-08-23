# Domain: score — matches + history

> Focused notes for the `score` module. See `../../../CLAUDE.md` for the whole backend.

Stores finished match results **and** in-progress live shared scoreboards.

## Files

| File | Role |
|------|------|
| `routes.go` | `/v1/matches/*`, `/v1/live-matches/*`, `/v1/users/{userID}/matches` (all Bearer) |
| `handler.go` | Finished-match handlers, DTOs, pagination |
| `live_handler.go` | Live start/get/update/end/list/cancel/accept/decline |
| `service.go` | History rules + live state machine; `FriendChecker`; optional `PushSender` |
| `repo.go` / `live_repo.go` | Partitioned `matches` + live table |
| `*_test.go` | handler, live_handler, service |

## History routes

```
POST   /v1/matches             → record a finished match (201)
GET    /v1/matches             → own history, paginated; ?q= name search, ?sport= filter
GET    /v1/matches/{id}        → detail (participant, recorder, or accepted friend)
DELETE /v1/matches/{id}        → creator only (204)
GET    /v1/users/{id}/matches  → a friend's history (403 unless self / accepted friend)
```

## Live routes

```
POST   /v1/live-matches              → start; registered opponent → PENDING + invite, else IN_PROGRESS
GET    /v1/live-matches              → open sessions, paginated
GET    /v1/live-matches/{id}         → creator or registered participant
PATCH  /v1/live-matches/{id}         → creator score snapshot
POST   /v1/live-matches/{id}/end     → archive to history
POST   /v1/live-matches/{id}/cancel
POST   /v1/live-matches/{id}/accept  → addressee only
POST   /v1/live-matches/{id}/decline
```

Friend vs friend: duplicate open live → 409. SSE `match.invite` / `match.started` / `match.score_updated`. FCM via `notify`.

## Match body

`sport` (TENNIS|PADEL|SQUASH|PING_PONG), `player_a_name`, `player_b_name`,
optional `player_a_user_id` / `player_b_user_id` (guests stay name-only),
`sets_a`, `sets_b`, `winner_side` (A|B), optional `played_at` (RFC3339, default now).

## Data model / performance

`matches` is **RANGE-partitioned by month on `played_at`** (migration 003: 3 months back →
24 months ahead + DEFAULT catch-all — extend periodically). Parent-level indexes propagate to
partitions: `(created_by, played_at DESC)`, partial on `player_a_user_id`/`player_b_user_id`,
and `(sport, played_at DESC)`. History queries order by `played_at DESC` and filter on user
columns so they prune partitions and hit indexes.

## Gotchas

- Cross-module friend visibility goes through the small `FriendChecker` interface — do **not**
  import `social` directly; keep the dependency inverted.
- Every list endpoint MUST use `httpx.ParsePage` / `httpx.NewPage` (default 20, max 100).
- Adding a partition range → new migration under `internal/migrate/sql/`.

## After changes

`cd BeCourtScore && go test ./internal/modules/score/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
