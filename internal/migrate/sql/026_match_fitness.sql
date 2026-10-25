ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS avg_heart_rate   INT,
    ADD COLUMN IF NOT EXISTS max_heart_rate   INT,
    ADD COLUMN IF NOT EXISTS active_calories  INT,
    ADD COLUMN IF NOT EXISTS fitness_source   TEXT,
    ADD COLUMN IF NOT EXISTS fitness_samples  JSONB;
