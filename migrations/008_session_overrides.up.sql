-- Track manual open/close override actions on sessions as independent events.
CREATE TABLE session_overrides (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    action VARCHAR(16) NOT NULL CHECK (action IN ('close', 'reopen')),
    actor_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_session_overrides_session ON session_overrides(session_id, created_at DESC);
