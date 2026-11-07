# Domain: social — friendships, user search, private groups

> Focused notes for the `social` module. See `../../../CLAUDE.md` for the whole backend.

Handles user search, public profiles, friend requests, the friend list, invite-code groups,
and the friend activity feed (kudos + short comments).
`social.Service` also implements `score.FriendChecker` (wired in `main.go`).
`social.FeedService` implements `score.ActivitySink` via a thin adapter in `cmd/api/main.go`.

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts `/v1/friends/*` and `/v1/users/*` (all Bearer) |
| `handler.go` | Handlers, DTOs, validation, pagination |
| `service.go` | Friendship rules; also exposes `IsFriend` for the score module |
| `repo.go` | Postgres access to `friendships` |
| `handler_test.go` | Handler tests |
| `groups.go`, `ladder.go`, `groups_repo.go`, `groups_handler.go` | Invite-code groups and the weekly ladder |
| `seasons.go`, `seasons_repo.go`, `seasons_handler.go`, `season_job.go` | Opt-in 6–8 week leagues + promo/relegation |
| `feed.go`, `feed_repo.go`, `feed_handler.go`, `feed_routes.go` | Friend activity feed (pull-on-read + SSE) |
| `feed_test.go`, `feed_handler_test.go` | FEED-01…06 + HTTP envelope |

## Routes

```
GET    /v1/users/search?q=              → by nickname/name prefix or exact email (min 2 chars)
GET    /v1/users/{userID}               → public profile (self or accepted friend; no email)
GET    /v1/friends                      → accepted friends, paginated
DELETE /v1/friends/{userID}             → unfriend (204)
GET    /v1/friends/requests             → incoming pending requests, paginated
POST   /v1/friends/requests             → {"user_id": "..."} send request (409 if pending/friends)
POST   /v1/friends/requests/{id}/accept → addressee only
POST   /v1/friends/requests/{id}/reject → addressee only
GET    /v1/groups                       → caller's groups
POST   /v1/groups                       → {"name","sport"} create; caller is owner
POST   /v1/groups/join                  → {"code"} join
GET    /v1/groups/by-code/{code}        → members only: members + this ISO week
GET    /v1/groups/{id}                  → members only: members + this ISO week
DELETE /v1/groups/{id}                  → owner only
DELETE /v1/groups/{id}/membership       → leave; owner must delete instead
GET    /v1/groups/{id}/seasons          → members: current + past seasons, paginated
GET    /v1/groups/{id}/seasons/current  → members: active season + live standings (404 if weekly-only)
GET    /v1/groups/{id}/seasons/{seasonId} → members: live if ACTIVE; COMPLETED is Pro archive
POST   /v1/groups/{id}/seasons          → owner: start season {weeks,division_count,count_social_nights}; 8+ Pro
POST   /v1/groups/{id}/seasons/{seasonId}/finalize → owner: manual rollover

GET    /v1/feed                         → friends' activities, paginated
POST   /v1/feed/{id}/reactions          → {"type":"kudos"} idempotent (201)
DELETE /v1/feed/{id}/reactions          → remove caller's kudos (204)
GET    /v1/feed/{id}/comments           → paginated, oldest→newest
POST   /v1/feed/{id}/comments           → {"body"} 1–280 chars (201)
DELETE /v1/feed/{id}/comments/{cid}     → comment author or activity author (204)
GET    /v1/me/feed-settings             → {feed_opt_out}
PATCH  /v1/me/feed-settings             → {feed_opt_out}
```

## Weekly ladder

- Tables `groups` and `group_members`. No match table of its own.
- Standings read finished `matches` whose sport is the group's sport and whose `played_at` falls in the current ISO week (Monday 00:00 UTC inclusive, next Monday exclusive), plus finished Americano / Mexicano / King of the Court nights in that window.
- A match counts when each side has at least one registered player and every registered player on both sides is a current member. A guest or a non-member drops the match from the ladder; it still stays on the registered player's career. Doubles award the win, the loss, and the set difference to every registered player on that side.
- Match points are a win 3 and a loss 1, the same constants as a season. A finished social night adds placement points (1st 3, 2nd 2, anyone who played 1) for current members. A night counts when `group_id` is this group, or when `group_id` is empty and every participant is a current member. Rank is points, then set difference, then wins. Elo is separate and is not used here.
- The weekly board always counts social nights. A season's `count_social_nights` switch is unchanged.
- Deleting a match removes it from the ladder because the ladder reads matches live. Invite-code collisions are retried; a non-owner cannot delete the group.

## Friendship model

- **One row per pair**, `status ∈ pending|accepted|rejected` (a rejected pair can be re-requested).
- Unique index on the unordered pair via `LEAST/GREATEST`; partial indexes for accepted/pending.
- Search results carry `friendship_status`: `none|pending_sent|pending_received|accepted`.

## SSE events (via `platform/sse` hub)

- `friend.request_received` → addressee
- `friend.request_accepted` → requester
- `friend.removed` → the removed side
- `feed.activity_created` → each accepted friend of the actor
- `feed.reaction_added` → activity author + actor's friends
- `feed.comment_added` → activity author + actor's friends
- `season.started` / `season.standings_updated` / `season.completed` → each season member

## Gotchas

- Never leak email in `/v1/users/{id}` unless it's the caller themselves.
- Keep the `LEAST/GREATEST` unordered-pair invariant on every write, or the unique index breaks.
- `IsFriend`/`FriendChecker` is consumed by `score` — keep its signature stable.

## After changes

`cd BeCourtScore && go test ./internal/modules/social/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
