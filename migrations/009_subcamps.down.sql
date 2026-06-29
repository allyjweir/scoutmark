ALTER TABLE chief_round_patrols DROP COLUMN subcamp_id;
ALTER TABLE chief_round_patrols ADD COLUMN scorer_user_id VARCHAR(36) NOT NULL REFERENCES users(id);

CREATE TABLE user_patrols (
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE CASCADE,
    sort_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, patrol_id)
);

CREATE INDEX idx_user_patrols_order ON user_patrols(user_id, sort_order);

ALTER TABLE patrols DROP COLUMN subcamp_id;

DROP TABLE user_subcamps;
DROP TABLE subcamps;
