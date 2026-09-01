-- Publish the applicant-approved HackAtlantic form without mutating any
-- previously published schema. Existing submitted applications remain pinned
-- to their original form; active drafts move forward and retain compatible
-- profile answers.

INSERT INTO ats.application_forms (
    id,
    cycle_id,
    version,
    schema_json,
    published_at,
    created_by
)
SELECT
    '70000000-0000-4000-8000-000000000004',
    cycles.id,
    2,
    '{
  "resumeRequired": false,
  "resumeAfterQuestionKey": "school",
  "questions": [
    {
      "key": "fullName",
      "label": "Name",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "email",
      "label": "Email",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "email",
      "help": "Verified through your signed-in account."
    },
    {
      "key": "school",
      "label": "School",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "dietaryRestrictions",
      "label": "Dietary restrictions",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text",
      "help": "Enter None if you do not have any."
    },
    {
      "key": "hackAtlanticExcitement",
      "label": "What are you most excited about at Hack Atlantic?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "maxWords": 100,
      "help": "Maximum 100 words."
    },
    {
      "key": "priorHackathonExperience",
      "label": "Prior hackathon experience",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "select",
      "options": ["This is my first", "1–3", "3+"]
    },
    {
      "key": "desiredTeammateNames",
      "label": "Desired teammate names (Optional)",
      "type": "string",
      "required": false,
      "section": "Hackathon Specific Questions",
      "control": "text"
    },
    {
      "key": "hardwareProject",
      "label": "Are you looking to make a hardware project?",
      "type": "boolean",
      "required": true,
      "section": "Hackathon Specific Questions"
    },
    {
      "key": "hardwareEquipment",
      "label": "What equipment are you looking to use?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "showWhen": {
        "key": "hardwareProject",
        "equals": true
      }
    }
  ]
}'::jsonb,
    CURRENT_TIMESTAMP,
    users.id
FROM ats.application_cycles AS cycles
JOIN ats.users AS users ON users.clerk_user_id = 'system_hackatlantic_2026'
WHERE cycles.slug = 'hackatlantic-2026'
ON CONFLICT (cycle_id, version) DO NOTHING;

INSERT INTO ats.application_forms (
    id,
    cycle_id,
    version,
    schema_json,
    published_at,
    created_by
)
SELECT
    '60000000-0000-4000-8000-000000000007',
    cycles.id,
    3,
    '{
  "resumeRequired": false,
  "resumeAfterQuestionKey": "school",
  "questions": [
    {
      "key": "fullName",
      "label": "Name",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "email",
      "label": "Email",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "email",
      "help": "Verified through your signed-in account."
    },
    {
      "key": "school",
      "label": "School",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text"
    },
    {
      "key": "dietaryRestrictions",
      "label": "Dietary restrictions",
      "type": "string",
      "required": true,
      "section": "Build your profile",
      "control": "text",
      "help": "Enter None if you do not have any."
    },
    {
      "key": "hackAtlanticExcitement",
      "label": "What are you most excited about at Hack Atlantic?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "maxWords": 100,
      "help": "Maximum 100 words."
    },
    {
      "key": "priorHackathonExperience",
      "label": "Prior hackathon experience",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "select",
      "options": ["This is my first", "1–3", "3+"]
    },
    {
      "key": "desiredTeammateNames",
      "label": "Desired teammate names (Optional)",
      "type": "string",
      "required": false,
      "section": "Hackathon Specific Questions",
      "control": "text"
    },
    {
      "key": "hardwareProject",
      "label": "Are you looking to make a hardware project?",
      "type": "boolean",
      "required": true,
      "section": "Hackathon Specific Questions"
    },
    {
      "key": "hardwareEquipment",
      "label": "What equipment are you looking to use?",
      "type": "string",
      "required": true,
      "section": "Hackathon Specific Questions",
      "control": "textarea",
      "showWhen": {
        "key": "hardwareProject",
        "equals": true
      }
    }
  ]
}'::jsonb,
    CURRENT_TIMESTAMP,
    forms.created_by
FROM ats.application_cycles AS cycles
JOIN LATERAL (
    SELECT created_by
    FROM ats.application_forms
    WHERE cycle_id = cycles.id
    ORDER BY version DESC
    LIMIT 1
) AS forms ON true
WHERE cycles.slug = 'dev-intake'
ON CONFLICT (cycle_id, version) DO NOTHING;

DELETE FROM ats.application_answers AS answers
USING ats.applications AS applications, ats.application_cycles AS cycles
WHERE answers.application_id = applications.id
  AND applications.cycle_id = cycles.id
  AND applications.status = 'draft'
  AND cycles.slug IN ('hackatlantic-2026', 'dev-intake')
  AND answers.question_key NOT IN (
      'fullName',
      'email',
      'school',
      'dietaryRestrictions',
      'hackAtlanticExcitement',
      'priorHackathonExperience',
      'desiredTeammateNames',
      'hardwareProject',
      'hardwareEquipment'
  );

UPDATE ats.applications AS applications
SET form_id = forms.id,
    lock_version = applications.lock_version + 1,
    updated_at = CURRENT_TIMESTAMP
FROM ats.application_cycles AS cycles
JOIN ats.application_forms AS forms
  ON forms.cycle_id = cycles.id
 AND (
      (cycles.slug = 'hackatlantic-2026' AND forms.version = 2)
      OR (cycles.slug = 'dev-intake' AND forms.version = 3)
 )
WHERE applications.cycle_id = cycles.id
  AND applications.status = 'draft'
  AND cycles.slug IN ('hackatlantic-2026', 'dev-intake');
