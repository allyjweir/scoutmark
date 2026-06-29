-- Subcamps: a named group of patrols. Each patrol belongs to exactly one
-- subcamp and scorers are assigned to subcamps (many scorers per subcamp).
-- This replaces the direct user-patrol assignment model.

CREATE TABLE subcamps (
    id VARCHAR(36) PRIMARY KEY,
    event_id VARCHAR(36) NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_subcamps_event ON subcamps(event_id);

-- Every patrol belongs to exactly one subcamp.
ALTER TABLE patrols ADD COLUMN subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE;
CREATE INDEX idx_patrols_subcamp ON patrols(subcamp_id);

-- Scorers are assigned to subcamps and their patrols derive from the subcamp.
CREATE TABLE user_subcamps (
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE,
    sort_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, subcamp_id)
);

CREATE INDEX idx_user_subcamps_order ON user_subcamps(user_id, sort_order);

DROP TABLE user_patrols;

-- Chief round entrants are now per-subcamp (top patrol per subcamp), not per-scorer.
ALTER TABLE chief_round_patrols DROP COLUMN scorer_user_id;
ALTER TABLE chief_round_patrols ADD COLUMN subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE;
