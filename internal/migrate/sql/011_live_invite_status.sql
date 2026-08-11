-- Live match invites: PENDING until the registered opponent accepts.

ALTER TABLE live_matches
    DROP CONSTRAINT IF EXISTS live_matches_status_check;

ALTER TABLE live_matches
    ADD CONSTRAINT live_matches_status_check
        CHECK (status IN ('PENDING', 'IN_PROGRESS', 'ENDED'));

-- Participant-facing open list includes invites + live sessions.
CREATE INDEX IF NOT EXISTS idx_live_matches_open_participant
    ON live_matches (created_by, started_at DESC)
    WHERE status IN ('PENDING', 'IN_PROGRESS');
