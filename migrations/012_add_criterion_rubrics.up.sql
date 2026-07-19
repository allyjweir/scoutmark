ALTER TABLE
    criteria
ADD
    COLUMN IF NOT EXISTS rubric_checklist JSONB NOT NULL DEFAULT '[]' :: jsonb;

ALTER TABLE
    criteria
ADD
    COLUMN IF NOT EXISTS rubric_bands JSONB NOT NULL DEFAULT '[]' :: jsonb;