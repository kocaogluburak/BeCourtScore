# Platform — shared infrastructure

> Focused notes for `internal/platform`. See `../../CLAUDE.md` for the whole backend.

Cross-cutting infrastructure shared by every domain module. No business logic lives here.

## Packages

| Package | Role |
|---------|------|
| `config/` | Env config loading (`PORT`, `DATABASE_URL`, `JWT_SECRET`, `GOOGLE_CLIENT_IDS`, `CORS_ORIGINS`) |
| `db/` | `pgxpool` construction + lifecycle |
| `httpx/` | JSON response helpers (`respond.go`) + pagination (`ParsePage`, `Page[T]`, `NewPage`) |
| `authkit/` | JWT sign/verify (`jwt.go`) + Bearer middleware (`middleware.go`) |
| `sse/` | Per-user SSE hub (`hub.go`): `Publish(userID, Event)` + `Handler()` |

## Pagination contract (mandatory everywhere)

Query: `?page=1&page_size=20` — **default 20, max 100** (`httpx.ParsePage`).
Envelope (`httpx.Page[T]`):

```json
{"items": [...], "page": 1, "page_size": 20, "total": 137, "has_more": true}
```

Never return unbounded arrays from a list endpoint.

## SSE hub

`sse.Hub` fans events out per authenticated user. Domain services take `*sse.Hub` and call
`Publish(userID, sse.Event{Type, Data})`. The `/v1/events` route is mounted by `identity`.
Standard events besides domain ones: `connected`, `heartbeat` (~25s).

## Gotchas

- Changing the `Page[T]` envelope is a breaking change for all three clients — coordinate.
- `authkit.Middleware(JWTSecret)` injects the user ID into the request context; handlers read it there.
- Keep this layer dependency-free of `modules/*` (infra must not import domains).

## After changes

`cd BeCourtScore && go test ./internal/platform/...` then `go test ./...`. Inventory: `BeCourtScore/test/test.md`.
