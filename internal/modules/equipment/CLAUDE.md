# Domain: equipment — racket string + shoes

> Focused notes for the `equipment` module. See `../../../CLAUDE.md` for the whole backend.

Pro users register physical gear. Finished-match `duration_seconds` accrues onto the
owner's **active** item of each type. Crossing `lifespan_hours` fires a one-shot
push (`notified_at`); reset / raised lifespan / activate clears it.

## Files

| File | Role |
|------|------|
| `routes.go` | `/v1/me/equipment/*` (all Bearer, Pro-gated) |
| `handler.go` | CRUD + activate/reset; `httpx.Page` list |
| `service.go` | validation, usage math, `score.UsageAccruer`, sweep |
| `repo.go` | Postgres `equipment` + accrual UPDATE + activate tx |
| `*_test.go` | accrual, CRUD/gate, handler routes |

## Routes

```
GET    /v1/me/equipment              list (paginated)
POST   /v1/me/equipment              create {type,name,lifespan_hours,purchase_date?}
GET    /v1/me/equipment/{id}         usage view
PATCH  /v1/me/equipment/{id}         name / lifespan / purchase_date / is_active
DELETE /v1/me/equipment/{id}         204
POST   /v1/me/equipment/{id}/activate
POST   /v1/me/equipment/{id}/reset
```

Types: `STRING` | `SHOES`. First item of a type is active; extras stay inactive until
`activate` (transaction: deactivate others of that type, then activate + clear `notified_at`).
Foreign ids → 404. Non-Pro → 402 `pro_required` via `AllowEquipment` (IsPro only).

## Accrual

`equipment.Service` implements `score.UsageAccruer`. Wired in `cmd/api/main.go` via
`scoreSvc.SetAccruer(equipmentSvc)`. Called after `CreateMatch` / `EndLiveMatch`
insert — recorder (`created_by`) only, best-effort. Hourly `SweepReminders` is the
backstop. Push data: `type=equipment.reminder`, `equipment_id`, `equipment_type`, `hours`.

## After changes

`cd BeCourtScore && go test ./internal/modules/equipment/...` then `go test ./...`.
Inventory: `BeCourtScore/test/test.md`.
