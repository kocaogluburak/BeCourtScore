-- Idempotent Elo history backfill support.
-- user_rating_history (027) only records going forward. Wrapped elo_peak needs
-- a unique (user_id, match_id) so a one-off replay can INSERT ... ON CONFLICT
-- DO NOTHING without duplicating rows already written by applySinglesElo.
--
-- The replay itself is Go (score.BackfillRatingHistory): nextRatings is pure
-- and must run in global played_at order. This migration only adds the unique
-- index the backfill (and future live inserts) rely on.
--
-- Safe to re-run. Does not rewrite user_ratings.

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_rating_history_match
    ON user_rating_history (user_id, match_id)
    WHERE match_id IS NOT NULL;
