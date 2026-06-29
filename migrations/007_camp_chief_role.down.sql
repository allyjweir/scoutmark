-- Reverse camp chief role migration
DROP INDEX IF EXISTS idx_chief_scores_round;
DROP INDEX IF EXISTS idx_chief_round_patrols_round;
DROP INDEX IF EXISTS idx_chief_rounds_session;

DROP TABLE IF EXISTS chief_scores;
DROP TABLE IF EXISTS chief_round_patrols;
DROP TABLE IF EXISTS chief_rounds;

-- Restore is_admin boolean
ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE users SET is_admin = TRUE WHERE role = 'admin';
ALTER TABLE users DROP COLUMN role;
