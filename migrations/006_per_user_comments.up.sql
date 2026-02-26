-- Migration 006: Per-user comments
-- Changes comments from single-field-on-score to per-user entries.
-- Each user can leave their own comment on each criterion within a draft/submission.
-- Per-user comments on draft criteria
CREATE TABLE IF NOT EXISTS draft_comments (
    id VARCHAR(36) PRIMARY KEY,
    draft_id VARCHAR(36) NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
    criterion_id VARCHAR(36) NOT NULL REFERENCES criteria(id) ON DELETE CASCADE,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (draft_id, criterion_id, user_id)
);

-- Per-user comments snapshot at submission time
CREATE TABLE IF NOT EXISTS submission_comments (
    id VARCHAR(36) PRIMARY KEY,
    submission_id VARCHAR(36) NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    criterion_id VARCHAR(36) NOT NULL REFERENCES criteria(id) ON DELETE CASCADE,
    user_id VARCHAR(36) NOT NULL,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);