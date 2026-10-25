# Domain: achievements — streaks, weekly goals, badges

> Focused notes for the `achievements` module. See `../../../CLAUDE.md` for the whole backend.

FREE habit loop: consecutive-week streaks, a weekly match goal, and a code-defined
badge catalog. Copy never leaves the clients — the API emits codes/keys only.

## Files

| File | Role |
|------|------|
| `routes.go` | `/v1/achievements/badges`, `/v1/users/{id}/badges`, `/v1/me/goal` (Bearer) |
| `handler.go` | Catalog, paginated unlocks, goal get/set |
| `service.go` | Eval + SSE; implements `score.AchievementHook` |
| `catalog.go` | 12 v1 badge codes (source of truth) |
| `rules.go` | Pure evaluators |
| `streak.go` | ISO-week math (Monday 00:00 UTC) |
| `repo.go` | `user_badges` / `user_goals` / `user_streaks` / `goal_reminders_sent` + match/rating/champion reads |
| `scheduler.go` | Hourly ticker: reminder / streak reconcile / one-time backfill |
| `*_test.go` | rules, streak, service, handler, scheduler |

## Routes

```
GET  /v1/achievements/badges     → catalog [{code, category, tier, threshold, icon, sort}]
GET  /v1/users/{userID}/badges   → httpx.Page of unlocks (self or accepted friend; else 403)
GET  /v1/me/goal                 → {weekly_target, reminder_opt_in, week, streak}
PUT  /v1/me/goal                 → {weekly_target 0..14, reminder_opt_in}
```

## Triggers

- `score` calls `OnMatchFinished` after CreateMatch / EndLiveMatch (nil-safe, best-effort).
- Scheduler (started in `cmd/api/main.go`): backfill at boot; daily streak reconcile;
  reminder once/day near 18:00 UTC when remaining ISO-week days ≤ 3.

## SSE / push

- `achievement.unlocked` `{badge_code, unlocked_at, meta, items[]}`
- `goal.progress` `{week_start, target, current, reached}`
- Reminder: `notify.SendLocalizedToUser` — iOS APNs loc-key, Android data `type=goal.reminder`.

## After changes

`cd BeCourtScore && go test ./internal/modules/achievements/...` then `go test ./...`.
Inventory: `BeCourtScore/test/test.md`.
