-- Replace is_admin boolean with role column.
-- Roles: 'scorer' (default), 'camp_chief', 'admin'
ALTER TABLE users ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'scorer';

-- Migrate existing admins
UPDATE users SET role = 'admin' WHERE is_admin = TRUE;

-- Drop the old boolean
ALTER TABLE users DROP COLUMN is_admin;

-- Chief rounds: one per session, created when all scorers submit
CREATE TABLE chief_rounds (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    winner_patrol_id VARCHAR(36) REFERENCES patrols(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE(session_id)
);

-- The winning patrol from each scorer that enters the chief round
CREATE TABLE chief_round_patrols (
    id VARCHAR(36) PRIMARY KEY,
    chief_round_id VARCHAR(36) NOT NULL REFERENCES chief_rounds(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id),
    scorer_user_id VARCHAR(36) NOT NULL REFERENCES users(id),
    total_score INT NOT NULL,
    UNIQUE(chief_round_id, patrol_id)
);

-- Camp chief's scores for each winning patrol
CREATE TABLE chief_scores (
    id VARCHAR(36) PRIMARY KEY,
    chief_round_id VARCHAR(36) NOT NULL REFERENCES chief_rounds(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id),
    criterion_id VARCHAR(36) NOT NULL REFERENCES criteria(id),
    value INT NOT NULL,
    UNIQUE(chief_round_id, patrol_id, criterion_id)
);

CREATE INDEX idx_chief_rounds_session ON chief_rounds(session_id);
CREATE INDEX idx_chief_round_patrols_round ON chief_round_patrols(chief_round_id);
CREATE INDEX idx_chief_scores_round ON chief_scores(chief_round_id);
