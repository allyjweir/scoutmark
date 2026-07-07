-- Rollback Migration 008: Restore user_patrols-based model
CREATE TABLE IF NOT EXISTS user_patrols (
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE CASCADE,
    sort_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, patrol_id)
);

CREATE INDEX IF NOT EXISTS idx_user_patrols_order ON user_patrols(user_id, sort_order);

-- Reconstruct assignments from user->subcamp and patrol->subcamp.
INSERT INTO
    user_patrols (user_id, patrol_id, sort_order)
SELECT
    u.id,
    p.id,
    p.sort_order
FROM
    users u
    JOIN patrols p ON p.subcamp_id = u.subcamp_id
WHERE
    u.subcamp_id IS NOT NULL ON CONFLICT (user_id, patrol_id) DO
UPDATE
SET
    sort_order = EXCLUDED.sort_order;

DROP TABLE IF EXISTS session_subcamps;

DROP INDEX IF EXISTS idx_patrols_subcamp_order;

ALTER TABLE
    patrols DROP COLUMN IF EXISTS sort_order;

ALTER TABLE
    patrols DROP COLUMN IF EXISTS subcamp_id;

ALTER TABLE
    users DROP COLUMN IF EXISTS subcamp_id;

DELETE FROM
    subcamps
WHERE
    id = 'subcamp-legacy';

DROP TABLE IF EXISTS subcamps;