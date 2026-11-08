-- Weekly pulse: the player's IANA time zone, and one send row per ISO week.
-- time_zone stays null until a client writes it. pulse_sends is not goal_reminders_sent.

ALTER TABLE users ADD COLUMN IF NOT EXISTS time_zone TEXT;

CREATE TABLE IF NOT EXISTS pulse_sends (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start DATE NOT NULL,
    sent_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, week_start)
);
