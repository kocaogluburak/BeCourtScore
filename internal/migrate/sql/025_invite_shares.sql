-- Tracked invite links minted from share cards / rematch / profile.

CREATE TABLE IF NOT EXISTS invite_shares (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    code            TEXT        NOT NULL UNIQUE,
    inviter_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source          TEXT        NOT NULL CHECK (source IN ('share_card','rematch','profile','generic')),
    sport           TEXT        CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL')),
    source_match_id UUID,
    click_count     INT         NOT NULL DEFAULT 0,
    signup_count    INT         NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_clicked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_invite_shares_inviter ON invite_shares (inviter_id, created_at DESC);

ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_via_code TEXT;
CREATE INDEX IF NOT EXISTS idx_users_invited_via_code ON users (invited_via_code) WHERE invited_via_code IS NOT NULL;
