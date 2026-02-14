-- Add optional written comments per criterion in drafts and submissions
ALTER TABLE
    draft_scores
ADD
    COLUMN comment TEXT NOT NULL DEFAULT '';

ALTER TABLE
    submission_scores
ADD
    COLUMN comment TEXT NOT NULL DEFAULT '';