DROP TABLE IF EXISTS session_awards;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS award_most_improved;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS award_best_patrol;

ALTER TABLE
    sessions DROP COLUMN IF EXISTS previous_session_id;