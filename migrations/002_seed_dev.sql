-- Scoutmark development seed data
-- Covers both use cases: weekly meetings and summer camp

-- ─── Events ────────────────────────────────────────────────────────

INSERT INTO events (id, name, description) VALUES
  ('evt-weekly-2026', 'Weekly Meetings 2026', 'Regular Tuesday night meetings'),
  ('evt-camp-2026', 'Summer Camp 2026', 'Annual summer camp at Lochgoilhead');

-- ─── Criteria Templates ────────────────────────────────────────────

-- Weekly meeting inspection template
INSERT INTO criteria_templates (id, name, description) VALUES
  ('tpl-weekly', 'Weekly Inspection', 'Standard weekly meeting patrol inspection');

INSERT INTO criteria (id, template_id, title, description, min_value, max_value, sort_order) VALUES
  ('crt-w-uniform', 'tpl-weekly', 'Uniform', 'Correct and complete uniform', 0, 10, 1),
  ('crt-w-behaviour', 'tpl-weekly', 'Behaviour', 'Conduct and teamwork during the meeting', 0, 10, 2),
  ('crt-w-patrol-corner', 'tpl-weekly', 'Patrol Corner', 'Tidiness and presentation of patrol corner', 0, 10, 3),
  ('crt-w-attendance', 'tpl-weekly', 'Attendance', 'Number of patrol members present', 0, 10, 4);

-- Camp inspection template
INSERT INTO criteria_templates (id, name, description) VALUES
  ('tpl-camp', 'Camp Inspection', 'Daily morning camp inspection criteria');

INSERT INTO criteria (id, template_id, title, description, min_value, max_value, sort_order) VALUES
  ('crt-c-tent', 'tpl-camp', 'Tent & Bedding', 'Tents properly pitched, bedding aired and rolled', 0, 10, 1),
  ('crt-c-kit', 'tpl-camp', 'Kit Layout', 'Personal kit neatly stored and accessible', 0, 10, 2),
  ('crt-c-hygiene', 'tpl-camp', 'Hygiene Area', 'Wash stands clean, toiletries organised', 0, 10, 3),
  ('crt-c-kitchen', 'tpl-camp', 'Kitchen Area', 'Cooking area clean, fire properly maintained', 0, 10, 4),
  ('crt-c-gadgets', 'tpl-camp', 'Camp Gadgets', 'Quality and creativity of pioneering projects', 0, 10, 5),
  ('crt-c-spirit', 'tpl-camp', 'Camp Spirit', 'Enthusiasm, teamwork and patrol morale', 0, 10, 6);

-- ─── Patrols ───────────────────────────────────────────────────────

-- Weekly meeting patrols
INSERT INTO patrols (id, name) VALUES
  ('pat-eagles', 'Eagles'),
  ('pat-foxes', 'Foxes');

-- Camp patrols (Subcamp Alpha - 6 patrols per subcamp for demo)
INSERT INTO patrols (id, name) VALUES
  ('pat-a1', 'Alpha 1 - Otters'),
  ('pat-a2', 'Alpha 2 - Hawks'),
  ('pat-a3', 'Alpha 3 - Badgers'),
  ('pat-a4', 'Alpha 4 - Wolves'),
  ('pat-a5', 'Alpha 5 - Stags'),
  ('pat-a6', 'Alpha 6 - Ravens');

-- Camp patrols (Subcamp Bravo)
INSERT INTO patrols (id, name) VALUES
  ('pat-b1', 'Bravo 1 - Seals'),
  ('pat-b2', 'Bravo 2 - Falcons'),
  ('pat-b3', 'Bravo 3 - Hares'),
  ('pat-b4', 'Bravo 4 - Bears'),
  ('pat-b5', 'Bravo 5 - Lynx'),
  ('pat-b6', 'Bravo 6 - Owls');

-- ─── Users ─────────────────────────────────────────────────────────
-- Password for all users is "scout2026"
-- bcrypt hash of "scout2026"

INSERT INTO users (id, username, password_hash, display_name, is_admin) VALUES
  ('usr-leader', 'leader', '$2a$10$YJ8wTQ5K9UxHvZpF7LqQd.kxBvR6q5HqJz0/NMxMYdVGk2J1eKXHe', 'Weekly Leader', TRUE),
  ('usr-alpha', 'alpha', '$2a$10$YJ8wTQ5K9UxHvZpF7LqQd.kxBvR6q5HqJz0/NMxMYdVGk2J1eKXHe', 'Subcamp Alpha', FALSE),
  ('usr-bravo', 'bravo', '$2a$10$YJ8wTQ5K9UxHvZpF7LqQd.kxBvR6q5HqJz0/NMxMYdVGk2J1eKXHe', 'Subcamp Bravo', FALSE);

-- ─── User-Patrol Assignments ───────────────────────────────────────

-- Weekly leader sees both patrols
INSERT INTO user_patrols (user_id, patrol_id, sort_order) VALUES
  ('usr-leader', 'pat-eagles', 1),
  ('usr-leader', 'pat-foxes', 2);

-- Alpha subcamp leader
INSERT INTO user_patrols (user_id, patrol_id, sort_order) VALUES
  ('usr-alpha', 'pat-a1', 1),
  ('usr-alpha', 'pat-a2', 2),
  ('usr-alpha', 'pat-a3', 3),
  ('usr-alpha', 'pat-a4', 4),
  ('usr-alpha', 'pat-a5', 5),
  ('usr-alpha', 'pat-a6', 6);

-- Bravo subcamp leader
INSERT INTO user_patrols (user_id, patrol_id, sort_order) VALUES
  ('usr-bravo', 'pat-b1', 1),
  ('usr-bravo', 'pat-b2', 2),
  ('usr-bravo', 'pat-b3', 3),
  ('usr-bravo', 'pat-b4', 4),
  ('usr-bravo', 'pat-b5', 5),
  ('usr-bravo', 'pat-b6', 6);

-- ─── Sessions ──────────────────────────────────────────────────────

-- Weekly sessions (one active now, one upcoming)
INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at) VALUES
  ('ses-week-current', 'evt-weekly-2026', 'tpl-weekly', 'Week 6 Meeting',
   DATE_SUB(NOW(), INTERVAL 1 HOUR), DATE_ADD(NOW(), INTERVAL 2 HOUR)),
  ('ses-week-next', 'evt-weekly-2026', 'tpl-weekly', 'Week 7 Meeting',
   DATE_ADD(NOW(), INTERVAL 7 DAY), DATE_ADD(NOW(), INTERVAL 7 DAY) + INTERVAL 3 HOUR);

-- Camp sessions (one active, one upcoming, one closed)
INSERT INTO sessions (id, event_id, template_id, name, starts_at, ends_at) VALUES
  ('ses-camp-day1', 'evt-camp-2026', 'tpl-camp', 'Day 1 Inspection',
   DATE_SUB(NOW(), INTERVAL 1 DAY), DATE_SUB(NOW(), INTERVAL 20 HOUR)),
  ('ses-camp-day2', 'evt-camp-2026', 'tpl-camp', 'Day 2 Inspection',
   DATE_SUB(NOW(), INTERVAL 30 MINUTE), DATE_ADD(NOW(), INTERVAL 2 HOUR)),
  ('ses-camp-day3', 'evt-camp-2026', 'tpl-camp', 'Day 3 Inspection',
   DATE_ADD(NOW(), INTERVAL 1 DAY), DATE_ADD(NOW(), INTERVAL 1 DAY) + INTERVAL 2 HOUR);
