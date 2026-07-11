-- Migration 009: Add explicit session-level lock metadata
ALTER TABLE
    sessions
ADD
    COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ;

ALTER TABLE
    sessions
ADD
    COLUMN IF NOT EXISTS locked_by VARCHAR(36) REFERENCES users(id) ON DELETE
SET
    NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_locked_at ON sessions(locked_at);