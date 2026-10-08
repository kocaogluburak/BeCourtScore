-- Private groups with an invite code. Weekly ladder is not stored:
-- it is computed from finished matches whose registered players are
-- all current members. Deleting a match drops it from the ladder
-- because the ladder reads matches live. Social tournament rounds
-- stay on tournament_matches and never enter this table.

CREATE TABLE IF NOT EXISTS groups (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    sport       TEXT        NOT NULL CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG')),
    owner_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invite_code TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (invite_code)
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id   UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_group_members_user
    ON group_members (user_id);
