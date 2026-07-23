-- Match history, moved from client-local DBs to the backend.
-- Partitioned by month on played_at for cheap pruning of time-range queries
-- and easy archival of old partitions.

CREATE TABLE IF NOT EXISTS matches (
    id                UUID        NOT NULL DEFAULT gen_random_uuid(),
    sport             TEXT        NOT NULL CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG')),
    player_a_name     TEXT        NOT NULL,
    player_b_name     TEXT        NOT NULL,
    player_a_user_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    player_b_user_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    sets_a            INT         NOT NULL CHECK (sets_a >= 0),
    sets_b            INT         NOT NULL CHECK (sets_b >= 0),
    winner_side       CHAR(1)     NOT NULL CHECK (winner_side IN ('A','B')),
    created_by        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    played_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, played_at)
) PARTITION BY RANGE (played_at);

-- Monthly partitions: 3 months back through 24 months ahead, plus a DEFAULT
-- catch-all so inserts never fail when a month has no dedicated partition.
-- Ops note: extend with new partitions periodically (see README).
DO $$
DECLARE
    m      DATE := date_trunc('month', NOW())::DATE - INTERVAL '3 months';
    last_m DATE := date_trunc('month', NOW())::DATE + INTERVAL '24 months';
BEGIN
    WHILE m <= last_m LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS matches_%s PARTITION OF matches FOR VALUES FROM (%L) TO (%L)',
            to_char(m, 'YYYY_MM'), m, m + INTERVAL '1 month'
        );
        m := (m + INTERVAL '1 month')::DATE;
    END LOOP;
END $$;

CREATE TABLE IF NOT EXISTS matches_default PARTITION OF matches DEFAULT;

-- Partitioned (parent-level) indexes; Postgres creates matching local
-- indexes on every partition automatically.
CREATE INDEX IF NOT EXISTS idx_matches_created_by_played_at
    ON matches (created_by, played_at DESC);
CREATE INDEX IF NOT EXISTS idx_matches_player_a_played_at
    ON matches (player_a_user_id, played_at DESC) WHERE player_a_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_matches_player_b_played_at
    ON matches (player_b_user_id, played_at DESC) WHERE player_b_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_matches_sport_played_at
    ON matches (sport, played_at DESC);
