-- Round 2 sessions are independently scheduled and configured.
ALTER TABLE sessions DROP COLUMN IF EXISTS source_session_id;
