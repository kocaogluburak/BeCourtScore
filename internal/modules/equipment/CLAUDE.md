# Domain: equipment — sport-scoped gear bag ("My Bag")

> Focused notes for the `equipment` module. See `../../../CLAUDE.md` for the whole backend.
> Plan: `plans/gear-bag/02-technical.md`.

Every signed-in user can keep gear (no Pro gate on CRUD; the **free limit** is 2 *live* rows,
Pro is unlimited — `isPro` via `entitlement.Gate.IsPro`, create-only 402 `pro_required`).
Gear belongs to a **sport** (`TENNIS|PADEL|SQUASH|PING_PONG|PICKLEBALL`) and a catalog
**category** (`RACKET`, `STRING`, `SHOES`, `OVERGRIP`, `PALA`, `PADDLE`, `BLADE`, `RUBBER_FH/BH`).
Finished-match `duration_seconds` accrues onto the recorder's **active, non-retired** item of
**every** category of the match's sport at once. Crossing the category threshold fires a
one-shot data-only push (`notified_at`); reset / raised lifespan / activate clears it.

## Files

| File | Role |
|------|------|
| `routes.go` | `/v1/me/equipment/*` + `GET /v1/equipment/catalog` (all Bearer) |
| `handler.go` | CRUD, activate/reset/archive/renew, catalog, win-rate; `httpx.Page` list |
| `service.go` | validation, item view (`display_name`, legacy `type`, usage math), free limit, accrual, reminders |
| `catalog.go` | catalog tree (`categories → brands → models`) read model + query |
| `winrate.go` | `EquipmentMatchSink` (`OnMatchRecorded/OnMatchDeleted`), `comboKey`, win-rate (`MinWinRateSample=5`) |
| `repo.go` | Postgres `equipment` (joined with catalog), accrual UPDATE, activate/renew tx, usage rows |
| `*_test.go` | `store_mem_test.go` is the shared in-memory store + fixture catalog |

## Routes

```
GET    /v1/equipment/catalog?sport=       catalog tree (one object with sport, else array of 5; not paginated)
GET    /v1/me/equipment?view=active|archive&sport=&category_id=   paginated, default view=active
POST   /v1/me/equipment                   {sport,category_id,brand_id|custom_brand_name,model_id?,custom_model_name?,
                                           nickname?,lifespan_hours?,lifespan_days?,purchase_date?,attributes?,parent_item_id?}
GET    /v1/me/equipment/{id}
PATCH  /v1/me/equipment/{id}              nickname (legacy `name` alias) / brand+model / lifespan / purchase_date /
                                          attributes / parent_item_id / is_active
DELETE /v1/me/equipment/{id}              204 (permanent; archive is preferred)
POST   /v1/me/equipment/{id}/activate     single active per category (tx swap)
POST   /v1/me/equipment/{id}/reset
POST   /v1/me/equipment/{id}/archive      retire by hand, no replacement (409 if already archived)
POST   /v1/me/equipment/{id}/renew        restring/regrip: retire + new generation, same lineage_id (201; body optional)
GET    /v1/me/equipment/win-rate?sport=   {enough_data,matches_played,matches_won,win_percent}; static path beats {id}
```

Foreign ids → 404. Old-client create body (no `category_id`) → 400.

## Rules worth remembering

- Lifespan by category `threshold_type`: `NONE` → no lifespan; `HOURS` → `lifespan_hours > 0` required;
  `HOURS_OR_DAYS` → at least one of hours/days. Items still accrue hours when `NONE` (never shown/reminded).
- Free limit counts `retired_at IS NULL` rows; archive frees a slot, renew is net-zero.
- Renew: old row `retired_at=NOW(), is_active=FALSE`; new row copies brand/model/attributes/lifespans/parent
  (empty renew body) or overrides them; usage, purchase date, nickname, `notified_at` start fresh; the new row
  inherits the old row's active flag. `renewals_this_year` is read-time per `lineage_id`.
- `type` in the item view is derived (Tennis `STRING`/`SHOES` only); `name` mirrors `display_name` for old clients.
- `display_name` = nickname ?? brand+model ?? legacy `name` ?? `—`.
- Reminders: hours threshold, or (HOURS_OR_DAYS) `COALESCE(purchase_date, created_at::date) + lifespan_days <= today`.
  Push data: `type=equipment.reminder`, `equipment_id`, `equipment_type` (category code), `sport`, `hours`, `days`.
- Win-rate combo key = sorted `category:brand:model` parts (ids, or `CUSTOM:<lowercased name>`), so a
  restring with the same brand/model keeps its sample. Only the recorder (`created_by`) who played the match is tracked,
  forward-only; `match_equipment_usage.match_id` is a soft reference (partitioned `matches`).
- Catalog is ops-editable SQL (`036_equipment_catalog.sql`); no admin UI.

## Score hooks

`equipment.Service` implements `score.UsageAccruer` (`AccrueMatchDuration(ctx,userID,sport,seconds)`) and
`score.EquipmentMatchSink`. Wired in `cmd/api/main.go` via `scoreSvc.SetAccruer(equipmentSvc)` and
`scoreSvc.SetEquipmentMatchSink(equipmentSvc)`. Called after `CreateMatch` / `EndLiveMatch` (recorder only,
best-effort + recover); `DeleteMatch` calls `OnMatchDeleted`. Hourly `SweepReminders` is the backstop.

## After changes

`cd BeCourtScore && go test ./internal/modules/equipment/... ./internal/migrate/...` then `go test ./...`.
Inventory: `BeCourtScore/test/test.md`.
