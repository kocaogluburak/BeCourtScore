-- Friendships: request -> accept flow between two registered users.
-- A single row per pair; direction encoded by requester/addressee.

CREATE TABLE IF NOT EXISTS friendships (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id  UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    addressee_id  UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status        TEXT        NOT NULL CHECK (status IN ('pending','accepted','rejected')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (requester_id <> addressee_id),
    UNIQUE (requester_id, addressee_id)
);

-- One row per unordered pair regardless of direction.
CREATE UNIQUE INDEX IF NOT EXISTS uq_friendships_pair
    ON friendships (LEAST(requester_id, addressee_id), GREATEST(requester_id, addressee_id));

-- Accepted-friend lists (both directions).
CREATE INDEX IF NOT EXISTS idx_friendships_requester_accepted
    ON friendships (requester_id) WHERE status = 'accepted';
CREATE INDEX IF NOT EXISTS idx_friendships_addressee_accepted
    ON friendships (addressee_id) WHERE status = 'accepted';

-- Incoming pending requests.
CREATE INDEX IF NOT EXISTS idx_friendships_addressee_pending
    ON friendships (addressee_id, created_at DESC) WHERE status = 'pending';

-- User search by nickname / name (case-insensitive prefix).
CREATE INDEX IF NOT EXISTS idx_users_nickname_lower ON users (LOWER(nickname) text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_users_name_lower     ON users (LOWER(name) text_pattern_ops);
