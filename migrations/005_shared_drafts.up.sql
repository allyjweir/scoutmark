-- Migration 005: Shared drafts
-- Changes drafts from per-user to per-patrol (shared), adds attribution tracking,
-- and changes submissions from per-user to per-patrol.
-- ─── Drafts: remove user_id ownership, add attribution ──────────────
-- Drop the old unique constraint and index
ALTER TABLE
    drafts DROP CONSTRAINT IF EXISTS drafts_user_id_session_id_patrol_id_key;

-- Drop user_id column (no longer per-user)
ALTER TABLE
    drafts DROP COLUMN user_id;

-- Add new unique constraint: one shared draft per patrol per session
ALTER TABLE
    drafts
ADD
    CONSTRAINT drafts_session_patrol_unique UNIQUE (session_id, patrol_id);

-- Add attribution columns to draft_scores
ALTER TABLE
    draft_scores
ADD
    COLUMN last_edited_by VARCHAR(36) REFERENCES users(id) ON DELETE
SET
    NULL;

ALTER TABLE
    draft_scores
ADD
    COLUMN last_edited_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ─── Submissions: remove user_id ownership, add submitted_by ────────
-- Drop the old unique constraint
ALTER TABLE
    submissions DROP CONSTRAINT IF EXISTS submissions_user_id_session_id_patrol_id_key;

-- Rename user_id → submitted_by for clarity (who pressed submit)
ALTER TABLE
    submissions RENAME COLUMN user_id TO submitted_by;

-- Add new unique constraint: one submission per patrol per session
ALTER TABLE
    submissions
ADD
    CONSTRAINT submissions_session_patrol_unique UNIQUE (session_id, patrol_id);

-- Add attribution to submission scores
ALTER TABLE
    submission_scores
ADD
    COLUMN scored_by VARCHAR(36) REFERENCES users(id) ON DELETE
SET
    NULL;