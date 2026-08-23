-- Allow hard-deleting users when they reported/confirmed tournament match results.
-- Without ON DELETE SET NULL, PostgreSQL RESTRICT blocks DELETE FROM users.

ALTER TABLE tournament_matches
    DROP CONSTRAINT IF EXISTS tournament_matches_reported_by_fkey,
    DROP CONSTRAINT IF EXISTS tournament_matches_confirmed_by_fkey;

ALTER TABLE tournament_matches
    ADD CONSTRAINT tournament_matches_reported_by_fkey
        FOREIGN KEY (reported_by) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT tournament_matches_confirmed_by_fkey
        FOREIGN KEY (confirmed_by) REFERENCES users(id) ON DELETE SET NULL;
