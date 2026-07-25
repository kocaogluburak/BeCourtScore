# Domain: social — friendships + user search

> Focused notes for the `social` module. See `../../../CLAUDE.md` for the whole backend.

Handles user search, public profiles, friend requests, and the friend list.
`social.Service` also implements `score.FriendChecker` (wired in `main.go`).

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts `/v1/friends/*` and `/v1/users/*` (all Bearer) |
| `handler.go` | Handlers, DTOs, validation, pagination |
| `service.go` | Friendship rules; also exposes `IsFriend` for the score module |
| `repo.go` | Postgres access to `friendships` |
| `handler_test.go` | Handler tests |

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
```

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

`cd BeCourtScore && go test ./internal/modules/social/...` then `go test ./...` (15 tests must pass).
