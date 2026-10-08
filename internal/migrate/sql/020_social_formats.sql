-- Social night formats share the tournament row. Court count sizes each round.
-- Each fixture can carry two partners so a 2v2 points match is not a singles match.

ALTER TABLE tournaments DROP CONSTRAINT IF EXISTS tournaments_format_check;
ALTER TABLE tournaments ADD CONSTRAINT tournaments_format_check
    CHECK (format IN ('SINGLE_ELIM', 'ROUND_ROBIN', 'AMERICANO', 'MEXICANO', 'KING_OF_THE_COURT'));

ALTER TABLE tournaments ADD COLUMN IF NOT EXISTS court_count INT NOT NULL DEFAULT 1;

ALTER TABLE tournament_matches ADD COLUMN IF NOT EXISTS participant_a2_id UUID REFERENCES tournament_participants(id);
ALTER TABLE tournament_matches ADD COLUMN IF NOT EXISTS participant_b2_id UUID REFERENCES tournament_participants(id);
