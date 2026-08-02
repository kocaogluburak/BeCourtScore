-- In-progress shared matches. Finished history remains in `matches`.

CREATE TABLE IF NOT EXISTS live_matches (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    sport             TEXT        NOT NULL CHECK (sport IN ('TENNIS', 'PADEL', 'SQUASH', 'PING_PONG')),
    status            TEXT        NOT NULL DEFAULT 'IN_PROGRESS'
                        CHECK (status IN ('IN_PROGRESS', 'ENDED')),
    player_a_name     TEXT        NOT NULL,
    player_b_name     TEXT        NOT NULL,
    player_a_user_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    player_b_user_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    sets_a            INT         NOT NULL DEFAULT 0 CHECK (sets_a >= 0),
    sets_b            INT         NOT NULL DEFAULT 0 CHECK (sets_b >= 0),
    games_a           INT         NOT NULL DEFAULT 0 CHECK (games_a >= 0),
    games_b           INT         NOT NULL DEFAULT 0 CHECK (games_b >= 0),
    score_a           INT         NOT NULL DEFAULT 0 CHECK (score_a >= 0),
    score_b           INT         NOT NULL DEFAULT 0 CHECK (score_b >= 0),
    is_tie_break      BOOLEAN     NOT NULL DEFAULT FALSE,
    sets_to_win       INT         NOT NULL DEFAULT 2 CHECK (sets_to_win >= 1),
    advantage_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    points_to_win     INT         NOT NULL DEFAULT 11 CHECK (points_to_win >= 1),
    winner_side       CHAR(1)     CHECK (winner_side IN ('A', 'B')),
    created_by        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Soft link only: `matches` is partitioned with PK (id, played_at), so a
    -- FK on matches(id) alone is invalid (SQLSTATE 42830).
    history_match_id  UUID,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at          TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_live_matches_created_by_open
    ON live_matches (created_by) WHERE status = 'IN_PROGRESS';
CREATE INDEX IF NOT EXISTS idx_live_matches_player_a_open
    ON live_matches (player_a_user_id)
    WHERE status = 'IN_PROGRESS' AND player_a_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_live_matches_player_b_open
    ON live_matches (player_b_user_id)
    WHERE status = 'IN_PROGRESS' AND player_b_user_id IS NOT NULL;
