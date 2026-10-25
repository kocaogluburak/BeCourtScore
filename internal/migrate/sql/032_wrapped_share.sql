-- Public CourtScore Wrapped share tokens (FREE). Snapshot is frozen at mint time.

CREATE TABLE IF NOT EXISTS wrapped_shares (
    token       TEXT        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    year        INT         NOT NULL,
    snapshot    JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, year)
);

CREATE INDEX IF NOT EXISTS idx_wrapped_shares_user ON wrapped_shares (user_id, year);
