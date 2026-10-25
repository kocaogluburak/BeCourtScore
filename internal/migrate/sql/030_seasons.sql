-- Seasonal leagues layered on top of invite-code groups. Live standings are
-- still computed from finished matches during an ACTIVE season; a frozen
-- snapshot is written at rollover so past seasons are immutable archive.

CREATE TABLE IF NOT EXISTS seasons (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id            UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    season_number       INT         NOT NULL,
    name                TEXT        NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'ACTIVE'
                          CHECK (status IN ('ACTIVE','COMPLETED')),
    starts_at           TIMESTAMPTZ NOT NULL,
    ends_at             TIMESTAMPTZ NOT NULL,
    division_count      INT         NOT NULL DEFAULT 1,
    count_social_nights BOOLEAN     NOT NULL DEFAULT FALSE,
    champion_user_id    UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, season_number)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_seasons_one_active
    ON seasons (group_id) WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS idx_seasons_group ON seasons (group_id, season_number DESC);

CREATE TABLE IF NOT EXISTS season_members (
    season_id     UUID NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    division_tier INT  NOT NULL DEFAULT 1,
    joined_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (season_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_season_members_div
    ON season_members (season_id, division_tier);

CREATE TABLE IF NOT EXISTS season_standings (
    season_id     UUID NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    division_tier INT  NOT NULL,
    rank          INT  NOT NULL,
    points        INT  NOT NULL,
    wins          INT  NOT NULL,
    losses        INT  NOT NULL,
    matches       INT  NOT NULL,
    set_diff      INT  NOT NULL,
    outcome       TEXT NOT NULL CHECK (outcome IN ('CHAMPION','PROMOTED','RELEGATED','STAYED')),
    display_name  TEXT NOT NULL,
    PRIMARY KEY (season_id, user_id)
);

ALTER TABLE tournaments ADD COLUMN IF NOT EXISTS group_id UUID
    REFERENCES groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_tournaments_group ON tournaments (group_id)
    WHERE group_id IS NOT NULL;
