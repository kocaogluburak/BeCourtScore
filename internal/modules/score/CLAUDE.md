# Domain: score — matches + history

> Focused notes for the `score` module. See `../../../CLAUDE.md` for the whole backend.

Stores finished match results **and** in-progress live shared scoreboards.
Also exposes player identity aggregates (career stats + head-to-head).

## Files

| File | Role |
|------|------|
| `routes.go` | `/v1/matches/*`, `/v1/live-matches/*`, `/v1/rematches/*`, `/v1/users/{userID}/matches|stats|h2h` (all Bearer) |
| `handler.go` | Finished-match + stats handlers, DTOs, pagination |
| `live_handler.go` | Live start/get/update/end/list/cancel/accept/decline |
| `rematch_handler.go` / `rematch.go` / `rematch_repo.go` | Friend rematch challenges (pending inbox + accept prefill) |
| `service.go` | History rules + live state machine + stats gates; `FriendChecker`; `UserLookup`; optional `PushSender` / `AchievementHook` / `ActivitySink` / `SeasonNotifier` / `UsageAccruer` (sport-scoped) / `EquipmentMatchSink` |
| `stats.go` | Pure aggregation: W/L, set diff, form, rivals, H2H |
| `wrapped.go` / `wrapped_handler.go` | Year-in-review: `GET /v1/me/wrapped` + public share token |
| `wrapped_share.go` | `POST /v1/me/wrapped/share`, `GET /v1/public/wrapped/{token}` |
| `elo_backfill.go` | One-off `BackfillRatingHistory` (`cmd/backfill-elo`) |
| `repo.go` / `live_repo.go` | Partitioned `matches` + live table |
| `*_test.go` | handler, live_handler, rematch, service, stats |

## History routes

```
POST   /v1/matches             → record a finished match (201)
GET    /v1/matches             → own history, paginated; ?q= name search, ?sport= filter
GET    /v1/matches/{id}        → detail (participant, recorder, or accepted friend)
GET    /v1/matches/{id}/stats  → derived point stats (Pro via AllowAdvancedStats)
DELETE /v1/matches/{id}        → creator only (204)
GET    /v1/users/{id}/matches  → a friend's history (403 unless self / accepted friend)
GET    /v1/users/{id}/stats    → career W/L + form + rivals (?sport= optional)
GET    /v1/users/{id}/h2h      → viewer vs target H2H (?sport=); self → 400
GET    /v1/users/{id}/fitness-trend → per-match HR/kcal (Pro via AllowStats; self/friend)
GET    /v1/me/wrapped?year=YYYY    → year-in-review (self, FREE — no AllowStats)
POST   /v1/me/wrapped/share?year=YYYY → mint public token `{token,url,year}` (self, FREE)
GET    /v1/public/wrapped/{token}  → frozen snapshot (no auth, FREE)
```

## Stats rules

- Count only when the target is a **registered participant** (`player_a_user_id` / `player_b_user_id` / `player_a2_user_id` / `player_b2_user_id`).
  Recorder-only rows (`created_by` alone) are ignored.
- Visibility matches friend history: **self or accepted friend** → else 403.
- `form`: last 5 outcomes from the subject's perspective (`W`/`L`), newest first.
- `rivals`: top 5 registered opponents by match count (guest opponents excluded).
- `by_sport`: always returned (all sports with ≥1 participant match), even when `?sport=` filters totals/form/rivals.
- `rating`: singles Elo for the filtered sport on the top-level payload (starting value 1500). `doubles_rating` is the separate player doubles Elo for that sport, also starting at 1500. Each `by_sport` row carries both. Singles moves only for a finished match where both sides are exactly one registered user. Doubles moves only when all four slots are distinct registered users: team strength is the average of the pair, and each player uses their own K. A guest name skips doubles Elo but the registered player's win/loss stays on the career card. Tournament social rounds are not history rows; when an Americano, Mexicano, or King of the Court night finishes, each completed 2v2 with four registered players updates doubles Elo once (`tournament_match_id` on `user_rating_history`). `social_nights` and `social_best_rank` count those finished nights for the player. Deleting a match does not rewind either rating. Each rated match also appends `user_rating_history` (migration 027, discipline column in 034) so Wrapped can report a singles year peak (`discipline = SINGLES`); `elo_year_end` from singles `user_ratings` is the fallback when history is empty. Rows from before 027 are filled by `go run ./cmd/backfill-elo`, which also replays doubles matches and finished social rounds into DOUBLES ratings without rewriting SINGLES.
- Wrapped year window is UTC `[Y-01-01, Y+1-01-01)`. `has_enough_data` is false below 5 participant matches. Year must be 2023..current calendar year.
- H2H numbers are from the **viewer's** perspective. Zero matches → 200 with zeros (not 404). `social_rounds` / `social_wins` / `social_losses` count completed Americano, Mexicano, and King of the Court rounds where the two users were on opposite sides. Those point scores are not added to set differential.
- Set differential uses `sets_a` / `sets_b` (not per-game `set_scores`).

## Live routes

```
POST   /v1/live-matches              → start; any registered player except starter → PENDING + invite each, else IN_PROGRESS
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

## Rematch routes

```
POST   /v1/rematches                 → create challenge {opponent_id, sport, source_match_id?, rules...} (201)
GET    /v1/rematches/incoming        → pending challenges addressed to me (paginated)
GET    /v1/rematches/outgoing        → my sent pending challenges (paginated)
POST   /v1/rematches/{id}/accept     → opponent only; pending→accepted; returns RematchPrefill (option A)
POST   /v1/rematches/{id}/reject     → opponent only; pending→rejected
POST   /v1/rematches/{id}/cancel     → challenger only; pending→cancelled
```

Accepted friend required. Duplicate open pending per directed pair+sport → 409.
Pending older than 48h expire lazily on list/create (no push). Accept is prefill-only
(clients open Match Setup; they start the match through the normal live/local flow).

SSE: `rematch.received` (opponent), `rematch.accepted` (challenger, prefill payload),
`rematch.rejected` (challenger), `rematch.cancelled` (opponent). FCM via `notify`.

## Match body

`sport` (TENNIS|PADEL|SQUASH|PING_PONG), `player_a_name`, `player_b_name`,
optional `player_a_user_id` / `player_b_user_id` and doubles partners
`player_a2_name` / `player_b2_name` / `player_a2_user_id` / `player_b2_user_id`
(guests stay name-only),
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
