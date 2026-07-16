-- Rollback for migration 011: restore legacy score-level comment columns
ALTER TABLE draft_scores ADD COLUMN IF NOT EXISTS comment TEXT NOT NULL DEFAULT '';

ALTER TABLE submission_scores ADD COLUMN IF NOT EXISTS comment TEXT NOT NULL DEFAULT '';
