-- Migration 013: Round 2 session support and explicit session patrol scoping
ALTER TABLE sessions
ADD COLUMN IF NOT EXISTS round_type TEXT NOT NULL DEFAULT 'regular';

ALTER TABLE sessions
ADD COLUMN IF NOT EXISTS source_session_id VARCHAR(36) UNIQUE REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE sessions
DROP CONSTRAINT IF EXISTS sessions_round_type_check;

ALTER TABLE sessions
ADD CONSTRAINT sessions_round_type_check CHECK (round_type IN ('regular', 'round2'));

CREATE INDEX IF NOT EXISTS idx_sessions_source_session_id ON sessions(source_session_id);

CREATE TABLE IF NOT EXISTS session_patrols (
    session_id VARCHAR(36) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    subcamp_id VARCHAR(36) NOT NULL REFERENCES subcamps(id) ON DELETE CASCADE,
    patrol_id VARCHAR(36) NOT NULL REFERENCES patrols(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (session_id, subcamp_id) REFERENCES session_subcamps(session_id, subcamp_id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, subcamp_id),
    UNIQUE (session_id, patrol_id)
);

CREATE INDEX IF NOT EXISTS idx_session_patrols_session ON session_patrols(session_id);
CREATE INDEX IF NOT EXISTS idx_session_patrols_patrol ON session_patrols(patrol_id);

CREATE OR REPLACE FUNCTION enforce_round2_session_patrols_source_subcamp()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    session_round_type TEXT;
    source_id VARCHAR(36);
BEGIN
    SELECT s.round_type, s.source_session_id
      INTO session_round_type, source_id
      FROM sessions s
     WHERE s.id = NEW.session_id;

    IF session_round_type = 'round2' THEN
        IF source_id IS NULL THEN
            RAISE EXCEPTION 'round2 session % has no source_session_id', NEW.session_id;
        END IF;

        IF NOT EXISTS (
            SELECT 1
              FROM session_subcamps ss
             WHERE ss.session_id = source_id
               AND ss.subcamp_id = NEW.subcamp_id
        ) THEN
            RAISE EXCEPTION 'subcamp % is not part of source session %', NEW.subcamp_id, source_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_round2_session_patrols_source_subcamp ON session_patrols;

CREATE TRIGGER trg_enforce_round2_session_patrols_source_subcamp
BEFORE INSERT OR UPDATE ON session_patrols
FOR EACH ROW
EXECUTE FUNCTION enforce_round2_session_patrols_source_subcamp();
