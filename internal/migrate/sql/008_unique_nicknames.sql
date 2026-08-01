-- Nicknames must be unique (case-insensitive) so friend search cannot
-- ambiguously target two accounts that share the same display name.
-- Existing duplicates keep the oldest row; later ones get a short id suffix.

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY LOWER(TRIM(nickname))
               ORDER BY created_at ASC, id ASC
           ) AS rn
    FROM users
    WHERE nickname IS NOT NULL AND TRIM(nickname) <> ''
)
UPDATE users u
SET nickname = LEFT(TRIM(u.nickname), 24) || '_' || SUBSTRING(REPLACE(u.id::text, '-', ''), 1, 4),
    updated_at = NOW()
FROM ranked r
WHERE u.id = r.id AND r.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_nickname_lower
    ON users (LOWER(TRIM(nickname)))
    WHERE nickname IS NOT NULL AND TRIM(nickname) <> '';
