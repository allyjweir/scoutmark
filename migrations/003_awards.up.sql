-- Add previous_session_id to sessions for linked-list chaining (e.g. Mon → Tue → Wed)
ALTER TABLE
    sessions
ADD
    COLUMN previous_session_id VARCHAR(36) REFERENCES sessions(id) ON DELETE
SET
    NULL;

-- Award flags on sessions (opt-in per session)
ALTER TABLE
    sessions
ADD
    COLUMN award_best_patrol BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE
    sessions
ADD
    COLUMN award_most_improved BOOLEAN NOT NULL DEFAULT FALSE;

-- User award selections (auto-calculated, user-overridable, saved incrementally)
CREATE TABLE session_awards (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    award_type VARCHAR(50) NOT NULL,
    -- 'best_patrol' or 'most_improved'
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_id, award_type)
);

CREATE INDEX idx_session_awards_session ON session_awards(session_id);