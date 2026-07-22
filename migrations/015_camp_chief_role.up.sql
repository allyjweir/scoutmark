ALTER TABLE users ADD COLUMN IF NOT EXISTS is_camp_chief BOOLEAN NOT NULL DEFAULT FALSE;

-- Preserve the existing seeded camp-chief account while moving authorization to an explicit role.
UPDATE users SET is_camp_chief = TRUE WHERE id = 'usr-campchief';
