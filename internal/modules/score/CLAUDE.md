# Domain: score — matches + history

> Focused notes for the `score` module. See `../../../CLAUDE.md` for the whole backend.

Stores finished match results and serves match history (own + friends').

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts `/v1/matches/*` and `/v1/users/{userID}/matches` (all Bearer) |
| `handler.go` | Handlers, DTOs, validation, pagination wiring |
| `service.go` | Business rules; visibility checks via `FriendChecker` (implemented by `social.Service`, wired in `main.go`) |
| `repo.go` | Postgres access to the partitioned `matches` table |
| `handler_test.go` | Handler tests |

## Routes

```
POST   /v1/matches             → record a finished match (201)
GET    /v1/matches             → own history, paginated; ?q= name search, ?sport= filter
GET    /v1/matches/{id}        → detail (participant, recorder, or accepted friend)
DELETE /v1/matches/{id}        → creator only (204)
GET    /v1/users/{id}/matches  → a friend's history (403 unless self / accepted friend)
```

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

`cd BeCourtScore && go test ./internal/modules/score/...` then `go test ./...` (15 tests must pass).
