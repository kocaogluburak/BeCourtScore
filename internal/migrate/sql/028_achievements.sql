-- Per-user badge unlocks, weekly goals, streaks, and reminder dedupe.
-- Badge definitions themselves are code-defined (achievements/catalog.go).

CREATE TABLE IF NOT EXISTS user_badges (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_code  TEXT        NOT NULL,
    unlocked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    meta        JSONB,
    PRIMARY KEY (user_id, badge_code)
);
CREATE INDEX IF NOT EXISTS idx_user_badges_user ON user_badges (user_id, unlocked_at DESC);

CREATE TABLE IF NOT EXISTS user_goals (
    user_id         UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    weekly_target   INT         NOT NULL DEFAULT 0,
    reminder_opt_in BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_streaks (
    user_id          UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    current_weeks    INT         NOT NULL DEFAULT 0,
    longest_weeks    INT         NOT NULL DEFAULT 0,
    last_active_week DATE,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS goal_reminders_sent (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start DATE NOT NULL,
    sent_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, week_start)
);
