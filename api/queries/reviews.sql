-- name: ListOrganizerApplications :many
SELECT
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version AS form_version,
    applications.status,
    applications.submitted_at,
    applicants.id AS applicant_id,
    COALESCE(applications.applicant_email_snapshot, applicants.primary_email) AS applicant_email,
    applicants.display_name AS applicant_display_name,
    COALESCE(
        jsonb_object_agg(answers.question_key, answers.value_json)
            FILTER (WHERE answers.question_key IS NOT NULL),
        '{}'::jsonb
    )::text AS answers_json,
    applications.created_at,
    applications.updated_at
FROM ats.applications AS applications
JOIN ats.application_forms AS forms ON forms.id = applications.form_id
JOIN ats.users AS applicants ON applicants.id = applications.applicant_user_id
LEFT JOIN ats.application_answers AS answers ON answers.application_id = applications.id
WHERE applications.status IN ('submitted', 'accepted', 'waitlisted', 'rejected')
  AND (sqlc.arg(status)::text = '' OR applications.status = sqlc.arg(status)::text)
  AND (
      sqlc.arg(search)::text = ''
      OR applications.id::text ILIKE '%' || sqlc.arg(search)::text || '%'
      OR applicants.primary_email ILIKE '%' || sqlc.arg(search)::text || '%'
      OR COALESCE(applicants.display_name, '') ILIKE '%' || sqlc.arg(search)::text || '%'
      OR EXISTS (
          SELECT 1
          FROM ats.application_answers AS searched_answers
          WHERE searched_answers.application_id = applications.id
            AND searched_answers.value_json::text ILIKE '%' || sqlc.arg(search)::text || '%'
      )
  )
GROUP BY
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version,
    applications.status,
    applications.submitted_at,
    applicants.id,
    applicants.primary_email,
    applicants.display_name,
    applications.created_at,
    applications.updated_at
ORDER BY applications.submitted_at DESC NULLS LAST, applications.created_at DESC;

-- name: GetOrganizerApplication :one
SELECT
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version AS form_version,
    applications.status,
    applications.submitted_at,
    applicants.id AS applicant_id,
    COALESCE(applications.applicant_email_snapshot, applicants.primary_email) AS applicant_email,
    applicants.display_name AS applicant_display_name,
    COALESCE(
        jsonb_object_agg(answers.question_key, answers.value_json)
            FILTER (WHERE answers.question_key IS NOT NULL),
        '{}'::jsonb
    )::text AS answers_json,
    applications.created_at,
    applications.updated_at
FROM ats.applications AS applications
JOIN ats.application_forms AS forms ON forms.id = applications.form_id
JOIN ats.users AS applicants ON applicants.id = applications.applicant_user_id
LEFT JOIN ats.application_answers AS answers ON answers.application_id = applications.id
WHERE applications.id = $1
  AND applications.status IN ('submitted', 'accepted', 'waitlisted', 'rejected')
GROUP BY
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version,
    applications.status,
    applications.submitted_at,
    applicants.id,
    applicants.primary_email,
    applicants.display_name,
    applications.created_at,
    applications.updated_at;

-- name: ListReviewerApplications :many
SELECT
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version AS form_version,
    applications.status,
    applications.submitted_at,
    applicants.id AS applicant_id,
    COALESCE(applications.applicant_email_snapshot, applicants.primary_email) AS applicant_email,
    applicants.display_name AS applicant_display_name,
    COALESCE(
        jsonb_object_agg(answers.question_key, answers.value_json)
            FILTER (WHERE answers.question_key IS NOT NULL),
        '{}'::jsonb
    )::text AS answers_json,
    assignments.assigned_by,
    assignments.assigned_at,
    reviews.id AS review_id,
    reviews.status AS review_status,
    COALESCE(reviews.score_json, '{}'::jsonb)::text AS review_score_json,
    reviews.recommendation AS review_recommendation,
    reviews.internal_notes AS review_internal_notes,
    reviews.lock_version AS review_lock_version,
    reviews.submitted_at AS review_submitted_at,
    reviews.created_at AS review_created_at,
    reviews.updated_at AS review_updated_at,
    applications.created_at,
    applications.updated_at
FROM ats.applications AS applications
JOIN ats.application_forms AS forms ON forms.id = applications.form_id
JOIN ats.users AS applicants ON applicants.id = applications.applicant_user_id
LEFT JOIN ats.application_answers AS answers ON answers.application_id = applications.id
LEFT JOIN ats.review_assignments AS assignments
    ON assignments.application_id = applications.id
   AND assignments.reviewer_user_id = $1
LEFT JOIN ats.reviews AS reviews
    ON reviews.application_id = applications.id
   AND reviews.reviewer_user_id = $1
WHERE applications.status = 'submitted'
GROUP BY
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version,
    applications.status,
    applications.submitted_at,
    applicants.id,
    applicants.primary_email,
    applicants.display_name,
    assignments.assigned_by,
    assignments.assigned_at,
    reviews.id,
    reviews.status,
    reviews.score_json,
    reviews.recommendation,
    reviews.internal_notes,
    reviews.lock_version,
    reviews.submitted_at,
    reviews.created_at,
    reviews.updated_at,
    applications.created_at,
    applications.updated_at
ORDER BY
    CASE WHEN assignments.assigned_at IS NULL THEN 1 ELSE 0 END,
    assignments.assigned_at DESC NULLS LAST,
    applications.submitted_at DESC,
    applications.created_at DESC;

-- name: GetReviewerApplication :one
SELECT
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version AS form_version,
    applications.status,
    applications.submitted_at,
    applicants.id AS applicant_id,
    applicants.primary_email AS applicant_email,
    applicants.display_name AS applicant_display_name,
    COALESCE(
        jsonb_object_agg(answers.question_key, answers.value_json)
            FILTER (WHERE answers.question_key IS NOT NULL),
        '{}'::jsonb
    )::text AS answers_json,
    assignments.assigned_by,
    assignments.assigned_at,
    reviews.id AS review_id,
    reviews.status AS review_status,
    COALESCE(reviews.score_json, '{}'::jsonb)::text AS review_score_json,
    reviews.recommendation AS review_recommendation,
    reviews.internal_notes AS review_internal_notes,
    reviews.lock_version AS review_lock_version,
    reviews.submitted_at AS review_submitted_at,
    reviews.created_at AS review_created_at,
    reviews.updated_at AS review_updated_at,
    applications.created_at,
    applications.updated_at
FROM ats.applications AS applications
JOIN ats.application_forms AS forms ON forms.id = applications.form_id
JOIN ats.users AS applicants ON applicants.id = applications.applicant_user_id
LEFT JOIN ats.application_answers AS answers ON answers.application_id = applications.id
LEFT JOIN ats.review_assignments AS assignments
    ON assignments.application_id = applications.id
   AND assignments.reviewer_user_id = $2
LEFT JOIN ats.reviews AS reviews
    ON reviews.application_id = applications.id
   AND reviews.reviewer_user_id = $2
WHERE applications.id = $1
  AND applications.status = 'submitted'
GROUP BY
    applications.id,
    applications.cycle_id,
    applications.form_id,
    forms.version,
    applications.status,
    applications.submitted_at,
    applicants.id,
    applicants.primary_email,
    applicants.display_name,
    assignments.assigned_by,
    assignments.assigned_at,
    reviews.id,
    reviews.status,
    reviews.score_json,
    reviews.recommendation,
    reviews.internal_notes,
    reviews.lock_version,
    reviews.submitted_at,
    reviews.created_at,
    reviews.updated_at,
    applications.created_at,
    applications.updated_at;

-- name: UserExists :one
SELECT EXISTS (
    SELECT 1
    FROM ats.users
    WHERE id = $1
) AS user_exists;

-- name: UserHasReviewerRole :one
SELECT EXISTS (
    SELECT 1
    FROM ats.user_roles
    WHERE user_id = $1
      AND role = 'reviewer'
) AS has_reviewer_role;

-- name: GrantReviewerRole :one
WITH inserted AS (
    INSERT INTO ats.user_roles (user_id, role, created_by)
    SELECT users.id, 'reviewer', sqlc.arg('created_by')
    FROM ats.users
    WHERE users.id = sqlc.arg('user_id')
    ON CONFLICT (user_id, role) DO NOTHING
    RETURNING user_id
)
SELECT EXISTS (SELECT 1 FROM inserted) AS assigned;

-- name: InsertReviewerRoleAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (sqlc.arg('actor_user_id'), 'reviewer_role_assigned', 'user', sqlc.arg('subject_id'), jsonb_build_object('role', 'reviewer'));

-- name: AssignReviewer :one
WITH inserted AS (
    INSERT INTO ats.review_assignments (application_id, reviewer_user_id, assigned_by)
    SELECT applications.id, sqlc.arg('reviewer_user_id'), sqlc.arg('assigned_by')
    FROM ats.applications AS applications
    WHERE applications.id = sqlc.arg('application_id')
      AND applications.status = 'submitted'
      AND EXISTS (
          SELECT 1
          FROM ats.user_roles
          WHERE user_id = sqlc.arg('reviewer_user_id')
            AND role = 'reviewer'
      )
    ON CONFLICT (application_id, reviewer_user_id) DO NOTHING
    RETURNING application_id
)
SELECT EXISTS (SELECT 1 FROM inserted) AS assigned;

-- name: InsertReviewAssignmentAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (sqlc.arg('actor_user_id'), 'reviewer_assigned', 'application', sqlc.arg('subject_id'), jsonb_build_object('reviewerUserId', sqlc.arg('reviewer_user_id')::text));

-- name: UpdateReviewDraft :one
INSERT INTO ats.reviews (
    application_id,
    reviewer_user_id,
    score_json,
    recommendation,
    internal_notes,
    lock_version
)
SELECT
    sqlc.arg('application_id'),
    sqlc.arg('reviewer_user_id'),
    sqlc.arg('score_json')::jsonb,
    sqlc.arg('recommendation')::text,
    NULLIF(sqlc.arg('internal_notes')::text, ''),
    1
FROM ats.applications AS applications
WHERE applications.id = sqlc.arg('application_id')
  AND applications.status = 'submitted'
  AND sqlc.arg('lock_version')::integer = 0
ON CONFLICT (application_id, reviewer_user_id) DO UPDATE
SET score_json = EXCLUDED.score_json,
    recommendation = EXCLUDED.recommendation,
    internal_notes = EXCLUDED.internal_notes,
    lock_version = ats.reviews.lock_version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE ats.reviews.status = 'draft'
  AND ats.reviews.lock_version = sqlc.arg('lock_version')::integer
RETURNING id;

-- name: SubmitReview :one
UPDATE ats.reviews AS reviews
SET status = 'submitted',
    submitted_at = CURRENT_TIMESTAMP,
    lock_version = reviews.lock_version + 1,
    updated_at = CURRENT_TIMESTAMP
FROM ats.applications AS applications
WHERE reviews.application_id = applications.id
  AND reviews.application_id = sqlc.arg('application_id')
  AND reviews.reviewer_user_id = sqlc.arg('reviewer_user_id')
  AND reviews.status = 'draft'
  AND reviews.lock_version = sqlc.arg('lock_version')::integer
  AND applications.status = 'submitted'
RETURNING reviews.id;

-- name: InsertReviewSubmissionAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (sqlc.arg('actor_user_id'), 'review_submitted', 'review', sqlc.arg('subject_id'), jsonb_build_object('applicationId', sqlc.arg('application_id')::text));
