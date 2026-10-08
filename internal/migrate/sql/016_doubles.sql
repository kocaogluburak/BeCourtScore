-- Optional second player on each side (doubles). Singles rows leave these null.
-- declined_user_ids: invitees who rejected a PENDING doubles invite without ending it
-- until every invitee has declined.

ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS player_a2_name TEXT,
    ADD COLUMN IF NOT EXISTS player_b2_name TEXT,
    ADD COLUMN IF NOT EXISTS player_a2_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS player_b2_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE live_matches
    ADD COLUMN IF NOT EXISTS player_a2_name TEXT,
    ADD COLUMN IF NOT EXISTS player_b2_name TEXT,
    ADD COLUMN IF NOT EXISTS player_a2_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS player_b2_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS declined_user_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_matches_player_a2_user
    ON matches (player_a2_user_id, played_at DESC)
    WHERE player_a2_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_matches_player_b2_user
    ON matches (player_b2_user_id, played_at DESC)
    WHERE player_b2_user_id IS NOT NULL;
