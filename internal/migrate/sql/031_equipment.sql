-- Per-user physical equipment (racket string + shoes). Accrual targets the
-- single ACTIVE row per (user, type). Hours are derived from accumulated_seconds.

CREATE TABLE IF NOT EXISTS equipment (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type                TEXT        NOT NULL CHECK (type IN ('STRING', 'SHOES')),
    name                TEXT        NOT NULL,
    purchase_date       DATE,
    lifespan_hours      INTEGER     NOT NULL CHECK (lifespan_hours > 0),
    accumulated_seconds BIGINT      NOT NULL DEFAULT 0,
    is_active           BOOLEAN     NOT NULL DEFAULT TRUE,
    notified_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_equipment_user ON equipment (user_id);

-- At most one ACTIVE item per (user, type). Accrual UPDATE ... WHERE is_active
-- hits exactly one row per type.
CREATE UNIQUE INDEX IF NOT EXISTS uq_equipment_active
    ON equipment (user_id, type) WHERE is_active;
