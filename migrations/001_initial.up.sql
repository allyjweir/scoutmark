-- Scoutmark initial schema (PostgreSQL)
CREATE TABLE events (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_sessions (
    token VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE patrols (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_patrols (
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE CASCADE,
    sort_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, patrol_id)
);

CREATE INDEX idx_user_patrols_order ON user_patrols(user_id, sort_order);

CREATE TABLE criteria_templates (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE criteria (
    id VARCHAR(36) PRIMARY KEY,
    template_id VARCHAR(36) NOT NULL REFERENCES criteria_templates(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    min_value INT NOT NULL DEFAULT 0,
    max_value INT NOT NULL DEFAULT 10,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_criteria_template ON criteria(template_id, sort_order);

CREATE TABLE sessions (
    id VARCHAR(36) PRIMARY KEY,
    event_id VARCHAR(36) NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    template_id VARCHAR(36) NOT NULL REFERENCES criteria_templates(id) ON DELETE RESTRICT,
    name VARCHAR(255) NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_event ON sessions(event_id);

CREATE INDEX idx_sessions_time ON sessions(starts_at, ends_at);

CREATE TABLE drafts (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_id, patrol_id)
);

CREATE TABLE draft_scores (
    id VARCHAR(36) PRIMARY KEY,
    draft_id VARCHAR(36) NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
    criterion_id VARCHAR(36) NOT NULL REFERENCES criteria(id) ON DELETE RESTRICT,
    value INT NOT NULL,
    UNIQUE (draft_id, criterion_id)
);

CREATE TABLE submissions (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE CASCADE,
    locked BOOLEAN NOT NULL DEFAULT TRUE,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_id, patrol_id)
);

CREATE INDEX idx_submissions_session ON submissions(session_id);

CREATE TABLE submission_scores (
    id VARCHAR(36) PRIMARY KEY,
    submission_id VARCHAR(36) NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    criterion_id VARCHAR(36) NOT NULL REFERENCES criteria(id) ON DELETE RESTRICT,
    value INT NOT NULL,
    UNIQUE (submission_id, criterion_id)
);