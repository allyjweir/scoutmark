-- Migration 013: Round 2 session support and explicit session patrol scoping
ALTER TABLE
    sessions
ADD
    COLUMN IF NOT EXISTS round_type TEXT NOT NULL DEFAULT 'regular';

ALTER TABLE
    sessions
ADD
    COLUMN IF NOT EXISTS source_session_id VARCHAR(36) UNIQUE REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE
    sessions DROP CONSTRAINT IF EXISTS sessions_round_type_check;

ALTER TABLE
    sessions
ADD
    CONSTRAINT sessions_round_type_check CHECK (round_type IN ('regular', 'round2'));

CREATE INDEX IF NOT EXISTS idx_sessions_source_session_id ON sessions(source_session_id);

CREATE TABLE IF NOT EXISTS session_patrols (
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (session_id, subcamp_id) REFERENCES session_subcamps(session_id, subcamp_id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, subcamp_id),
    UNIQUE (session_id, patrol_id)
);

CREATE INDEX IF NOT EXISTS idx_session_patrols_session ON session_patrols(session_id);

CREATE INDEX IF NOT EXISTS idx_session_patrols_patrol ON session_patrols(patrol_id);