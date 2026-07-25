-- Tournaments: organizer-run brackets & round-robins.
-- Participants join with a shared code; organizer draws the fixture,
-- results are entered by organizer or players (with opponent confirmation).

CREATE TABLE IF NOT EXISTS tournaments (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             TEXT        NOT NULL UNIQUE,
    join_code        TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    sport            TEXT        NOT NULL CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG')),
    format           TEXT        NOT NULL CHECK (format IN ('SINGLE_ELIM','ROUND_ROBIN')),
    status           TEXT        NOT NULL DEFAULT 'REGISTRATION'
                       CHECK (status IN ('DRAFT','REGISTRATION','LOCKED','ONGOING','COMPLETED','CANCELLED')),
    organizer_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    max_participants INT         NOT NULL DEFAULT 32 CHECK (max_participants BETWEEN 2 AND 128),
    champion_id      UUID,       -- set when COMPLETED (tournament_participants.id)
    starts_at        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tournaments_organizer ON tournaments(organizer_id);

CREATE TABLE IF NOT EXISTS tournament_participants (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id  UUID        NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name   TEXT        NOT NULL,          -- snapshot at join time
    seed           INT,                            -- null until draw / manual seeding
    status         TEXT        NOT NULL DEFAULT 'CONFIRMED'
                     CHECK (status IN ('CONFIRMED','WITHDRAWN')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tournament_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_tparticipants_tournament ON tournament_participants(tournament_id);

CREATE TABLE IF NOT EXISTS tournament_matches (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id         UUID        NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    round                 INT         NOT NULL,
    position_in_round     INT         NOT NULL,
    participant_a_id      UUID        REFERENCES tournament_participants(id),
    participant_b_id      UUID        REFERENCES tournament_participants(id),
    next_match_id         UUID        REFERENCES tournament_matches(id),
    winner_participant_id UUID        REFERENCES tournament_participants(id),
    score_summary         TEXT,
    reported_by           UUID        REFERENCES users(id),
    confirmed_by          UUID        REFERENCES users(id),
    match_id              UUID,       -- optional link to score.matches (unused in v1)
    status                TEXT        NOT NULL DEFAULT 'PENDING'
                            CHECK (status IN ('PENDING','READY','PENDING_CONFIRMATION','COMPLETED','BYE')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tournament_id, round, position_in_round)
);
CREATE INDEX IF NOT EXISTS idx_tmatches_tournament ON tournament_matches(tournament_id);
CREATE INDEX IF NOT EXISTS idx_tmatches_next ON tournament_matches(next_match_id);
