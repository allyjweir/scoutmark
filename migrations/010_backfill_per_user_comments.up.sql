-- Migration 010: Backfill legacy score-level comments into per-user comment tables
--
-- Prior versions stored one comment in draft_scores.comment / submission_scores.comment.
-- Newer flows read comments from draft_comments / submission_comments.
-- This migration copies any non-empty legacy comments that do not already have
-- a per-user comment row for the same draft/submission + criterion + user.

-- Backfill draft comments from draft_scores.comment (attributed to last_edited_by)
INSERT INTO draft_comments (id, draft_id, criterion_id, user_id, display_name, comment, created_at, updated_at)
SELECT
    'mig-' || substr(md5(ds.id || ':draft'), 1, 32) AS id,
    ds.draft_id,
    ds.criterion_id,
    ds.last_edited_by,
    COALESCE(u.display_name, ''),
    ds.comment,
    COALESCE(ds.last_edited_at, NOW()),
    COALESCE(ds.last_edited_at, NOW())
FROM draft_scores ds
LEFT JOIN users u ON u.id = ds.last_edited_by
WHERE ds.last_edited_by IS NOT NULL
  AND COALESCE(trim(ds.comment), '') <> ''
  AND NOT EXISTS (
      SELECT 1
      FROM draft_comments dc
      WHERE dc.draft_id = ds.draft_id
        AND dc.criterion_id = ds.criterion_id
        AND dc.user_id = ds.last_edited_by
  );

-- Backfill submission comments from submission_scores.comment (attributed to scored_by)
INSERT INTO submission_comments (id, submission_id, criterion_id, user_id, display_name, comment, created_at)
SELECT
    'mig-' || substr(md5(ss.id || ':submission'), 1, 32) AS id,
    ss.submission_id,
    ss.criterion_id,
    ss.scored_by,
    COALESCE(u.display_name, ''),
    ss.comment,
    COALESCE(s.submitted_at, NOW())
FROM submission_scores ss
JOIN submissions s ON s.id = ss.submission_id
LEFT JOIN users u ON u.id = ss.scored_by
WHERE ss.scored_by IS NOT NULL
  AND COALESCE(trim(ss.comment), '') <> ''
  AND NOT EXISTS (
      SELECT 1
      FROM submission_comments sc
      WHERE sc.submission_id = ss.submission_id
        AND sc.criterion_id = ss.criterion_id
        AND sc.user_id = ss.scored_by
  );
