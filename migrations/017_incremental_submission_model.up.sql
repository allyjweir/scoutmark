-- Migration 017: Incrementally simplify score persistence around submissions
-- Keep existing tables/data, but make submissions the canonical store for
-- in-progress and finalised scores.

-- Add mutable timestamp and uniqueness to submission comments so they can be
-- edited before finalisation.
ALTER TABLE submission_comments
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE submission_comments
DROP CONSTRAINT IF EXISTS submission_comments_submission_id_criterion_id_user_id_key;

ALTER TABLE submission_comments
ADD CONSTRAINT submission_comments_submission_id_criterion_id_user_id_key
UNIQUE (submission_id, criterion_id, user_id);

-- Helpful index for mixed draft/finalised reads.
CREATE INDEX IF NOT EXISTS idx_submissions_session_locked
ON submissions(session_id, locked);

-- Track mutable in-progress edit time for submission-backed drafts.
ALTER TABLE submissions
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE submissions
SET updated_at = submitted_at;

-- Backfill in-progress drafts into unlocked submissions where no submission
-- exists yet. This preserves production in-progress data without dropping
-- legacy tables.
INSERT INTO submissions (id, submitted_by, session_id, patrol_id, locked, submitted_at)
SELECT
    d.id AS id,
    src.submitted_by,
    d.session_id,
    d.patrol_id,
    FALSE,
    d.updated_at
FROM drafts d
CROSS JOIN LATERAL (
    SELECT COALESCE(
        (
            SELECT ds.last_edited_by
            FROM draft_scores ds
            WHERE ds.draft_id = d.id AND ds.last_edited_by IS NOT NULL
            ORDER BY ds.last_edited_at DESC
            LIMIT 1
        ),
        (
            SELECT dc.user_id
            FROM draft_comments dc
            WHERE dc.draft_id = d.id
            ORDER BY dc.updated_at DESC
            LIMIT 1
        ),
        (
            SELECT u.id
            FROM patrols p
            JOIN users u ON u.subcamp_id = p.subcamp_id
            WHERE p.id = d.patrol_id
            ORDER BY u.created_at ASC
            LIMIT 1
        )
    ) AS submitted_by
) src
WHERE src.submitted_by IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM submissions s
      WHERE s.id = d.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM submissions s
      WHERE s.session_id = d.session_id AND s.patrol_id = d.patrol_id
  );

-- Backfill draft scores into submission_scores for newly/unlocked submissions.
INSERT INTO submission_scores (id, submission_id, criterion_id, value, scored_by)
SELECT
    ds.id AS id,
    s.id,
    ds.criterion_id,
    ds.value,
    ds.last_edited_by
FROM draft_scores ds
JOIN drafts d ON d.id = ds.draft_id
JOIN submissions s ON s.session_id = d.session_id AND s.patrol_id = d.patrol_id
WHERE NOT EXISTS (
    SELECT 1
    FROM submission_scores ss
    WHERE ss.id = ds.id
)
  AND NOT EXISTS (
    SELECT 1
    FROM submission_scores ss
    WHERE ss.submission_id = s.id AND ss.criterion_id = ds.criterion_id
);

-- Backfill draft comments into submission comments where missing.
INSERT INTO submission_comments (id, submission_id, criterion_id, user_id, display_name, comment, created_at, updated_at)
SELECT
    dc.id AS id,
    s.id,
    dc.criterion_id,
    dc.user_id,
    dc.display_name,
    dc.comment,
    dc.created_at,
    dc.updated_at
FROM draft_comments dc
JOIN drafts d ON d.id = dc.draft_id
JOIN submissions s ON s.session_id = d.session_id AND s.patrol_id = d.patrol_id
WHERE COALESCE(trim(dc.comment), '') <> ''
  AND NOT EXISTS (
      SELECT 1
      FROM submission_comments sc
      WHERE sc.id = dc.id
  )
  AND NOT EXISTS (
      SELECT 1
      FROM submission_comments sc
      WHERE sc.submission_id = s.id
        AND sc.criterion_id = dc.criterion_id
        AND sc.user_id = dc.user_id
  );
