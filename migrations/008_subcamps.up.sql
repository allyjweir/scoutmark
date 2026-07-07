-- Migration 008: Introduce explicit subcamps and session-to-subcamp scoping
CREATE TABLE IF NOT EXISTS subcamps (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Temporary legacy subcamp for pre-existing rows while migrating away from user_patrols.
INSERT INTO
    subcamps (id, name)
VALUES
    ('subcamp-legacy', 'Legacy') ON CONFLICT (id) DO NOTHING;

ALTER TABLE
    users
ADD
    COLUMN IF NOT EXISTS subcamp_id VARCHAR(36) REFERENCES subcamps(id) ON DELETE
SET
    NULL;

ALTER TABLE
    patrols
ADD
    COLUMN IF NOT EXISTS subcamp_id VARCHAR(36) REFERENCES subcamps(id) ON DELETE RESTRICT;

ALTER TABLE
    patrols
ADD
    COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;

UPDATE
    patrols
SET
    subcamp_id = 'subcamp-legacy'
WHERE
    subcamp_id IS NULL;

-- Preserve old behavior for non-admin users during migration.
UPDATE
    users
SET
    subcamp_id = 'subcamp-legacy'
WHERE
    subcamp_id IS NULL
    AND is_admin = FALSE;

ALTER TABLE
    patrols
ALTER COLUMN
    subcamp_id
SET
    NOT NULL;

CREATE INDEX IF NOT EXISTS idx_patrols_subcamp_order ON patrols(subcamp_id, sort_order);

CREATE TABLE IF NOT EXISTS session_subcamps (
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, subcamp_id)
);

CREATE INDEX IF NOT EXISTS idx_session_subcamps_subcamp ON session_subcamps(subcamp_id);

-- Existing sessions include all existing subcamps by default.
INSERT INTO
    session_subcamps (session_id, subcamp_id)
SELECT
    s.id,
    sc.id
FROM
    sessions s
    CROSS JOIN subcamps sc ON CONFLICT (session_id, subcamp_id) DO NOTHING;

DROP INDEX IF EXISTS idx_user_patrols_order;

DROP TABLE IF EXISTS user_patrols;