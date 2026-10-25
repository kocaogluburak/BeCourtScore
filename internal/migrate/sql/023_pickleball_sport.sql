-- Add PICKLEBALL as a 5th supported sport. Widen every sport CHECK constraint.
-- matches is RANGE-partitioned by month; the sport CHECK is a parent-level
-- constraint, so a single DROP/ADD propagates to all partitions.

ALTER TABLE matches
    DROP CONSTRAINT IF EXISTS matches_sport_check,
    ADD CONSTRAINT matches_sport_check
        CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL'));

ALTER TABLE tournaments
    DROP CONSTRAINT IF EXISTS tournaments_sport_check,
    ADD CONSTRAINT tournaments_sport_check
        CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL'));

ALTER TABLE live_matches
    DROP CONSTRAINT IF EXISTS live_matches_sport_check,
    ADD CONSTRAINT live_matches_sport_check
        CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL'));

ALTER TABLE groups
    DROP CONSTRAINT IF EXISTS groups_sport_check,
    ADD CONSTRAINT groups_sport_check
        CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL'));

-- user_ratings.sport is free-text (no CHECK); PICKLEBALL rows are created
-- automatically by the per-sport rating path. No change needed there.
