CREATE TABLE IF NOT EXISTS session_subcamp_locks (
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE,
    locked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_by VARCHAR(36) REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (session_id, subcamp_id)
);
