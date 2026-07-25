-- Performance indexes for list GET endpoints as tables grow.
-- Covers friendships, tournament "mine" lookups, and match remaining-status scans.

-- Accepted friends: covering friend-id for UNION-style listFriends (both directions).
CREATE INDEX IF NOT EXISTS idx_friendships_requester_accepted_friend
    ON friendships (requester_id, addressee_id) WHERE status = 'accepted';
CREATE INDEX IF NOT EXISTS idx_friendships_addressee_accepted_friend
    ON friendships (addressee_id, requester_id) WHERE status = 'accepted';

-- "My tournaments": EXISTS / IN lookup by participant user_id.
CREATE INDEX IF NOT EXISTS idx_tparticipants_user
    ON tournament_participants (user_id, tournament_id);

-- Organizer list ordered by created_at.
CREATE INDEX IF NOT EXISTS idx_tournaments_organizer_created
    ON tournaments (organizer_id, created_at DESC);

-- Confirmed participant counts per tournament.
CREATE INDEX IF NOT EXISTS idx_tparticipants_tournament_confirmed
    ON tournament_participants (tournament_id)
    WHERE status = 'CONFIRMED';

-- Remaining / status-filtered fixture scans.
CREATE INDEX IF NOT EXISTS idx_tmatches_tournament_status
    ON tournament_matches (tournament_id, status);
