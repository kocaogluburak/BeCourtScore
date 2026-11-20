-- 036_equipment_catalog: sport-scoped gear bag (plans/gear-bag/02-technical.md §2.1).
--
-- * ops-editable catalog (categories / brands / models) + seed
-- * equipment table EXTENDED ONLY (relax NOT NULLs, add columns, new indexes) so the
--   previous binary keeps working against the new schema (rollback safety, §3)
-- * AC-22 backfill: legacy STRING/SHOES rows -> Tennis categories
-- * match_equipment_usage: forward-only racket/string/... combo win-rate

-- ---------------------------------------------------------------------------
-- Catalog tables
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS equipment_categories (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sport                  TEXT NOT NULL CHECK (sport IN ('TENNIS','PADEL','SQUASH','PING_PONG','PICKLEBALL')),
    code                   TEXT NOT NULL,
    parent_category_id     UUID NULL REFERENCES equipment_categories(id),
    display_order          INT  NOT NULL DEFAULT 0,
    is_critical            BOOLEAN NOT NULL DEFAULT FALSE,
    supports_renew         BOOLEAN NOT NULL DEFAULT FALSE,
    threshold_type         TEXT NOT NULL CHECK (threshold_type IN ('NONE','HOURS','HOURS_OR_DAYS')),
    default_lifespan_hours INT  NULL CHECK (default_lifespan_hours IS NULL OR default_lifespan_hours > 0),
    default_lifespan_days  INT  NULL CHECK (default_lifespan_days  IS NULL OR default_lifespan_days  > 0),
    is_active              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sport, code)
);

CREATE TABLE IF NOT EXISTS equipment_brands (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL UNIQUE,
    accent_color_hex  TEXT NOT NULL CHECK (accent_color_hex ~ '^#[0-9A-Fa-f]{6}$'),
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS equipment_models (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id             UUID NOT NULL REFERENCES equipment_categories(id),
    brand_id                UUID NOT NULL REFERENCES equipment_brands(id),
    name                    TEXT NOT NULL,
    default_lifespan_hours  INT  NULL,
    default_lifespan_days   INT  NULL,
    is_active               BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (category_id, brand_id, name)
);
CREATE INDEX IF NOT EXISTS idx_equipment_models_category_brand ON equipment_models (category_id, brand_id);

-- ---------------------------------------------------------------------------
-- Seed: categories (16). Parents first, then children (STRING->RACKET, RUBBER_*->BLADE).
-- Squash/STRING is HOURS only (technical analysis note: AC-2 table is authoritative).
-- ---------------------------------------------------------------------------
INSERT INTO equipment_categories
    (sport, code, parent_category_id, display_order, is_critical, supports_renew, threshold_type, default_lifespan_hours, default_lifespan_days)
VALUES
    ('TENNIS',     'RACKET',   NULL, 1, FALSE, FALSE, 'NONE',  NULL, NULL),
    ('TENNIS',     'SHOES',    NULL, 3, FALSE, FALSE, 'HOURS', 60,   NULL),
    ('TENNIS',     'OVERGRIP', NULL, 4, FALSE, FALSE, 'HOURS', 6,    NULL),
    ('PADEL',      'PALA',     NULL, 1, FALSE, FALSE, 'HOURS', 90,   NULL),
    ('PADEL',      'SHOES',    NULL, 2, FALSE, FALSE, 'HOURS', 60,   NULL),
    ('PADEL',      'OVERGRIP', NULL, 3, FALSE, FALSE, 'HOURS', 6,    NULL),
    ('SQUASH',     'RACKET',   NULL, 1, FALSE, FALSE, 'NONE',  NULL, NULL),
    ('SQUASH',     'SHOES',    NULL, 3, FALSE, FALSE, 'HOURS', 55,   NULL),
    ('PING_PONG',  'BLADE',    NULL, 1, FALSE, FALSE, 'NONE',  NULL, NULL),
    ('PICKLEBALL', 'PADDLE',   NULL, 1, FALSE, FALSE, 'HOURS', 80,   NULL),
    ('PICKLEBALL', 'SHOES',    NULL, 2, FALSE, FALSE, 'HOURS', 60,   NULL),
    ('PICKLEBALL', 'OVERGRIP', NULL, 3, FALSE, FALSE, 'HOURS', 6,    NULL)
ON CONFLICT (sport, code) DO NOTHING;

INSERT INTO equipment_categories
    (sport, code, parent_category_id, display_order, is_critical, supports_renew, threshold_type, default_lifespan_hours, default_lifespan_days)
SELECT v.sport, v.code, p.id, v.display_order, v.is_critical, v.supports_renew, v.threshold_type,
       v.default_lifespan_hours, v.default_lifespan_days
FROM (VALUES
    ('TENNIS',    'STRING',    'RACKET', 2, TRUE, TRUE, 'HOURS_OR_DAYS', 15, 90),
    ('SQUASH',    'STRING',    'RACKET', 2, TRUE, TRUE, 'HOURS',         15, NULL::int),
    ('PING_PONG', 'RUBBER_FH', 'BLADE',  2, TRUE, TRUE, 'HOURS',         45, NULL::int),
    ('PING_PONG', 'RUBBER_BH', 'BLADE',  3, TRUE, TRUE, 'HOURS',         60, NULL::int)
) AS v(sport, code, parent_code, display_order, is_critical, supports_renew, threshold_type,
       default_lifespan_hours, default_lifespan_days)
JOIN equipment_categories p ON p.sport = v.sport AND p.code = v.parent_code
ON CONFLICT (sport, code) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Seed: brands (34)
-- ---------------------------------------------------------------------------
INSERT INTO equipment_brands (name, accent_color_hex) VALUES
    ('Babolat', '#0B3D91'),
    ('Wilson', '#D32F2F'),
    ('Head', '#FBC02D'),
    ('Yonex', '#1565C0'),
    ('Tecnifibre', '#EF6C00'),
    ('Dunlop', '#FDD835'),
    ('Prince', '#6A1B9A'),
    ('Luxilon', '#212121'),
    ('Solinco', '#00897B'),
    ('ASICS', '#003DA5'),
    ('Nike', '#111111'),
    ('Adidas', '#0F7A3D'),
    ('New Balance', '#CC092F'),
    ('Bullpadel', '#FFB300'),
    ('Nox', '#D50032'),
    ('Siux', '#00A651'),
    ('Varlion', '#5C2D91'),
    ('Joma', '#0033A0'),
    ('Karakal', '#000000'),
    ('Salming', '#FDB927'),
    ('Ashaway', '#006747'),
    ('Butterfly', '#E4002B'),
    ('Stiga', '#005BAC'),
    ('Donic', '#F7941D'),
    ('Yasaka', '#C8102E'),
    ('DHS', '#ED1C24'),
    ('Tibhar', '#0072CE'),
    ('Joola', '#FFD100'),
    ('Franklin', '#004B87'),
    ('Gamma', '#F7941D'),
    ('Paddletek', '#6CACE4'),
    ('CRBN', '#000000'),
    ('Six Zero', '#FF5A00'),
    ('Selkirk', '#8E8E8E')
ON CONFLICT (name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Seed: models (167). Brand x category pairing is implied by these rows.
-- RUBBER_FH and RUBBER_BH carry the same brand/model list as separate rows.
-- ---------------------------------------------------------------------------
INSERT INTO equipment_models (category_id, brand_id, name)
SELECT c.id, b.id, v.model
FROM (VALUES
    ('TENNIS', 'RACKET', 'Babolat', 'Pure Drive'),
    ('TENNIS', 'RACKET', 'Babolat', 'Pure Aero'),
    ('TENNIS', 'RACKET', 'Babolat', 'Pure Strike'),
    ('TENNIS', 'RACKET', 'Babolat', 'Boost Drive'),
    ('TENNIS', 'RACKET', 'Wilson', 'Blade 98'),
    ('TENNIS', 'RACKET', 'Wilson', 'Clash 100'),
    ('TENNIS', 'RACKET', 'Wilson', 'Pro Staff 97'),
    ('TENNIS', 'RACKET', 'Wilson', 'Ultra 100'),
    ('TENNIS', 'RACKET', 'Head', 'Speed MP'),
    ('TENNIS', 'RACKET', 'Head', 'Radical MP'),
    ('TENNIS', 'RACKET', 'Head', 'Extreme MP'),
    ('TENNIS', 'RACKET', 'Yonex', 'Ezone 98'),
    ('TENNIS', 'RACKET', 'Yonex', 'Vcore 95'),
    ('TENNIS', 'RACKET', 'Yonex', 'Percept 100'),
    ('TENNIS', 'RACKET', 'Tecnifibre', 'TFight 300'),
    ('TENNIS', 'RACKET', 'Tecnifibre', 'TFlash 300'),
    ('TENNIS', 'RACKET', 'Dunlop', 'FX 500'),
    ('TENNIS', 'RACKET', 'Dunlop', 'CX 200'),
    ('TENNIS', 'RACKET', 'Prince', 'Phantom 100X'),
    ('TENNIS', 'RACKET', 'Prince', 'Textreme Tour 100'),
    ('TENNIS', 'STRING', 'Luxilon', 'ALU Power'),
    ('TENNIS', 'STRING', 'Luxilon', 'Big Banger Original'),
    ('TENNIS', 'STRING', 'Luxilon', 'Element'),
    ('TENNIS', 'STRING', 'Babolat', 'RPM Blast'),
    ('TENNIS', 'STRING', 'Babolat', 'VS Touch'),
    ('TENNIS', 'STRING', 'Babolat', 'Xcel'),
    ('TENNIS', 'STRING', 'Solinco', 'Hyper-G'),
    ('TENNIS', 'STRING', 'Solinco', 'Confidential'),
    ('TENNIS', 'STRING', 'Solinco', 'Tour Bite'),
    ('TENNIS', 'STRING', 'Head', 'Lynx'),
    ('TENNIS', 'STRING', 'Head', 'Hawk Touch'),
    ('TENNIS', 'STRING', 'Tecnifibre', 'X-One Biphase'),
    ('TENNIS', 'STRING', 'Tecnifibre', 'Black Code'),
    ('TENNIS', 'STRING', 'Wilson', 'NXT'),
    ('TENNIS', 'STRING', 'Wilson', 'Revolve'),
    ('TENNIS', 'SHOES', 'ASICS', 'Gel-Resolution'),
    ('TENNIS', 'SHOES', 'ASICS', 'Court FF'),
    ('TENNIS', 'SHOES', 'Nike', 'Vapor Lite'),
    ('TENNIS', 'SHOES', 'Nike', 'Zoom Vapor Cage'),
    ('TENNIS', 'SHOES', 'Adidas', 'Barricade'),
    ('TENNIS', 'SHOES', 'Adidas', 'CourtJam'),
    ('TENNIS', 'SHOES', 'Babolat', 'Jet Mach'),
    ('TENNIS', 'SHOES', 'Babolat', 'SFX3'),
    ('TENNIS', 'SHOES', 'New Balance', 'Fuel Cell 996'),
    ('TENNIS', 'SHOES', 'New Balance', '996v5'),
    ('TENNIS', 'OVERGRIP', 'Babolat', 'Pro Tacky'),
    ('TENNIS', 'OVERGRIP', 'Babolat', 'VS Original'),
    ('TENNIS', 'OVERGRIP', 'Wilson', 'Pro Overgrip'),
    ('TENNIS', 'OVERGRIP', 'Wilson', 'Sublime'),
    ('TENNIS', 'OVERGRIP', 'Head', 'XtremeSoft'),
    ('TENNIS', 'OVERGRIP', 'Head', 'Tacky Touch'),
    ('TENNIS', 'OVERGRIP', 'Yonex', 'Super Grap'),
    ('TENNIS', 'OVERGRIP', 'Yonex', 'AC102'),
    ('PADEL', 'PALA', 'Bullpadel', 'Vertex 04'),
    ('PADEL', 'PALA', 'Bullpadel', 'Hack'),
    ('PADEL', 'PALA', 'Bullpadel', 'Flow'),
    ('PADEL', 'PALA', 'Nox', 'AT10 Genius'),
    ('PADEL', 'PALA', 'Nox', 'ML10'),
    ('PADEL', 'PALA', 'Nox', 'Luxury'),
    ('PADEL', 'PALA', 'Babolat', 'Technical Viper'),
    ('PADEL', 'PALA', 'Babolat', 'Air Viper'),
    ('PADEL', 'PALA', 'Head', 'Extreme Pro'),
    ('PADEL', 'PALA', 'Head', 'Flash'),
    ('PADEL', 'PALA', 'Adidas', 'Metalbone'),
    ('PADEL', 'PALA', 'Adidas', 'Adipower'),
    ('PADEL', 'PALA', 'Siux', 'Electra'),
    ('PADEL', 'PALA', 'Siux', 'Tango'),
    ('PADEL', 'PALA', 'Varlion', 'Lethal Pro'),
    ('PADEL', 'PALA', 'Varlion', 'Bond'),
    ('PADEL', 'SHOES', 'ASICS', 'Gel-Padel Pro'),
    ('PADEL', 'SHOES', 'ASICS', 'Court FF Padel'),
    ('PADEL', 'SHOES', 'Bullpadel', 'Vertex'),
    ('PADEL', 'SHOES', 'Bullpadel', 'Comfort'),
    ('PADEL', 'SHOES', 'Nox', 'ML10 Shoe'),
    ('PADEL', 'SHOES', 'Nox', 'AT10 Shoe'),
    ('PADEL', 'SHOES', 'Joma', 'Slam'),
    ('PADEL', 'SHOES', 'Joma', 'T.Ace'),
    ('PADEL', 'OVERGRIP', 'Babolat', 'Pro Tacky'),
    ('PADEL', 'OVERGRIP', 'Babolat', 'VS Original'),
    ('PADEL', 'OVERGRIP', 'Wilson', 'Pro Overgrip'),
    ('PADEL', 'OVERGRIP', 'Wilson', 'Sublime'),
    ('PADEL', 'OVERGRIP', 'Nox', 'Comfort Grip'),
    ('PADEL', 'OVERGRIP', 'Nox', 'Pro Grip'),
    ('PADEL', 'OVERGRIP', 'Bullpadel', 'GB1200'),
    ('PADEL', 'OVERGRIP', 'Bullpadel', 'Comfort Grip'),
    ('SQUASH', 'RACKET', 'Tecnifibre', 'Carboflex 125'),
    ('SQUASH', 'RACKET', 'Tecnifibre', 'Suprem 105'),
    ('SQUASH', 'RACKET', 'Dunlop', 'Blaze Pro'),
    ('SQUASH', 'RACKET', 'Dunlop', 'Sonic Core Pro'),
    ('SQUASH', 'RACKET', 'Head', 'Graphene Touch Radical'),
    ('SQUASH', 'RACKET', 'Head', 'Extreme 135'),
    ('SQUASH', 'RACKET', 'Karakal', 'Raw 115'),
    ('SQUASH', 'RACKET', 'Karakal', 'SN 90'),
    ('SQUASH', 'RACKET', 'Prince', 'Pro Warrior'),
    ('SQUASH', 'RACKET', 'Prince', 'Team Elite'),
    ('SQUASH', 'STRING', 'Tecnifibre', '305 Max'),
    ('SQUASH', 'STRING', 'Tecnifibre', 'X-One'),
    ('SQUASH', 'STRING', 'Ashaway', 'SuperNick XL'),
    ('SQUASH', 'STRING', 'Ashaway', 'PowerNick 18'),
    ('SQUASH', 'STRING', 'Head', 'Sonic Pro'),
    ('SQUASH', 'STRING', 'Head', 'Synthetic Gut'),
    ('SQUASH', 'SHOES', 'ASICS', 'Gel-Blade'),
    ('SQUASH', 'SHOES', 'ASICS', 'Upcourt'),
    ('SQUASH', 'SHOES', 'Head', 'Grid Pro'),
    ('SQUASH', 'SHOES', 'Head', 'Nitro'),
    ('SQUASH', 'SHOES', 'Salming', 'Viper'),
    ('SQUASH', 'SHOES', 'Salming', 'Falco'),
    ('PING_PONG', 'BLADE', 'Butterfly', 'Viscaria'),
    ('PING_PONG', 'BLADE', 'Butterfly', 'Timo Boll ALC'),
    ('PING_PONG', 'BLADE', 'Stiga', 'Clipper Wood'),
    ('PING_PONG', 'BLADE', 'Stiga', 'Infinity VPS V'),
    ('PING_PONG', 'BLADE', 'Donic', 'Waldner Senso Carbon'),
    ('PING_PONG', 'BLADE', 'Donic', 'Persson Powerplay'),
    ('PING_PONG', 'BLADE', 'Yasaka', 'Extra Offensive'),
    ('PING_PONG', 'BLADE', 'Yasaka', 'Ma Lin Extra Offensive'),
    ('PING_PONG', 'RUBBER_FH', 'Butterfly', 'Tenergy 05'),
    ('PING_PONG', 'RUBBER_FH', 'Butterfly', 'Dignics 09C'),
    ('PING_PONG', 'RUBBER_FH', 'Butterfly', 'Rozena'),
    ('PING_PONG', 'RUBBER_FH', 'DHS', 'Hurricane 3'),
    ('PING_PONG', 'RUBBER_FH', 'DHS', 'Hurricane 8'),
    ('PING_PONG', 'RUBBER_FH', 'Tibhar', 'Evolution MX-P'),
    ('PING_PONG', 'RUBBER_FH', 'Tibhar', 'Grass D.Tecs'),
    ('PING_PONG', 'RUBBER_FH', 'Stiga', 'Mantra M'),
    ('PING_PONG', 'RUBBER_FH', 'Stiga', 'Calibra LT'),
    ('PING_PONG', 'RUBBER_FH', 'Joola', 'Rhyzm P'),
    ('PING_PONG', 'RUBBER_FH', 'Joola', 'Dynaryz AGR'),
    ('PING_PONG', 'RUBBER_FH', 'Donic', 'Bluestorm Z1 Turbo'),
    ('PING_PONG', 'RUBBER_FH', 'Donic', 'Acuda S2'),
    ('PING_PONG', 'RUBBER_BH', 'Butterfly', 'Tenergy 05'),
    ('PING_PONG', 'RUBBER_BH', 'Butterfly', 'Dignics 09C'),
    ('PING_PONG', 'RUBBER_BH', 'Butterfly', 'Rozena'),
    ('PING_PONG', 'RUBBER_BH', 'DHS', 'Hurricane 3'),
    ('PING_PONG', 'RUBBER_BH', 'DHS', 'Hurricane 8'),
    ('PING_PONG', 'RUBBER_BH', 'Tibhar', 'Evolution MX-P'),
    ('PING_PONG', 'RUBBER_BH', 'Tibhar', 'Grass D.Tecs'),
    ('PING_PONG', 'RUBBER_BH', 'Stiga', 'Mantra M'),
    ('PING_PONG', 'RUBBER_BH', 'Stiga', 'Calibra LT'),
    ('PING_PONG', 'RUBBER_BH', 'Joola', 'Rhyzm P'),
    ('PING_PONG', 'RUBBER_BH', 'Joola', 'Dynaryz AGR'),
    ('PING_PONG', 'RUBBER_BH', 'Donic', 'Bluestorm Z1 Turbo'),
    ('PING_PONG', 'RUBBER_BH', 'Donic', 'Acuda S2'),
    ('PICKLEBALL', 'PADDLE', 'Joola', 'Hyperion CFS'),
    ('PICKLEBALL', 'PADDLE', 'Joola', 'Perseus'),
    ('PICKLEBALL', 'PADDLE', 'Selkirk', 'Vanguard Power Air'),
    ('PICKLEBALL', 'PADDLE', 'Selkirk', 'Luxx Control Air'),
    ('PICKLEBALL', 'PADDLE', 'CRBN', 'CRBN1'),
    ('PICKLEBALL', 'PADDLE', 'CRBN', 'CRBN2'),
    ('PICKLEBALL', 'PADDLE', 'Six Zero', 'Double Black Diamond'),
    ('PICKLEBALL', 'PADDLE', 'Six Zero', 'Icon'),
    ('PICKLEBALL', 'PADDLE', 'Franklin', 'Signature Pro'),
    ('PICKLEBALL', 'PADDLE', 'Franklin', 'Ben Johns Falcon'),
    ('PICKLEBALL', 'PADDLE', 'Gamma', 'Shard'),
    ('PICKLEBALL', 'PADDLE', 'Gamma', 'Micro'),
    ('PICKLEBALL', 'PADDLE', 'Paddletek', 'Bantam EX-L'),
    ('PICKLEBALL', 'PADDLE', 'Paddletek', 'Tempest Wave'),
    ('PICKLEBALL', 'SHOES', 'ASICS', 'Gel-Renma'),
    ('PICKLEBALL', 'SHOES', 'ASICS', 'Court FF 3'),
    ('PICKLEBALL', 'SHOES', 'Nike', 'Zoom Vapor Cage'),
    ('PICKLEBALL', 'SHOES', 'Nike', 'Court Air Zoom'),
    ('PICKLEBALL', 'SHOES', 'Adidas', 'CourtJam'),
    ('PICKLEBALL', 'SHOES', 'Adidas', 'GameCourt'),
    ('PICKLEBALL', 'OVERGRIP', 'Joola', 'Tacky Touch'),
    ('PICKLEBALL', 'OVERGRIP', 'Joola', 'Spin Grip'),
    ('PICKLEBALL', 'OVERGRIP', 'Selkirk', 'Sure Grip'),
    ('PICKLEBALL', 'OVERGRIP', 'Selkirk', 'Pro Grip'),
    ('PICKLEBALL', 'OVERGRIP', 'Gamma', 'Tac'),
    ('PICKLEBALL', 'OVERGRIP', 'Gamma', 'EZ Wrap')
) AS v(sport, code, brand, model)
JOIN equipment_categories c ON c.sport = v.sport AND c.code = v.code
JOIN equipment_brands b ON b.name = v.brand
ON CONFLICT (category_id, brand_id, name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- equipment: EXTEND ONLY (relax / add). Existing column meaning is unchanged and the
-- old uq_equipment_active (user_id, type) index is kept for rollback safety.
-- ---------------------------------------------------------------------------
ALTER TABLE equipment
    ALTER COLUMN type           DROP NOT NULL,
    ALTER COLUMN name           DROP NOT NULL,
    ALTER COLUMN lifespan_hours DROP NOT NULL;
ALTER TABLE equipment DROP CONSTRAINT IF EXISTS equipment_type_check;
ALTER TABLE equipment DROP CONSTRAINT IF EXISTS equipment_lifespan_hours_check;
ALTER TABLE equipment
    ADD CONSTRAINT equipment_lifespan_hours_check CHECK (lifespan_hours IS NULL OR lifespan_hours > 0);

ALTER TABLE equipment
    ADD COLUMN IF NOT EXISTS sport             TEXT,
    ADD COLUMN IF NOT EXISTS category_id       UUID REFERENCES equipment_categories(id),
    ADD COLUMN IF NOT EXISTS brand_id          UUID REFERENCES equipment_brands(id),
    ADD COLUMN IF NOT EXISTS model_id          UUID REFERENCES equipment_models(id),
    ADD COLUMN IF NOT EXISTS custom_brand_name TEXT,
    ADD COLUMN IF NOT EXISTS custom_model_name TEXT,
    ADD COLUMN IF NOT EXISTS nickname          TEXT,
    ADD COLUMN IF NOT EXISTS attributes        JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS lifespan_days     INT,
    ADD COLUMN IF NOT EXISTS parent_item_id    UUID REFERENCES equipment(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS lineage_id        UUID,
    ADD COLUMN IF NOT EXISTS retired_at        TIMESTAMPTZ;

ALTER TABLE equipment DROP CONSTRAINT IF EXISTS equipment_lifespan_days_check;
ALTER TABLE equipment DROP CONSTRAINT IF EXISTS equipment_not_active_and_retired;
ALTER TABLE equipment
    ADD CONSTRAINT equipment_lifespan_days_check CHECK (lifespan_days IS NULL OR lifespan_days > 0),
    ADD CONSTRAINT equipment_not_active_and_retired CHECK (NOT (is_active AND retired_at IS NOT NULL));

-- AC-22 backfill (categories were seeded above): STRING -> Tennis/STRING, SHOES -> Tennis/SHOES.
-- accumulated_seconds / is_active / notified_at / name stay untouched.
UPDATE equipment e SET
    sport = 'TENNIS',
    category_id = c.id,
    lineage_id = COALESCE(e.lineage_id, e.id)
FROM equipment_categories c
WHERE e.category_id IS NULL AND e.type = 'STRING' AND c.sport = 'TENNIS' AND c.code = 'STRING';

UPDATE equipment e SET
    sport = 'TENNIS',
    category_id = c.id,
    lineage_id = COALESCE(e.lineage_id, e.id)
FROM equipment_categories c
WHERE e.category_id IS NULL AND e.type = 'SHOES' AND c.sport = 'TENNIS' AND c.code = 'SHOES';

UPDATE equipment SET lineage_id = id WHERE lineage_id IS NULL;
ALTER TABLE equipment ALTER COLUMN lineage_id SET NOT NULL;

-- Rows inserted by the previous binary (rollback) do not know lineage_id: default it to
-- the row id so the NOT NULL above never breaks an old INSERT.
CREATE OR REPLACE FUNCTION equipment_set_lineage() RETURNS trigger AS $$
BEGIN
    IF NEW.lineage_id IS NULL THEN
        NEW.lineage_id := NEW.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_equipment_set_lineage ON equipment;
CREATE TRIGGER trg_equipment_set_lineage
    BEFORE INSERT ON equipment
    FOR EACH ROW EXECUTE FUNCTION equipment_set_lineage();

CREATE UNIQUE INDEX IF NOT EXISTS uq_equipment_active_category
    ON equipment (user_id, category_id) WHERE is_active AND retired_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_equipment_user_sport_active ON equipment (user_id, sport) WHERE retired_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_equipment_lineage ON equipment (lineage_id);
CREATE INDEX IF NOT EXISTS idx_equipment_parent ON equipment (parent_item_id);

-- ---------------------------------------------------------------------------
-- Forward-only match <-> equipment combo usage (AC-16/17). match_id is a SOFT reference
-- (matches is partitioned with composite PK (id, played_at); no FK, same as feed activities).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS match_equipment_usage (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id    UUID NOT NULL,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sport       TEXT NOT NULL,
    combo_key   TEXT NOT NULL,
    item_ids    UUID[] NOT NULL,
    won         BOOLEAN NOT NULL,
    played_at   TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_meu_user_sport_combo ON match_equipment_usage (user_id, sport, combo_key);
CREATE INDEX IF NOT EXISTS idx_meu_match ON match_equipment_usage (match_id);
