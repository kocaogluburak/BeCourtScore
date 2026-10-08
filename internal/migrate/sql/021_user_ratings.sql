-- Per-sport Elo for registered singles matches. Deleting a match does not rewind the rating.

CREATE TABLE IF NOT EXISTS user_ratings (
    user_id          UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sport            TEXT    NOT NULL,
    rating           INT     NOT NULL DEFAULT 1500,
    matches_counted  INT     NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, sport)
);
