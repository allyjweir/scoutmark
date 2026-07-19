-- Migration 013 down: remove round 2 support and explicit session patrol scoping
DROP TABLE IF EXISTS session_patrols;

ALTER TABLE
    sessions DROP CONSTRAINT IF EXISTS sessions_round_type_check;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS source_session_id;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS round_type;