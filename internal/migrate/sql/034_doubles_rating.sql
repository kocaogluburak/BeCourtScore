-- Doubles Elo sits beside singles. Existing rows stay SINGLES.
-- A social round is keyed by tournament_match_id so a replay cannot move
-- the rating twice. Deleting a match still does not rewind a rating.

ALTER TABLE user_ratings
    ADD COLUMN discipline TEXT NOT NULL DEFAULT 'SINGLES';

ALTER TABLE user_ratings DROP CONSTRAINT user_ratings_pkey;
ALTER TABLE user_ratings ADD PRIMARY KEY (user_id, sport, discipline);

ALTER TABLE user_ratings
    ADD CONSTRAINT user_ratings_discipline_chk CHECK (discipline IN ('SINGLES', 'DOUBLES'));

ALTER TABLE user_rating_history
    ADD COLUMN discipline TEXT NOT NULL DEFAULT 'SINGLES';

ALTER TABLE user_rating_history
    ADD COLUMN tournament_match_id UUID;

ALTER TABLE user_rating_history
    ADD CONSTRAINT user_rating_history_discipline_chk CHECK (discipline IN ('SINGLES', 'DOUBLES'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_rating_history_tmatch
    ON user_rating_history (user_id, tournament_match_id)
    WHERE tournament_match_id IS NOT NULL;
