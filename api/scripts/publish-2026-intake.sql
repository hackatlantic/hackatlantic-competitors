BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM ats.application_cycles
        WHERE active
          AND slug <> 'hackatlantic-2026'
    ) THEN
        RAISE EXCEPTION 'another application cycle is already active';
    END IF;
END;
$$;

INSERT INTO ats.users (
    id,
    clerk_user_id,
    primary_email,
    display_name
) VALUES (
    '70000000-0000-4000-8000-000000000001',
    'system_hackatlantic_2026',
    'system@hackatlantic.ca',
    'HackAtlantic System'
)
ON CONFLICT (clerk_user_id) DO NOTHING;

INSERT INTO ats.application_cycles (
    id,
    slug,
    name,
    applications_open_at,
    applications_close_at,
    active
) VALUES (
    '70000000-0000-4000-8000-000000000002',
    'hackatlantic-2026',
    'HackAtlantic 2026',
    CURRENT_TIMESTAMP - INTERVAL '1 minute',
    TIMESTAMPTZ '2026-09-30 23:59:59 America/Halifax',
    true
)
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name,
    applications_close_at = EXCLUDED.applications_close_at,
    active = true,
    updated_at = CURRENT_TIMESTAMP;

INSERT INTO ats.application_forms (
    id,
    cycle_id,
    version,
    schema_json,
    published_at,
    created_by
)
SELECT
    '70000000-0000-4000-8000-000000000003',
    cycles.id,
    1,
    '{
      "resumeRequired": true,
      "questions": [
        {
          "key": "fullName",
          "label": "What is your full name?",
          "type": "string",
          "required": true,
          "help": "Use the name you want organizers to use."
        },
        {
          "key": "school",
          "label": "What school do you attend?",
          "type": "string",
          "required": true,
          "help": "Enter the full name of your current school, college, or university."
        },
        {
          "key": "yearsCoding",
          "label": "How many years have you been coding?",
          "type": "number",
          "required": true
        },
        {
          "key": "needsTravelSupport",
          "label": "Do you need travel support?",
          "type": "boolean",
          "required": false
        }
      ]
    }'::jsonb,
    CURRENT_TIMESTAMP,
    users.id
FROM ats.application_cycles AS cycles
CROSS JOIN ats.users AS users
WHERE cycles.slug = 'hackatlantic-2026'
  AND users.clerk_user_id = 'system_hackatlantic_2026'
ON CONFLICT (cycle_id, version) DO NOTHING;

COMMIT;
