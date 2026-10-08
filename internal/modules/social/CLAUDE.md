# Domain: social — friendships, user search, private groups

> Focused notes for the `social` module. See `../../../CLAUDE.md` for the whole backend.

Handles user search, public profiles, friend requests, the friend list, and invite-code groups.
`social.Service` also implements `score.FriendChecker` (wired in `main.go`).

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts `/v1/friends/*` and `/v1/users/*` (all Bearer) |
| `handler.go` | Handlers, DTOs, validation, pagination |
| `service.go` | Friendship rules; also exposes `IsFriend` for the score module |
| `repo.go` | Postgres access to `friendships` |
| `handler_test.go` | Handler tests |
| `groups.go`, `ladder.go`, `groups_repo.go`, `groups_handler.go` | Invite-code groups and the weekly ladder |

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
```

## Weekly ladder

- Tables `groups` and `group_members`. No match table of its own.
- Standings read finished `matches` whose sport is the group's sport and whose `played_at` falls in the current ISO week (Monday 00:00 UTC inclusive, next Monday exclusive).
- A match counts when each side has at least one registered player and every registered player on both sides is a current member. A guest or a non-member drops the match. Doubles award the win and the set difference to every registered player on that side.
- Rank is wins, then set difference. Elo is separate and is not used here.
- Americano rounds live on `tournament_matches`, so they never enter the ladder.
- Deleting a match removes it from the ladder because the ladder reads matches live. Invite-code collisions are retried; a non-owner cannot delete the group.

## Friendship model

- **One row per pair**, `status ∈ pending|accepted|rejected` (a rejected pair can be re-requested).
- Unique index on the unordered pair via `LEAST/GREATEST`; partial indexes for accepted/pending.
- Search results carry `friendship_status`: `none|pending_sent|pending_received|accepted`.

## SSE events (via `platform/sse` hub)

- `friend.request_received` → addressee
- `friend.request_accepted` → requester
- `friend.removed` → the removed side

## Gotchas

- Never leak email in `/v1/users/{id}` unless it's the caller themselves.
- Keep the `LEAST/GREATEST` unordered-pair invariant on every write, or the unique index breaks.
- `IsFriend`/`FriendChecker` is consumed by `score` — keep its signature stable.

## After changes

`cd BeCourtScore && go test ./internal/modules/social/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
