-- Pro subscriptions + public live share tokens.

CREATE TABLE IF NOT EXISTS subscriptions (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    store            TEXT        NOT NULL CHECK (store IN ('google', 'apple')),
    product_id       TEXT        NOT NULL,
    original_txn_id  TEXT        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN ('active', 'grace', 'expired', 'revoked')),
    expires_at       TIMESTAMPTZ NOT NULL,
    grace_until      TIMESTAMPTZ,
    purchase_token   TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (store, original_txn_id)
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_status
    ON subscriptions (user_id, status);

ALTER TABLE live_matches
    ADD COLUMN IF NOT EXISTS share_token TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_live_matches_share_token
    ON live_matches (share_token)
    WHERE share_token IS NOT NULL;
