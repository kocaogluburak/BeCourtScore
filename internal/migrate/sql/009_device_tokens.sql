-- Device tokens for FCM / APNs push delivery (via Firebase Cloud Messaging).

CREATE TABLE IF NOT EXISTS device_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT        NOT NULL,
    platform    TEXT        NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (token)
);

CREATE INDEX IF NOT EXISTS idx_device_tokens_user ON device_tokens (user_id);
