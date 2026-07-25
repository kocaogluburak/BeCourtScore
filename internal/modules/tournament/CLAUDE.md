# Domain: tournament — brackets, draws, results

> Focused notes for the `tournament` module. See `../../../CLAUDE.md` for the whole backend.
> NOTE: the root doc still calls this a "stub" — it is **fully implemented**. Update the root when convenient.

Single-elimination and round-robin tournaments: create, join by code, lock, draw,
live bracket, and result entry with opponent confirmation. Live updates over SSE.

## Files

| File | Role |
|------|------|
| `routes.go` | Mounts `/v1/tournaments/*` (all Bearer) |
| `handler.go` | Handlers, DTOs, validation |
| `service.go` | State machine + view types (`Bracket`, `Standing`), slug/join-code generation, SSE publishing |
| `bracket.go` | Pure draw builders: `buildSingleElim` (BYE handling) + `buildRoundRobin` (circle method) |
| `repo.go` | Postgres: tournaments, participants, fixtures/matches |
| `*_test.go` | `service_test.go`, `bracket_test.go`, `handler_test.go` |

## Routes

```
POST   /v1/tournaments                         → create (name, sport, format, max_participants)
GET    /v1/tournaments/mine                     → organizer/participant list, paginated
GET    /v1/tournaments/{ref}                     → detail (ref = slug); adds is_organizer/has_joined
DELETE /v1/tournaments/{ref}                     → organizer only, only in REGISTRATION|LOCKED
POST   /v1/tournaments/{ref}/join                → {join code} → participant
GET    /v1/tournaments/{ref}/participants        → paginated
DELETE /v1/tournaments/{ref}/participants/me     → leave (REGISTRATION only)
POST   /v1/tournaments/{ref}/lock                → organizer; needs ≥2 participants
POST   /v1/tournaments/{ref}/draw                → organizer; mode RANDOM|MANUAL(+seeding) → bracket
GET    /v1/tournaments/{ref}/bracket             → rounds (+ standings for round-robin)
POST   /v1/tournaments/matches/{matchId}/result  → organizer completes, player reports
POST   /v1/tournaments/matches/{matchId}/confirm → opponent/organizer approve|reject
```

## State machine

`REGISTRATION → LOCKED → ONGOING → (COMPLETED)`

- **Draw** only from `LOCKED`; sets status to `ONGOING`.
- **Result**: organizer submit = immediate `completeMatch` + advance; a player submit = `reportMatch`
  → match becomes `PENDING_CONFIRMATION`, opponent must `confirm`. A player can't confirm their own report.
- **Advance**: single-elim moves the winner into `next_match`; the final (no next match) sets the champion.
  Round-robin completes when no fixtures remain; champion = top of computed standings.

## Formats (`validFormats`)

- `SINGLE_ELIM` — `buildSingleElim`: real matches first, BYEs to the tail, BYE winners auto-advance.
- `ROUND_ROBIN` — `buildRoundRobin`: circle method; standings sorted by wins then set-diff (`parseScore`).

Valid sports: TENNIS, PADEL, SQUASH, PING_PONG. `max_participants` default 32, range 2..128.

## SSE events (to all participants unless noted)

`tournament.participant_joined`, `tournament.locked`, `tournament.draw_completed`,
`tournament.match_completed`, `tournament.completed` (+champion),
`tournament.match_pending_confirmation` (→ opponent), `tournament.match_disputed` (→ organizer),
`tournament.deleted`.

## Gotchas

- Keep draw builders **pure** (no DB) in `bracket.go` so `bracket_test.go` can verify them directly.
- `store` interface is stubbed in tests — add a method there AND in the stub when extending.
- Join codes use a confusion-free alphabet (no O/0/I/1); slugs strip Turkish chars via `trReplacer`.

## After changes

`cd BeCourtScore && go test ./internal/modules/tournament/...` then `go test ./...` (15 tests must pass).
