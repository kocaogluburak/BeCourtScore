-- Per-set (or per-game for direct-point sports) score lines on finished matches.
-- Shape: [{"a":6,"b":4},{"a":7,"b":6}]

ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS set_scores JSONB NOT NULL DEFAULT '[]'::jsonb;
