-- Append-only Elo snapshots so Wrapped can report a year peak.
-- Deleting a match does not rewind ratings or history (same as user_ratings).
-- No FK to matches: the parent table is RANGE-partitioned.

CREATE TABLE IF NOT EXISTS user_rating_history (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sport           TEXT        NOT NULL,
    rating          INT         NOT NULL,
    matches_counted INT         NOT NULL,
    match_id        UUID,
    played_at       TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS user_rating_history_user_played
    ON user_rating_history (user_id, played_at);
