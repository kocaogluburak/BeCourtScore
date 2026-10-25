-- Rematch challenges: one pending directed pair+sport at a time.
-- source_match_id is a soft link (matches is RANGE-partitioned: no FK on id alone).

CREATE TABLE IF NOT EXISTS rematch_challenges (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    challenger_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    opponent_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sport             TEXT        NOT NULL CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL')),
    sets_to_win       INT         NOT NULL DEFAULT 2 CHECK (sets_to_win >= 1),
    advantage_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    points_to_win     INT         NOT NULL DEFAULT 11 CHECK (points_to_win >= 1),
    set_rule          TEXT,
    final_set         TEXT,
    source_match_id   UUID,
    status            TEXT        NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','accepted','rejected','expired','cancelled')),
    live_match_id     UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (challenger_id <> opponent_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_rematch_open
    ON rematch_challenges (challenger_id, opponent_id, sport)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_rematch_opponent_pending
    ON rematch_challenges (opponent_id, created_at DESC) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_rematch_challenger_pending
    ON rematch_challenges (challenger_id, created_at DESC) WHERE status = 'pending';
