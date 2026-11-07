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
| `internal/modules/tournament/*_test.go` | lifecycle, bracket, Americano rotation, Mexicano re-pair, King of the Court movement, point standings, public preview omits join code |
| `internal/modules/social/handler_test.go` | search, profile, send/accept/reject/list/unfriend, group create/delete |
| `internal/modules/social/service_test.go` | request state machine, reopen-after-reject, AreFriends, profile status |
| `internal/modules/social/feed_test.go` | FEED-01…06: friend-only list + opt-out, OnMatchRecorded SSE, kudos idempotent/coalesce, comment validate/60s+10-per-day push cap/delete, POST 3s+40/day cap, 403 visibility, OnMatchDeleted cascade |
| `internal/modules/social/feed_handler_test.go` | GET /v1/feed page envelope, reaction 201/403, comments CRUD + 429, feed-settings |
| `internal/modules/social/groups_test.go` | ISO week window, non-member match ignored, member match counted, invite-code collision, blank/long name and unknown sport rejected, non-owner cannot delete |
| `internal/modules/social/seasons_test.go` | season points 3/1/0, doubles, cross-division ignored, window, night placement, promo/relegation ±2 / ±1, next-season newcomers lowest |
| `internal/modules/social/season_job_test.go` | rollover freezes once + champion + next ACTIVE + SSE; 8+ start 402; archive 402; current free; nil-safe OnMatchFinished |
| `internal/modules/social/seasons_handler_test.go` | list page envelope, current 404, archive/start 402, start 201, finalize 200 |
| `internal/modules/score/handler_test.go` | match CRUD + friendship gate + stats/h2h + fitness-trend + GET /matches/{id}/stats (200 Pro / 402 / 403 / 404) |
| `internal/modules/score/rematch_handler_test.go` | rematch create/list/accept/reject/cancel HTTP + 403/409 |
| `internal/modules/score/rematch_service_test.go` | friend gate, self, duplicate 409, accept prefill, reject/cancel, 48h expiry, SSE/push |
| `internal/modules/invite/handler_test.go` | mint 201 + public resolve 200/404 |
| `internal/modules/invite/service_test.go` | URL-safe code, click increment, RecordSignup attribution, invalid code noop |
| `internal/modules/score/live_handler_test.go` | live match start/get/update/end/list/cancel/accept/decline + conflict; open-list SQL uses jsonb_exists (not `?`) |
| `internal/modules/score/format_test.go` | set_rule / final_set defaults and unknown values |
| `internal/modules/score/pointlog_test.go` | duration, serve game, break conversion, golden break, old match omits point_log, ace/DF/winner/UE tallies, unknown tags ignored, serve/break unaffected, cap sanitizes outcomes |
| `internal/modules/score/fitness_test.go` | cap/clamp HR/kcal samples; drop invalid source |
| `internal/modules/score/service_test.go` | friendship gate, PENDING invite, guest IN_PROGRESS, guest opponent is not invited, duplicate open 409, accept/decline, opponent cancel IN_PROGRESS, stale 24h expire, participant list, push invite, doubles invite body, non-friend partner sends nothing, stats/h2h gates, GetMatchPointStats Pro/forbidden/missing |
| `internal/modules/score/players_test.go` | side label, identical names kept, invitee list skips starter/blank/duplicate, nameless partner rejected |
| `internal/modules/score/stats_test.go` | W/L form, rivals, sport filter, recorder-only ignored, H2H viewer perspective, doubles partner |
| `internal/modules/score/social_career_test.go` | night placement, social H2H ignores ties, doubles replay skips a guest |
| `internal/modules/score/wrapped_test.go` | year aggregate (totals, rival, longest, months, streaks, new sport, empty), handler 400s, free vs AllowStats, year prune + Elo peak, history append, public share 201/200/404 |
| `internal/modules/score/elo_test.go` | Elo +16 from 1500, K after 20, guest skips singles, four-player doubles Elo, sports stay separate |
| `internal/modules/score/elo_backfill_test.go` | replay singles from 1500, skip doubles, deterministic |
| `internal/modules/tournament/service_test.go` | group_id requires membership; stored on create |
| `internal/modules/score/service_test.go` (sport set) | PICKLEBALL accepted by CreateMatch and moves Elo; unknown sport rejected |
| `internal/modules/tournament/service_test.go` (sport set) | PICKLEBALL accepted as tournament sport in ListMine filter |
| `internal/modules/notify/handler_test.go` | device register + unregister |
| `internal/modules/notify/service_test.go` | Register/Unregister validation + SendToUser fan-out + localized skip-web |
| `internal/modules/achievements/rules_test.go` | badge unlock boundaries (9/10 matches, 4/5 wins, Elo 1599/1600, 9/10 TB sets), recorder-only empty, goal/streak/tournament |
| `internal/modules/achievements/streak_test.go` | ISO week UTC Monday, consecutive increment, gap reset, longest, reconcile, remaining days |
| `internal/modules/achievements/service_test.go` | OnMatchFinished persist+SSE+idempotent unlock, goal 0..14, friend badge gate |
| `internal/modules/achievements/handler_test.go` | catalog 200, badges page envelope, 403 non-friend, PUT goal 400/200 |
| `internal/modules/achievements/scheduler_test.go` | reminder window+dedupe, early-week skip, streak reconcile reset, backfill once, hour gate |
| `internal/modules/notify/fcm_test.go` | NewFCMSenderFromEnv → Noop when unset/bad path/invalid JSON |
| `internal/modules/identity/handler_test.go` | me/patch/delete + auth/refresh/logout |
| `internal/modules/identity/service_test.go` | AuthWithProvider / Refresh / Revoke / DeleteUser |
| `internal/modules/identity/google_test.go` | Google claims + unknown provider |
| `internal/modules/identity/apple_test.go` | Apple claims + email_verified bool/string + audience + JWK decode |
| `internal/platform/authkit/jwt_test.go` | JWT issue/parse |
| `internal/platform/authkit/middleware_test.go` | Bearer missing/invalid/valid → ctx |
| `internal/platform/sse/hub_test.go` | subscribe/publish |
| `internal/platform/httpx/pagination_test.go` | page clamp |
| `internal/modules/equipment/accrual_test.go` | seconds→hours math; active-only accrual both types; nil/zero/lapsed no-op; threshold once + notified_at; refire after reset/lifespan raise; shoes vs string payload; sweep idempotent |
| `internal/modules/equipment/service_test.go` | CRUD validation, ownership 404, activate swaps one active/type, Pro gate on every method, usage remaining/percent |
| `internal/modules/equipment/handler_test.go` | list page envelope, create 201, 402 pro_required, 404 foreign, delete 204, activate/reset 200 |
| `internal/modules/score/service_test.go` (accruer) | CreateMatch/EndLiveMatch call UsageAccruer with seconds; skip nil/zero |

## Shared happy-path cases

| Case | Durum | Not |
|---|---|---|
| AUTH-01…04 | n/a (client) | — |
| TOUR-01…04 | done | service + handler |
| SCORE-01…03 | n/a (client) | score module = match history API |
| LIVE-01 | done | start live match → 201; service end→history |
| SERVE-01 | done | live score patch accepts optional server_is_a |
| POINT-01 | done | point_log + duration_seconds; old match omits them and detail stats stay empty; log capped at 500 |
| POINT-02 | done | optional o/op tags; Summarize ace/DF/winner/UE; GET /v1/matches/{id}/stats 200 Pro / 402 / 403 / 404; AllowAdvancedStats is IsPro-only |
| FIT-01 | done | avg/max HR + kcal + fitness_samples persist; negatives/over-range dropped; samples capped at 240 |
| FIT-02 | done | GetMatch and ListMyMatches keep summary for free users and strip fitness_samples unless IsPro |
| FIT-03 | done | GET /v1/users/{id}/fitness-trend 402 free / 200 Pro / 403 non-friend |
| LIVE-02 | done | list open + cancel without history; creator or IN_PROGRESS opponent; list predicate is jsonb_exists |
| LIVE-03 | done | friend start → PENDING + match.invite; guest → IN_PROGRESS |
| LIVE-04 | done | accept/decline; list includes opponent; duplicate open → 409; doubles fan-out invite, one decline keeps PENDING |
| LIVE-05 | done | opponent cancel IN_PROGRESS; PENDING opponent cancel forbidden; 24h stale expire on list/start |
| HIST-01 | done | create match persists set_scores; invalid lines rejected |
| SPORT-01 | done | 5 sports accepted (incl. PICKLEBALL) across matches/tournaments/groups; migration 023 widens sport CHECKs |
| STATS-01 | done | user stats self/friend gate; form + rivals aggregation; recorder-only ignored; doubles partner counts, same-side is not a rival; guest W/L stays; doubles rating and social night summary attach |
| H2H-01 | done | h2h self → 400; friend gate; empty 200; viewer-perspective W/L; social rounds stay off set diff |
| REMATCH-01 | done | create requires accepted friend (403 self/non-friend); duplicate pending → 409 |
| REMATCH-02 | done | accept opponent-only + pending→accepted prefill DTO (no live_match_id) |
| REMATCH-03 | done | reject/cancel transitions + wrong-state 409 + SSE/push fan-out |
| REMATCH-04 | done | pending older than 48h expire lazily on list/create |
| WRAP-01 | done | GET /v1/me/wrapped year window + aggregation; FREE (no AllowStats); Elo peak from history with year-end fallback |
| INVITE-01 | done | mint unique URL-safe code; public resolve increments click_count; missing → 404 |
| INVITE-02 | done | RecordSignup increments signup_count + sets invited_via_code; invalid code is a no-op |
| PUSH-01 | done | register + unregister device token |
| SOCIAL-01…02 | done | list/accept/reject/unfriend handler + service |
| SOCIAL-03 | done | SendRequest FCM friend.request_received to addressee |
| SOCIAL-04 | done | AcceptRequest FCM friend.request_accepted to requester |
| GROUP-01 | done | group create/join/leave/delete; weekly ladder points (win 3 / loss 1 plus social-night placement); non-owner cannot delete |
| SEASON-01 | done | win 3 / loss 1 / absent 0; cross-division ignored; night 1st/2nd/3rd mapping |
| SEASON-02 | done | promo top 2 / relegate bottom 2; <6 uses ±1; champion = D1 rank 1; newcomers → lowest |
| SEASON-03 | done | rollover idempotent freeze + next ACTIVE + SSE season.completed/started |
| SEASON-04 | done | start 8+ and archive COMPLETED → 402 pro_required; current/active stay free |
| ID-BE-01 | done | AuthWithProvider + Refresh + Revoke |
| ID-BE-02 | done | DELETE /v1/me hard-delete account (handler + service) |
| PRO-01 | done | entitlement snapshot: under cap / 10+ lock / active / grace / expired |
| PRO-02 | done | POST /v1/matches 402 pro_required; EndLiveMatch skips history when locked |
| PRO-03 | done | tournament create max>8 Pro; default caps to 8 when over quota |
| PRO-04 | done | billing verify handlers + Google RTDN parse; Apple JWS payload fallback |
| ACH-01 | done | 12-code catalog; unlock boundaries; recorder-only ignored; idempotent insert |
| ACH-02 | done | ISO-week streak increment / gap reset / reconcile; weekly goal 0..14 |
| ACH-03 | done | GET/PUT /v1/me/goal + GET badges page + 403 non-friend; SSE unlock + goal.progress |
| ACH-04 | done | reminder window ≤3 days + opt-in + dedupe; localized push skips web; backfill once |
| FEED-01 | done | list shows only accepted friends' activities; non-friend & opted-out actors excluded |
| FEED-02 | done | OnMatchRecorded creates 1 activity + SSE to friends; opted-out actor → no row, no fan-out |
| FEED-03 | done | kudos idempotent; first kudos pushes author; self-kudos not pushed |
| FEED-04 | done | comment 1–280; SSE + 60s push rate-limit + 10/day cap (resets next day); delete by comment/activity author; others 403 |
| FEED-05 | done | visibility 403 on /v1/feed/{id}/* when caller is not actor/friend |
| FEED-06 | done | OnMatchDeleted removes the activity (cascades reactions/comments) |
| EQUIP-01 | done | duration accrues onto active STRING+SHOES; inactive/nil/zero/non-Pro no-op |
| EQUIP-02 | done | threshold push once + notified_at; reset / raised lifespan refires; sweep idempotent |
| EQUIP-03 | done | CRUD + activate one-active-per-type + ownership 404; AllowEquipment IsPro-only 402 |

## Öncelik (2 haftalık)

1. ~~Identity Auth/Refresh/Revoke~~
2. ~~Social list/reject gap’leri~~
3. ~~Score/social service + notify unregister + auth middleware~~
4. SQL/repo yalnızca bug ısırırsa

## Katkı kuralları

- Yeni public davranış → test + bu tabloda satır güncelle
- `svcFacade` değişince `stubService` güncelle
- Suite yeşil olmadan iş bitmedi
