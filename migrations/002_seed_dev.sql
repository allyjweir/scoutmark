-- Scoutmark development seed data
-- Blair Atholl 2026
-- ─── Event ─────────────────────────────────────────────────────────
INSERT INTO
  events (id, name, description)
VALUES
  (
    'evt-ba-2026',
    'Blair Atholl 2026',
    'Blair Atholl Jamborette 2026'
  );

-- ─── Criteria Template ─────────────────────────────────────────────
INSERT INTO
  criteria_templates (id, name, description)
VALUES
  (
    'tpl-camp',
    'Camp Inspection',
    'Daily camp inspection criteria'
  );

INSERT INTO
  criteria (
    id,
    template_id,
    title,
    description,
    min_value,
    max_value,
    sort_order
  )
VALUES
  (
    'crt-tent',
    'tpl-camp',
    'Tent & Bedding',
    'Tents properly pitched, bedding aired and rolled',
    0,
    10,
    1
  ),
  (
    'crt-kit',
    'tpl-camp',
    'Kit Layout',
    'Personal kit neatly stored and accessible',
    0,
    10,
    2
  ),
  (
    'crt-hygiene',
    'tpl-camp',
    'Hygiene Area',
    'Wash stands clean, toiletries organised',
    0,
    10,
    3
  ),
  (
    'crt-kitchen',
    'tpl-camp',
    'Kitchen Area',
    'Cooking area clean, fire properly maintained',
    0,
    10,
    4
  ),
  (
    'crt-gadgets',
    'tpl-camp',
    'Camp Gadgets',
    'Quality and creativity of pioneering projects',
    0,
    10,
    5
  ),
  (
    'crt-spirit',
    'tpl-camp',
    'Camp Spirit',
    'Enthusiasm, teamwork and patrol morale',
    0,
    10,
    6
  );

-- ─── Users ─────────────────────────────────────────────────────────
-- Password for all users: "blairatholl"
-- bcrypt hash: $2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO
INSERT INTO
  users (
    id,
    username,
    password_hash,
    display_name,
    is_admin
  )
VALUES
  (
    'usr-campchief',
    'campchief',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'Camp Chief',
    TRUE
  ),
  (
    'usr-morrison',
    'morrison',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'Morrison',
    FALSE
  ),
  (
    'usr-macdonald',
    'macdonald',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'MacDonald',
    FALSE
  ),
  (
    'usr-maclean',
    'maclean',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'MacLean',
    FALSE
  ),
  (
    'usr-murray',
    'murray',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'Murray',
    FALSE
  ),
  (
    'usr-robertson',
    'robertson',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'Robertson',
    FALSE
  ),
  (
    'usr-stewart',
    'stewart',
    '$2a$10$Z9qHSj8pYtZjW2THpkrUTu9uHkKmPBH3S4QAgFstDayG3c/L0UTdO',
    'Stewart',
    FALSE
  );

-- ─── Patrols ───────────────────────────────────────────────────────
-- 5 patrols per user, named after European countries
-- Morrison's patrols
INSERT INTO
  patrols (id, name)
VALUES
  ('pat-mor-1', 'France'),
  ('pat-mor-2', 'Germany'),
  ('pat-mor-3', 'Italy'),
  ('pat-mor-4', 'Spain'),
  ('pat-mor-5', 'Portugal');

-- MacDonald's patrols
INSERT INTO
  patrols (id, name)
VALUES
  ('pat-mac-1', 'Sweden'),
  ('pat-mac-2', 'Norway'),
  ('pat-mac-3', 'Denmark'),
  ('pat-mac-4', 'Finland'),
  ('pat-mac-5', 'Iceland');

-- MacLean's patrols
INSERT INTO
  patrols (id, name)
VALUES
  ('pat-mcl-1', 'Netherlands'),
  ('pat-mcl-2', 'Belgium'),
  ('pat-mcl-3', 'Switzerland'),
  ('pat-mcl-4', 'Austria'),
  ('pat-mcl-5', 'Luxembourg');

-- Murray's patrols
INSERT INTO
  patrols (id, name)
VALUES
  ('pat-mur-1', 'Poland'),
  ('pat-mur-2', 'Czech Republic'),
  ('pat-mur-3', 'Hungary'),
  ('pat-mur-4', 'Romania'),
  ('pat-mur-5', 'Croatia');

-- Robertson's patrols
INSERT INTO
  patrols (id, name)
VALUES
  ('pat-rob-1', 'Greece'),
  ('pat-rob-2', 'Ireland'),
  ('pat-rob-3', 'Estonia'),
  ('pat-rob-4', 'Latvia'),
  ('pat-rob-5', 'Lithuania');

-- Stewart's patrols
INSERT INTO
  patrols (id, name)
VALUES
  ('pat-stw-1', 'Slovenia'),
  ('pat-stw-2', 'Slovakia'),
  ('pat-stw-3', 'Bulgaria'),
  ('pat-stw-4', 'Serbia'),
  ('pat-stw-5', 'Montenegro');

-- ─── User-Patrol Assignments ──────────────────────────────────────
INSERT INTO
  user_patrols (user_id, patrol_id, sort_order)
VALUES
  ('usr-morrison', 'pat-mor-1', 1),
  ('usr-morrison', 'pat-mor-2', 2),
  ('usr-morrison', 'pat-mor-3', 3),
  ('usr-morrison', 'pat-mor-4', 4),
  ('usr-morrison', 'pat-mor-5', 5);

INSERT INTO
  user_patrols (user_id, patrol_id, sort_order)
VALUES
  ('usr-macdonald', 'pat-mac-1', 1),
  ('usr-macdonald', 'pat-mac-2', 2),
  ('usr-macdonald', 'pat-mac-3', 3),
  ('usr-macdonald', 'pat-mac-4', 4),
  ('usr-macdonald', 'pat-mac-5', 5);

INSERT INTO
  user_patrols (user_id, patrol_id, sort_order)
VALUES
  ('usr-maclean', 'pat-mcl-1', 1),
  ('usr-maclean', 'pat-mcl-2', 2),
  ('usr-maclean', 'pat-mcl-3', 3),
  ('usr-maclean', 'pat-mcl-4', 4),
  ('usr-maclean', 'pat-mcl-5', 5);

INSERT INTO
  user_patrols (user_id, patrol_id, sort_order)
VALUES
  ('usr-murray', 'pat-mur-1', 1),
  ('usr-murray', 'pat-mur-2', 2),
  ('usr-murray', 'pat-mur-3', 3),
  ('usr-murray', 'pat-mur-4', 4),
  ('usr-murray', 'pat-mur-5', 5);

INSERT INTO
  user_patrols (user_id, patrol_id, sort_order)
VALUES
  ('usr-robertson', 'pat-rob-1', 1),
  ('usr-robertson', 'pat-rob-2', 2),
  ('usr-robertson', 'pat-rob-3', 3),
  ('usr-robertson', 'pat-rob-4', 4),
  ('usr-robertson', 'pat-rob-5', 5);

INSERT INTO
  user_patrols (user_id, patrol_id, sort_order)
VALUES
  ('usr-stewart', 'pat-stw-1', 1),
  ('usr-stewart', 'pat-stw-2', 2),
  ('usr-stewart', 'pat-stw-3', 3),
  ('usr-stewart', 'pat-stw-4', 4),
  ('usr-stewart', 'pat-stw-5', 5);

-- ─── Sessions ──────────────────────────────────────────────────────
-- Blair Atholl 2026: Sunday 8 Feb – Thursday 12 Feb
-- Sunday (today): open from now for 3 hours
-- Mon–Thu: 06:00 – 10:00
INSERT INTO
  sessions (
    id,
    event_id,
    template_id,
    name,
    starts_at,
    ends_at
  )
VALUES
  (
    'ses-sun',
    'evt-ba-2026',
    'tpl-camp',
    'Sunday',
    NOW(),
    NOW() + INTERVAL '3 hours'
  ),
  (
    'ses-mon',
    'evt-ba-2026',
    'tpl-camp',
    'Monday',
    '2026-02-09 06:00:00',
    '2026-02-09 10:00:00'
  ),
  (
    'ses-tue',
    'evt-ba-2026',
    'tpl-camp',
    'Tuesday',
    '2026-02-10 06:00:00',
    '2026-02-10 10:00:00'
  ),
  (
    'ses-wed',
    'evt-ba-2026',
    'tpl-camp',
    'Wednesday',
    '2026-02-11 06:00:00',
    '2026-02-11 10:00:00'
  ),
  (
    'ses-thu',
    'evt-ba-2026',
    'tpl-camp',
    'Thursday',
    '2026-02-12 06:00:00',
    '2026-02-12 10:00:00'
  );