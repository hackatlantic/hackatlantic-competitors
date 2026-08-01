-- name: LockAcceptedAttendeeForIssue :one
SELECT
    attendee.id,
    attendee.user_id,
    attendee.display_name,
    attendee.email
FROM ats.attendees AS attendee
JOIN ats.applications AS application ON application.id = attendee.application_id
WHERE attendee.id = sqlc.arg(attendee_id)
  AND application.status = 'accepted'
  AND application.decision_released_at IS NOT NULL
FOR UPDATE OF attendee, application;

-- name: GetPassAttendee :one
SELECT pass.attendee_id
FROM ats.passes AS pass
WHERE pass.id = sqlc.arg(pass_id);

-- name: GetActivePassForAttendeeForUpdate :one
SELECT
    pass.id,
    pass.attendee_id,
    pass.qr_token_hash,
    pass.claim_token_hash,
    pass.status,
    pass.issued_at,
    pass.revoked_at,
    pass.replaced_by_pass_id
FROM ats.passes AS pass
WHERE pass.attendee_id = sqlc.arg(attendee_id)
  AND pass.status = 'active'
FOR UPDATE OF pass;

-- name: LockActivePassForMutation :one
SELECT
    pass.id,
    pass.attendee_id,
    pass.qr_token_hash,
    pass.claim_token_hash,
    pass.status,
    pass.issued_at,
    pass.revoked_at,
    pass.replaced_by_pass_id,
    attendee.user_id,
    attendee.display_name,
    attendee.email
FROM ats.passes AS pass
JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
JOIN ats.applications AS application ON application.id = attendee.application_id
WHERE pass.id = sqlc.arg(pass_id)
  AND pass.status = 'active'
  AND application.status = 'accepted'
  AND application.decision_released_at IS NOT NULL
FOR UPDATE OF pass, attendee, application;

-- name: InsertPass :one
INSERT INTO ats.passes AS pass (
    attendee_id,
    qr_token_hash,
    claim_token_hash
)
VALUES (
    sqlc.arg(attendee_id),
    sqlc.arg(qr_token_hash),
    sqlc.arg(claim_token_hash)
)
RETURNING
    id,
    attendee_id,
    qr_token_hash,
    claim_token_hash,
    status,
    issued_at,
    revoked_at,
    replaced_by_pass_id;

-- name: ReplaceActivePass :exec
UPDATE ats.passes AS pass
SET status = 'replaced',
    revoked_at = CURRENT_TIMESTAMP,
    replaced_by_pass_id = sqlc.arg(replacement_pass_id)
WHERE pass.id = sqlc.arg(pass_id)
  AND pass.status = 'active';

-- name: RevokeActivePass :exec
UPDATE ats.passes AS pass
SET status = 'revoked',
    revoked_at = CURRENT_TIMESTAMP
WHERE pass.id = sqlc.arg(pass_id)
  AND pass.status = 'active';

-- name: GetWebPassForAttendee :one
SELECT
    pass.id,
    pass.attendee_id,
    attendee.display_name,
    pass.status,
    pass.issued_at,
    pass.revoked_at
FROM ats.passes AS pass
JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
JOIN ats.applications AS application ON application.id = attendee.application_id
WHERE attendee.user_id = sqlc.arg(user_id)
  AND pass.status = 'active'
  AND application.status = 'accepted'
  AND application.decision_released_at IS NOT NULL;

-- name: ResolveActivePassByClaimTokenHash :one
SELECT
    pass.id,
    pass.attendee_id,
    attendee.display_name,
    pass.status,
    pass.issued_at
FROM ats.passes AS pass
JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
JOIN ats.applications AS application ON application.id = attendee.application_id
WHERE pass.claim_token_hash = sqlc.arg(claim_token_hash)
  AND pass.status = 'active'
  AND application.status = 'accepted'
  AND application.decision_released_at IS NOT NULL;

-- name: GetOrganizerPassSummary :one
SELECT
    attendee.id AS attendee_id,
    pass.id AS pass_id,
    attendee.display_name,
    pass.status,
    pass.issued_at,
    pass.revoked_at
FROM ats.attendees AS attendee
LEFT JOIN ats.passes AS pass ON pass.attendee_id = attendee.id AND pass.status = 'active'
WHERE attendee.application_id = sqlc.arg(application_id);

-- name: InsertPassIssuedAudit :exec
INSERT INTO ats.audit_events AS audit_event (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (
    sqlc.arg(actor_user_id),
    'pass_issued',
    'pass',
    sqlc.arg(pass_id),
    jsonb_build_object('attendeeId', sqlc.arg(attendee_id)::text)
);

-- name: InsertPassReissuedAudit :exec
INSERT INTO ats.audit_events AS audit_event (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (
    sqlc.arg(actor_user_id),
    'pass_reissued',
    'pass',
    sqlc.arg(pass_id),
    jsonb_build_object(
        'attendeeId',
        sqlc.arg(attendee_id)::text,
        'replacementPassId',
        sqlc.arg(replacement_pass_id)::text
    )
);

-- name: InsertPassRevokedAudit :exec
INSERT INTO ats.audit_events AS audit_event (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (
    sqlc.arg(actor_user_id),
    'pass_revoked',
    'pass',
    sqlc.arg(pass_id),
    jsonb_build_object('attendeeId', sqlc.arg(attendee_id)::text)
);

-- name: InsertPassLinkEmail :exec
INSERT INTO ats.email_outbox AS outbox (
    event_type,
    recipient_user_id,
    recipient_email,
    template_key,
    template_data_json,
    dedupe_key
)
VALUES (
    'pass_link',
    sqlc.arg(recipient_user_id),
    sqlc.arg(recipient_email),
    'pass_link',
    sqlc.arg(template_data_json),
    sqlc.arg(dedupe_key)
)
ON CONFLICT (dedupe_key) DO NOTHING;
