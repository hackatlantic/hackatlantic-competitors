-- name: LockApplicationForDecision :one
SELECT
    applications.id,
    applications.cycle_id,
    applications.applicant_user_id,
    applications.status,
    applications.current_decision_id,
    applicants.primary_email AS applicant_email,
    COALESCE(NULLIF(applicants.display_name, ''), applicants.primary_email) AS applicant_display_name
FROM ats.applications AS applications
JOIN ats.users AS applicants ON applicants.id = applications.applicant_user_id
WHERE applications.id = $1
FOR UPDATE OF applications;

-- name: GetLatestDecisionForApplication :one
SELECT
    decision.id,
    decision.application_id,
    decision.outcome,
    decision.internal_reason,
    decision.decided_by,
    decision.decided_at,
    decision.released_by,
    decision.released_at,
    decision.supersedes_id,
    decision.created_at
FROM ats.decisions AS decision
WHERE decision.id = (
    SELECT applications.current_decision_id
    FROM ats.applications AS applications
    WHERE applications.id = $1
);

-- name: InsertDecision :one
INSERT INTO ats.decisions (
    application_id,
    outcome,
    internal_reason,
    decided_by,
    supersedes_id
)
VALUES ($1, $2, $3, $4, $5)
RETURNING
    id,
    application_id,
    outcome,
    internal_reason,
    decided_by,
    decided_at,
    released_by,
    released_at,
    supersedes_id,
    created_at;

-- name: UpdateApplicationDecisionCache :exec
UPDATE ats.applications
SET status = $2,
    current_decision = $2,
    current_decision_id = $3,
    decision_released_at = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: UpsertAcceptedAttendee :one
WITH inserted AS (
    INSERT INTO ats.attendees (
        cycle_id,
        application_id,
        user_id,
        display_name,
        email
    )
    VALUES ($1, $2, $3, $4, $5)
    ON CONFLICT (application_id) DO NOTHING
    RETURNING id
), attendee AS (
    SELECT id, true AS attendee_created FROM inserted
    UNION ALL
    SELECT attendees.id, false AS attendee_created
    FROM ats.attendees
    WHERE attendees.application_id = $2
      AND NOT EXISTS (SELECT 1 FROM inserted)
)
SELECT id, attendee_created
FROM attendee;

-- name: SeedAttendeeHackerRole :exec
INSERT INTO ats.attendee_roles (attendee_id, role)
VALUES ($1, 'hacker')
ON CONFLICT (attendee_id, role) DO NOTHING;

-- name: InsertDecisionRecordedAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (sqlc.arg('actor_user_id'), 'decision_recorded', 'decision', sqlc.arg('subject_id'), jsonb_build_object('applicationId', sqlc.arg('application_id')::text, 'outcome', sqlc.arg('outcome')::text));

-- name: InsertAttendeeCreatedAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (sqlc.arg('actor_user_id'), 'attendee_created', 'attendee', sqlc.arg('subject_id'), jsonb_build_object('applicationId', sqlc.arg('application_id')::text));

-- name: LockCurrentDecisionForRelease :one
SELECT
    decision.id,
    decision.application_id,
    decision.outcome,
    decision.internal_reason,
    decision.decided_by,
    decision.decided_at,
    decision.released_by,
    decision.released_at,
    decision.supersedes_id,
    decision.created_at,
    applications.applicant_user_id,
    applicants.primary_email AS applicant_email
FROM ats.decisions AS decision
JOIN ats.applications AS applications ON applications.id = decision.application_id
JOIN ats.users AS applicants ON applicants.id = applications.applicant_user_id
WHERE decision.id = $1
  AND applications.current_decision_id = decision.id
FOR UPDATE OF decision, applications;

-- name: ReleaseDecision :one
UPDATE ats.decisions
SET released_by = $2,
    released_at = CURRENT_TIMESTAMP
WHERE id = $1
  AND released_at IS NULL
RETURNING released_at;

-- name: UpdateApplicationDecisionRelease :exec
UPDATE ats.applications
SET decision_released_at = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: InsertDecisionReleaseEmail :exec
INSERT INTO ats.email_outbox (
    event_type,
    recipient_user_id,
    recipient_email,
    template_key,
    template_data_json,
    dedupe_key
)
VALUES ('decision_release', $1, $2, 'decision_release', $3, $4)
ON CONFLICT (dedupe_key) DO NOTHING;

-- name: InsertDecisionReleasedAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (sqlc.arg('actor_user_id'), 'decision_released', 'decision', sqlc.arg('subject_id'), jsonb_build_object('applicationId', sqlc.arg('application_id')::text, 'outcome', sqlc.arg('outcome')::text));

-- name: GetReleasedDecisionForApplicant :one
SELECT
    decisions.application_id,
    decisions.outcome,
    decisions.released_at
FROM ats.applications
JOIN ats.decisions ON decisions.application_id = applications.id
WHERE applications.id = $1
  AND applications.applicant_user_id = $2
  AND applications.decision_released_at IS NOT NULL
  AND decisions.released_at IS NOT NULL
  AND decisions.id = applications.current_decision_id;
