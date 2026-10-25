# Domain: invite — tracked invite links

> Focused notes for the `invite` module. See `../../../CLAUDE.md` for the whole backend.

Mints short URL-safe codes for share cards / rematch / profile, records clicks
and signup attribution. Sharing is **free** (not routed through the live Pro gate).

## Files

| File | Role |
|------|------|
| `routes.go` | Bearer `POST /v1/invites` + public `GET /v1/public/invite/{code}` |
| `handler.go` | Mint + public resolve DTOs |
| `service.go` | Code mint (base32), `shareURL`, `RecordSignup` (best-effort) |
| `repo.go` | `invite_shares` + `users.invited_via_code` |

## Routes

```
POST   /v1/invites                  → {source, sport?, source_match_id?} → {code, url} (201)
GET    /v1/public/invite/{code}     → {inviter, sport?, url} + increments click_count
```

`url` = `{publicWebOrigin}/i/{code}` (default `https://court-score.com`).
Clients render QR locally from `url`.

`source` ∈ `share_card|rematch|profile|generic`.

## Attribution

`identity` auth accepts optional `invite_code`. On `is_new_user`, it calls
`invite.RecordSignup` (wired in `cmd/api/main.go`). Invalid codes never fail signup.

Primary metric: `signup_count / click_count` per row.

## After changes

`cd BeCourtScore && go test ./internal/modules/invite/...` then `go test ./...`.
