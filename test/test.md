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
| `internal/modules/social/handler_test.go` | search, profile, send/accept/reject/list/unfriend |
| `internal/modules/social/service_test.go` | request state machine, reopen-after-reject, AreFriends, profile status |
| `internal/modules/score/handler_test.go` | match CRUD + friendship gate |
| `internal/modules/score/live_handler_test.go` | live match start/get/update/end/list/cancel/accept/decline + conflict |
| `internal/modules/score/service_test.go` | friendship gate, PENDING invite, guest IN_PROGRESS, duplicate open 409, accept/decline, participant list, push invite |
| `internal/modules/notify/handler_test.go` | device register + unregister |
| `internal/modules/notify/service_test.go` | Register/Unregister validation + SendToUser fan-out |
| `internal/modules/notify/fcm_test.go` | NewFCMSenderFromEnv → Noop when unset/bad path/invalid JSON |
| `internal/modules/identity/handler_test.go` | me/patch + auth/refresh/logout |
| `internal/modules/identity/service_test.go` | AuthWithProvider / Refresh / Revoke |
| `internal/modules/identity/google_test.go` | Google claims + unknown provider |
| `internal/platform/authkit/jwt_test.go` | JWT issue/parse |
| `internal/platform/authkit/middleware_test.go` | Bearer missing/invalid/valid → ctx |
| `internal/platform/sse/hub_test.go` | subscribe/publish |
| `internal/platform/httpx/pagination_test.go` | page clamp |

## Shared happy-path cases

| Case | Durum | Not |
|---|---|---|
| AUTH-01…04 | n/a (client) | — |
| TOUR-01…04 | done | service + handler |
| SCORE-01…03 | n/a (client) | score module = match history API |
| LIVE-01 | done | start live match → 201; service end→history |
| LIVE-02 | done | list open + cancel without history |
| LIVE-03 | done | friend start → PENDING + match.invite; guest → IN_PROGRESS |
| LIVE-04 | done | accept/decline; list includes opponent; duplicate open → 409 |
| HIST-01 | done | create match persists set_scores; invalid lines rejected |
| PUSH-01 | done | register + unregister device token |
| SOCIAL-01…02 | done | list/accept/reject/unfriend handler + service |
| ID-BE-01 | done | AuthWithProvider + Refresh + Revoke |

## Öncelik (2 haftalık)

1. ~~Identity Auth/Refresh/Revoke~~
2. ~~Social list/reject gap’leri~~
3. ~~Score/social service + notify unregister + auth middleware~~
4. SQL/repo yalnızca bug ısırırsa

## Katkı kuralları

- Yeni public davranış → test + bu tabloda satır güncelle
- `svcFacade` değişince `stubService` güncelle
- Suite yeşil olmadan iş bitmedi
