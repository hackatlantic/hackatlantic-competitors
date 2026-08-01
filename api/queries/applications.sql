-- name: GetCurrentApplicationForm :one
SELECT
    forms.id AS form_id,
    forms.version AS form_version,
    forms.schema_json,
    cycles.id AS cycle_id
FROM ats.application_cycles AS cycles
JOIN ats.application_forms AS forms
    ON forms.cycle_id = cycles.id
   AND forms.published_at IS NOT NULL
WHERE cycles.active
  AND cycles.applications_open_at <= CURRENT_TIMESTAMP
  AND CURRENT_TIMESTAMP < cycles.applications_close_at
ORDER BY forms.version DESC
LIMIT 1;

-- name: CreateApplication :one
INSERT INTO ats.applications (cycle_id, form_id, applicant_user_id)
SELECT cycles.id, forms.id, $1
FROM ats.application_cycles AS cycles
JOIN ats.application_forms AS forms
    ON forms.cycle_id = cycles.id
   AND forms.published_at IS NOT NULL
WHERE forms.id = sqlc.arg(form_id)
  AND cycles.active
  AND cycles.applications_open_at <= CURRENT_TIMESTAMP
  AND CURRENT_TIMESTAMP < cycles.applications_close_at
ON CONFLICT (cycle_id, applicant_user_id) DO UPDATE
SET updated_at = ats.applications.updated_at
RETURNING id;

-- name: GetApplicationForApplicant :one
SELECT
    applications.id,
    applications.cycle_id,
    applications.form_id,
    applications.applicant_user_id,
    (
        CASE
            WHEN applications.status IN ('accepted', 'waitlisted', 'rejected') THEN 'submitted'
            ELSE applications.status
        END
    )::text AS status,
    applications.submitted_at,
    applications.withdrawn_at,
    applications.current_decision,
    applications.decision_released_at,
    applications.lock_version,
    applications.created_at,
    applications.updated_at,
    forms.version AS form_version,
    forms.schema_json,
    COALESCE(
        jsonb_object_agg(answers.question_key, answers.value_json)
            FILTER (WHERE answers.question_key IS NOT NULL),
        '{}'::jsonb
    )::text AS answers_json
FROM ats.applications AS applications
JOIN ats.application_forms AS forms ON forms.id = applications.form_id
LEFT JOIN ats.application_answers AS answers ON answers.application_id = applications.id
WHERE applications.id = $1
  AND applications.applicant_user_id = $2
GROUP BY
    applications.id,
    applications.cycle_id,
    applications.form_id,
    applications.applicant_user_id,
    applications.status,
    applications.submitted_at,
    applications.withdrawn_at,
    applications.current_decision,
    applications.decision_released_at,
    applications.lock_version,
    applications.created_at,
    applications.updated_at,
    forms.version,
    forms.schema_json;

-- name: ListApplicationsForApplicant :many
SELECT
    applications.id,
    applications.cycle_id,
    applications.form_id,
    (
        CASE
            WHEN applications.status IN ('accepted', 'waitlisted', 'rejected') THEN 'submitted'
            ELSE applications.status
        END
    )::text AS status,
    applications.submitted_at,
    applications.lock_version,
    applications.created_at,
    applications.updated_at,
    forms.version AS form_version,
    COALESCE(
        jsonb_object_agg(answers.question_key, answers.value_json)
            FILTER (WHERE answers.question_key IS NOT NULL),
        '{}'::jsonb
    )::text AS answers_json
FROM ats.applications AS applications
JOIN ats.application_forms AS forms ON forms.id = applications.form_id
LEFT JOIN ats.application_answers AS answers ON answers.application_id = applications.id
WHERE applications.applicant_user_id = $1
GROUP BY
    applications.id,
    applications.cycle_id,
    applications.form_id,
    applications.status,
    applications.submitted_at,
    applications.lock_version,
    applications.created_at,
    applications.updated_at,
    forms.version
ORDER BY applications.created_at DESC;

-- name: UpdateApplicationDraft :one
UPDATE ats.applications
SET lock_version = lock_version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
  AND applicant_user_id = $2
  AND status = 'draft'
  AND lock_version = $3
RETURNING id;

-- name: DeleteApplicationAnswers :exec
DELETE FROM ats.application_answers
WHERE application_id = $1;

-- name: ReplaceApplicationAnswers :exec
INSERT INTO ats.application_answers (application_id, question_key, value_json)
SELECT $1, entry.key, entry.value
FROM pg_catalog.jsonb_each(sqlc.arg(answers_json)::jsonb) AS entry
ON CONFLICT (application_id, question_key) DO UPDATE
SET value_json = EXCLUDED.value_json,
    updated_at = CURRENT_TIMESTAMP;

-- name: UpdateApplicationSubmission :one
UPDATE ats.applications AS applications
SET status = 'submitted',
    submitted_at = CURRENT_TIMESTAMP,
    applicant_email_snapshot = sqlc.arg('applicant_email_snapshot'),
    lock_version = applications.lock_version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE applications.id = $1
  AND applications.applicant_user_id = $2
  AND applications.status = 'draft'
  AND applications.lock_version = $3
  AND EXISTS (
      SELECT 1
      FROM ats.application_cycles AS cycles
      WHERE cycles.id = applications.cycle_id
        AND cycles.active
        AND cycles.applications_open_at <= CURRENT_TIMESTAMP
        AND CURRENT_TIMESTAMP < cycles.applications_close_at
  )
RETURNING id;

-- name: InsertSubmissionConfirmation :exec
INSERT INTO ats.email_outbox (
    event_type,
    recipient_user_id,
    recipient_email,
    template_key,
    template_data_json,
    dedupe_key
)
VALUES ('submission_confirmation', $1, $2, 'submission_confirmation', $3, $4)
ON CONFLICT (dedupe_key) DO NOTHING;

-- name: IsApplicationSubmissionWindowOpen :one
SELECT EXISTS (
    SELECT 1
    FROM ats.applications AS applications
    JOIN ats.application_cycles AS cycles ON cycles.id = applications.cycle_id
    WHERE applications.id = $1
      AND applications.applicant_user_id = $2
      AND cycles.active
      AND cycles.applications_open_at <= CURRENT_TIMESTAMP
      AND CURRENT_TIMESTAMP < cycles.applications_close_at
) AS is_open;

-- name: SeedIntakeFixtureUser :one
INSERT INTO ats.users (id, clerk_user_id, primary_email, display_name)
VALUES ($1, $2, $3, $4)
ON CONFLICT (clerk_user_id) DO UPDATE
SET primary_email = EXCLUDED.primary_email,
    display_name = EXCLUDED.display_name,
    updated_at = CURRENT_TIMESTAMP
RETURNING id;

-- name: OtherActiveApplicationCycleExists :one
SELECT EXISTS (
    SELECT 1
    FROM ats.application_cycles
    WHERE active
      AND id <> $1
);

-- name: EnsureIntakeFixtureCycle :exec
INSERT INTO ats.application_cycles (
    id,
    slug,
    name,
    applications_open_at,
    applications_close_at,
    active
)
VALUES (
    $1,
    $2,
    $3,
    CURRENT_TIMESTAMP - INTERVAL '1 day',
    CURRENT_TIMESTAMP + INTERVAL '365 days',
    true
)
ON CONFLICT (id) DO UPDATE
SET slug = EXCLUDED.slug,
    name = EXCLUDED.name,
    applications_open_at = EXCLUDED.applications_open_at,
    applications_close_at = EXCLUDED.applications_close_at,
    active = true,
    updated_at = CURRENT_TIMESTAMP;

-- name: EnsureIntakeFixtureForm :exec
INSERT INTO ats.application_forms (
    id,
    cycle_id,
    version,
    schema_json,
    published_at,
    created_by
)
VALUES ($1, $2, 2, $3, CURRENT_TIMESTAMP, $4)
ON CONFLICT (cycle_id, version) DO UPDATE
SET schema_json = EXCLUDED.schema_json,
    published_at = EXCLUDED.published_at
WHERE ats.application_forms.published_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM ats.applications
      WHERE applications.form_id = ats.application_forms.id
  );
