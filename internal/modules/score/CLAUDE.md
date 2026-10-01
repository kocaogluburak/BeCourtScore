# Domain: score — matches + history

> Focused notes for the `score` module. See `../../../CLAUDE.md` for the whole backend.

Stores finished match results **and** in-progress live shared scoreboards.
Also exposes player identity aggregates (career stats + head-to-head).

## Files

| File | Role |
|------|------|
| `routes.go` | `/v1/matches/*`, `/v1/live-matches/*`, `/v1/users/{userID}/matches|stats|h2h` (all Bearer) |
| `handler.go` | Finished-match + stats handlers, DTOs, pagination |
| `live_handler.go` | Live start/get/update/end/list/cancel/accept/decline |
| `service.go` | History rules + live state machine + stats gates; `FriendChecker`; optional `PushSender` |
| `stats.go` | Pure aggregation: W/L, set diff, form, rivals, H2H |
| `repo.go` / `live_repo.go` | Partitioned `matches` + live table |
| `*_test.go` | handler, live_handler, service, stats |

## History routes

```
POST   /v1/matches             → record a finished match (201)
GET    /v1/matches             → own history, paginated; ?q= name search, ?sport= filter
GET    /v1/matches/{id}        → detail (participant, recorder, or accepted friend)
DELETE /v1/matches/{id}        → creator only (204)
GET    /v1/users/{id}/matches  → a friend's history (403 unless self / accepted friend)
GET    /v1/users/{id}/stats    → career W/L + form + rivals (?sport= optional)
GET    /v1/users/{id}/h2h      → viewer vs target H2H (?sport=); self → 400
```

## Stats rules

- Count only when the target is a **registered participant** (`player_a_user_id` / `player_b_user_id`).
  Recorder-only rows (`created_by` alone) are ignored.
- Visibility matches friend history: **self or accepted friend** → else 403.
- `form`: last 5 outcomes from the subject's perspective (`W`/`L`), newest first.
- `rivals`: top 5 registered opponents by match count (guest opponents excluded).
- `by_sport`: always returned (all sports with ≥1 participant match), even when `?sport=` filters totals/form/rivals.
- H2H numbers are from the **viewer's** perspective. Zero matches → 200 with zeros (not 404).
- Set differential uses `sets_a` / `sets_b` (not per-game `set_scores`).

## Live routes

```
POST   /v1/live-matches              → start; registered opponent → PENDING + invite, else IN_PROGRESS
GET    /v1/live-matches              → open sessions, paginated
GET    /v1/live-matches/{id}         → creator or registered participant
PATCH  /v1/live-matches/{id}         → creator score snapshot
POST   /v1/live-matches/{id}/end     → archive to history
POST   /v1/live-matches/{id}/cancel  → creator (PENDING|IN_PROGRESS) or registered opponent (IN_PROGRESS only)
POST   /v1/live-matches/{id}/accept  → addressee only
POST   /v1/live-matches/{id}/decline
```

Friend vs friend: duplicate open live → 409. SSE `match.invite` / `match.started` / `match.score_updated`. FCM via `notify`.
Open sessions with `updated_at` older than 24h are lazily expired (ENDED, no history) on list/start/get.

## Match body

`sport` (TENNIS|PADEL|SQUASH|PING_PONG), `player_a_name`, `player_b_name`,
optional `player_a_user_id` / `player_b_user_id` (guests stay name-only),
`sets_a`, `sets_b`, `winner_side` (A|B), optional `played_at` (RFC3339, default now).

## Data model / performance

`matches` is **RANGE-partitioned by month on `played_at`** (migration 003: 3 months back →
24 months ahead + DEFAULT catch-all — extend periodically). Parent-level indexes propagate to
partitions: `(created_by, played_at DESC)`, partial on `player_a_user_id`/`player_b_user_id`,
`(sport, played_at DESC)`, and H2H pair index (migration 014:
`LEAST/GREATEST(player_a_user_id, player_b_user_id), played_at DESC` where both IDs are set).

Stats load participant-scoped rows then aggregate in Go (`stats.go`).

## Gotchas

- Cross-module friend visibility goes through the small `FriendChecker` interface — do **not**
  import `social` directly; keep the dependency inverted.
- Every list endpoint MUST use `httpx.ParsePage` / `httpx.NewPage` (default 20, max 100).
- Adding a partition range → new migration under `internal/migrate/sql/`.

## After changes

`cd BeCourtScore && go test ./internal/modules/score/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
