CREATE TABLE IF NOT EXISTS users (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email          TEXT        UNIQUE,
    email_verified BOOLEAN     NOT NULL DEFAULT FALSE,
    nickname       TEXT,
    name           TEXT,
    surname        TEXT,
    profile_icon   TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth_identities (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT        NOT NULL,
    provider_subject TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_subject)
);

CREATE INDEX IF NOT EXISTS idx_auth_identities_user_id ON auth_identities(user_id);
