-- Migration 011: Remove legacy score-level comment columns
--
-- Comments now live in draft_comments and submission_comments.
-- These columns were kept temporarily for backfill compatibility.
ALTER TABLE draft_scores DROP COLUMN IF EXISTS comment;

ALTER TABLE submission_scores DROP COLUMN IF EXISTS comment;
