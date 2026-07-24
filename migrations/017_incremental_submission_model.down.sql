ALTER TABLE submission_comments
DROP CONSTRAINT IF EXISTS submission_comments_submission_id_criterion_id_user_id_key;

ALTER TABLE submission_comments
DROP COLUMN IF EXISTS updated_at;

DROP INDEX IF EXISTS idx_submissions_session_locked;

ALTER TABLE submissions
DROP COLUMN IF EXISTS updated_at;
