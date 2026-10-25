-- Friend activity feed: one row per recorded match, plus kudos + short comments.
-- matches is RANGE-partitioned with composite PK (id, played_at), so match_id
-- is a soft UUID reference (cleaned up via score.ActivitySink.OnMatchDeleted).

CREATE TABLE IF NOT EXISTS activities (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type          TEXT        NOT NULL CHECK (type IN ('match_recorded')),
    match_id      UUID,
    sport         TEXT        NOT NULL,
    summary       JSONB       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_activities_actor_created
    ON activities (actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activities_match ON activities (match_id);

CREATE TABLE IF NOT EXISTS activity_reactions (
    activity_id   UUID        NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type          TEXT        NOT NULL DEFAULT 'kudos' CHECK (type IN ('kudos')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (activity_id, user_id, type)
);

CREATE TABLE IF NOT EXISTS activity_comments (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id   UUID        NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body          TEXT        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 280),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_activity_comments_activity_created
    ON activity_comments (activity_id, created_at ASC);

ALTER TABLE users ADD COLUMN IF NOT EXISTS feed_opt_out BOOLEAN NOT NULL DEFAULT FALSE;
