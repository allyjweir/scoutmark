-- Migration 013 down: remove round 2 support and explicit session patrol scoping
DROP TRIGGER IF EXISTS trg_enforce_round2_session_patrols_source_subcamp ON session_patrols;
DROP FUNCTION IF EXISTS enforce_round2_session_patrols_source_subcamp();

DROP TABLE IF EXISTS session_patrols;

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_round_type_check;

ALTER TABLE sessions
DROP COLUMN IF EXISTS source_session_id;

ALTER TABLE sessions
DROP COLUMN IF EXISTS round_type;
