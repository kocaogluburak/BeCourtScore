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
| `internal/modules/tournament/*_test.go` | lifecycle, bracket, Americano rotation, Mexicano re-pair, King of the Court movement, point standings |
| `internal/modules/social/handler_test.go` | search, profile, send/accept/reject/list/unfriend, group create/delete |
| `internal/modules/social/service_test.go` | request state machine, reopen-after-reject, AreFriends, profile status |
| `internal/modules/social/groups_test.go` | ISO week window, non-member match ignored, member match counted, invite-code collision, blank/long name and unknown sport rejected, non-owner cannot delete |
| `internal/modules/score/handler_test.go` | match CRUD + friendship gate + stats/h2h handlers |
| `internal/modules/score/live_handler_test.go` | live match start/get/update/end/list/cancel/accept/decline + conflict |
| `internal/modules/score/format_test.go` | set_rule / final_set defaults and unknown values |
| `internal/modules/score/pointlog_test.go` | duration, serve game, break conversion, golden break, old match omits point_log |
| `internal/modules/score/service_test.go` | friendship gate, PENDING invite, guest IN_PROGRESS, guest opponent is not invited, duplicate open 409, accept/decline, opponent cancel IN_PROGRESS, stale 24h expire, participant list, push invite, doubles invite body, non-friend partner sends nothing, stats/h2h gates |
| `internal/modules/score/players_test.go` | side label, identical names kept, invitee list skips starter/blank/duplicate, nameless partner rejected |
| `internal/modules/score/stats_test.go` | W/L form, rivals, sport filter, recorder-only ignored, H2H viewer perspective, doubles partner |
| `internal/modules/score/elo_test.go` | Elo +16 from 1500, K after 20, guest/doubles skipped, sports stay separate |
| `internal/modules/notify/handler_test.go` | device register + unregister |
| `internal/modules/notify/service_test.go` | Register/Unregister validation + SendToUser fan-out |
| `internal/modules/notify/fcm_test.go` | NewFCMSenderFromEnv → Noop when unset/bad path/invalid JSON |
| `internal/modules/identity/handler_test.go` | me/patch/delete + auth/refresh/logout |
| `internal/modules/identity/service_test.go` | AuthWithProvider / Refresh / Revoke / DeleteUser |
| `internal/modules/identity/google_test.go` | Google claims + unknown provider |
| `internal/modules/identity/apple_test.go` | Apple claims + email_verified bool/string + audience + JWK decode |
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
| SERVE-01 | done | live score patch accepts optional server_is_a |
| POINT-01 | done | point_log + duration_seconds; old match omits them and detail stats stay empty; log capped at 500 |
| LIVE-02 | done | list open + cancel without history; creator or IN_PROGRESS opponent |
| LIVE-03 | done | friend start → PENDING + match.invite; guest → IN_PROGRESS |
| LIVE-04 | done | accept/decline; list includes opponent; duplicate open → 409; doubles fan-out invite, one decline keeps PENDING |
| LIVE-05 | done | opponent cancel IN_PROGRESS; PENDING opponent cancel forbidden; 24h stale expire on list/start |
| HIST-01 | done | create match persists set_scores; invalid lines rejected |
| STATS-01 | done | user stats self/friend gate; form + rivals aggregation; recorder-only ignored; doubles partner counts, same-side is not a rival |
| H2H-01 | done | h2h self → 400; friend gate; empty 200; viewer-perspective W/L |
| PUSH-01 | done | register + unregister device token |
| SOCIAL-01…02 | done | list/accept/reject/unfriend handler + service |
| SOCIAL-03 | done | SendRequest FCM friend.request_received to addressee |
| SOCIAL-04 | done | AcceptRequest FCM friend.request_accepted to requester |
| GROUP-01 | done | group create/join/leave/delete; weekly ladder from member matches in the ISO week; non-owner cannot delete |
| ID-BE-01 | done | AuthWithProvider + Refresh + Revoke |
| ID-BE-02 | done | DELETE /v1/me hard-delete account (handler + service) |
| PRO-01 | done | entitlement snapshot: under cap / 10+ lock / active / grace / expired |
| PRO-02 | done | POST /v1/matches 402 pro_required; EndLiveMatch skips history when locked |
| PRO-03 | done | tournament create max>8 Pro; default caps to 8 when over quota |
| PRO-04 | done | billing verify handlers + Google RTDN parse; Apple JWS payload fallback |

## Öncelik (2 haftalık)

1. ~~Identity Auth/Refresh/Revoke~~
2. ~~Social list/reject gap’leri~~
3. ~~Score/social service + notify unregister + auth middleware~~
4. SQL/repo yalnızca bug ısırırsa

## Katkı kuralları

- Yeni public davranış → test + bu tabloda satır güncelle
- `svcFacade` değişince `stubService` güncelle
- Suite yeşil olmadan iş bitmedi
