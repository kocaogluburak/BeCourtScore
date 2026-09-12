-- Head-to-head lookups: both players registered, ordered by most recent.
-- Career stats reuse existing partial indexes on player_a_user_id / player_b_user_id.

CREATE INDEX IF NOT EXISTS idx_matches_h2h_pair_played
    ON matches (
        LEAST(player_a_user_id, player_b_user_id),
        GREATEST(player_a_user_id, player_b_user_id),
        played_at DESC
    )
    WHERE player_a_user_id IS NOT NULL
      AND player_b_user_id IS NOT NULL;
