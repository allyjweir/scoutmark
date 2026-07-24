INSERT INTO criteria_templates (id, name, description)
VALUES (
    'tpl-round2-scoring',
    'Round 2 Scoring',
    'A single overall score for Camp Chief Round 2 finalists.'
)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description;

INSERT INTO criteria (id, template_id, title, description, min_value, max_value, sort_order)
VALUES (
    'crit-round2-overall',
    'tpl-round2-scoring',
    'Overall Score',
    'Score the patrol overall for the Camp Chief''s Pendant.',
    1,
    10,
    0
)
ON CONFLICT (id) DO UPDATE
SET template_id = EXCLUDED.template_id,
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    min_value = EXCLUDED.min_value,
    max_value = EXCLUDED.max_value,
    sort_order = EXCLUDED.sort_order;
