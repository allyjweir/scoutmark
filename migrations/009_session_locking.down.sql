-- Rollback Migration 009: Remove explicit session-level lock metadata
DROP INDEX IF EXISTS idx_sessions_locked_at;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS locked_by;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS locked_at;