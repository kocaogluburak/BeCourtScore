# BeCourtScore — Test Stratejisi

## Hedef

Solo geliştirmede backend contract’ını sabitlemek. Happy-path regress’siz ilerlemek için unit + fake/stub; gerçek DB / E2E yok.

## Katmanlar

1. Pure domain (bracket, pagination, LIKE escape)
2. Handler / service — `stubService` / fake store ile
3. Smoke / SQL integration — yalnızca ihtiyaç olursa

## Komut

```bash
cd BeCourtScore && go test ./...
# veya
make test
```

## Envanter

| Dosya | Kapsam |
|---|---|
| `internal/modules/tournament/*_test.go` | lifecycle, bracket, routes, handlers |
| `internal/modules/social/*_test.go` | search, friendship handlers |
| `internal/modules/score/handler_test.go` | match CRUD + friendship gate |
| `internal/modules/score/live_handler_test.go` | live match start/get/update/end |
| `internal/modules/notify/handler_test.go` | device register + noop FCM |
| `internal/modules/identity/handler_test.go` | me/patch + auth/refresh/logout |
| `internal/modules/identity/service_test.go` | AuthWithProvider / Refresh / Revoke |
| `internal/modules/identity/google_test.go` | Google claims + unknown provider |
| `internal/platform/authkit/jwt_test.go` | JWT issue/parse |
| `internal/platform/sse/hub_test.go` | subscribe/publish |
| `internal/platform/httpx/pagination_test.go` | page clamp |

## Shared happy-path cases

| Case | Durum | Not |
|---|---|---|
| AUTH-01…04 | n/a (client) | — |
| TOUR-01…04 | done | service + handler |
| SCORE-01…03 | n/a (client) | score module = match history API |
| LIVE-01 | done | start live match → 201 |
| PUSH-01 | done | register device token → 201 |
| SOCIAL-01…02 | partial | accept/unfriend handler; list/reject gaps |
| ID-BE-01 | done | AuthWithProvider + Refresh + Revoke |

## Öncelik (2 haftalık)

1. ~~Identity Auth/Refresh/Revoke~~ 
2. Social list/reject gap’leri (sonraki)
3. SQL/repo yalnızca bug ısırırsa

## Katkı kuralları

- Yeni public davranış → test + bu tabloda satır güncelle
- `svcFacade` değişince `stubService` güncelle
- Suite yeşil olmadan iş bitmedi
