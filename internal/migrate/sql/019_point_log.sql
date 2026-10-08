ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS point_log JSONB,
    ADD COLUMN IF NOT EXISTS duration_seconds INT;
