-- Rollback migration 005: Restore per-user drafts and submissions
-- Remove attribution from submission_scores
ALTER TABLE
    submission_scores DROP COLUMN IF EXISTS scored_by;

-- Restore submissions to per-user
ALTER TABLE
    submissions DROP CONSTRAINT IF EXISTS submissions_session_patrol_unique;

ALTER TABLE
    submissions RENAME COLUMN submitted_by TO user_id;

ALTER TABLE
    submissions
ADD
    CONSTRAINT submissions_user_id_session_id_patrol_id_key UNIQUE (user_id, session_id, patrol_id);

-- Remove attribution from draft_scores
ALTER TABLE
    draft_scores DROP COLUMN IF EXISTS last_edited_at;

ALTER TABLE
    draft_scores DROP COLUMN IF EXISTS last_edited_by;

-- Restore drafts to per-user
ALTER TABLE
    drafts DROP CONSTRAINT IF EXISTS drafts_session_patrol_unique;

ALTER TABLE
    drafts
ADD
    COLUMN user_id VARCHAR(36) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE
    drafts
ADD
    CONSTRAINT drafts_user_id_session_id_patrol_id_key UNIQUE (user_id, session_id, patrol_id);