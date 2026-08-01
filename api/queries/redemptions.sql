-- name: ListActiveCheckpoints :many
SELECT checkpoint.id, checkpoint.name
FROM ats.checkpoints AS checkpoint
WHERE checkpoint.active
ORDER BY checkpoint.name, checkpoint.id;

-- name: ResolveActivePassByQRTokenHash :one
SELECT
    pass.id,
    pass.attendee_id,
    attendee.display_name,
    pass.status,
    pass.issued_at
FROM ats.passes AS pass
JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
JOIN ats.applications AS application ON application.id = attendee.application_id
WHERE pass.qr_token_hash = sqlc.arg(qr_token_hash)
  AND pass.status = 'active'
  AND application.status = 'accepted'
  AND application.decision_released_at IS NOT NULL;

-- name: FindRedemptionRequestForUpdate :one
SELECT
    request.id,
    request.idempotency_key,
    request.scanner_user_id,
    request.checkpoint_id,
    request.qr_token_hash,
    request.pass_id,
    request.attendee_id,
    request.outcome,
    request.attendee_display_name,
    request.pass_status,
    request.checkpoint_name,
    request.redemption_id,
    request.created_at
FROM ats.redemption_requests AS request
WHERE request.idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE OF request;

-- name: LockCheckpointForRedemption :one
SELECT
    checkpoint.id,
    checkpoint.cycle_id,
    checkpoint.name,
    checkpoint.opens_at,
    checkpoint.closes_at,
    checkpoint.default_allowed,
    checkpoint.default_max_redemptions,
    checkpoint.active
FROM ats.checkpoints AS checkpoint
WHERE checkpoint.id = sqlc.arg(checkpoint_id)
FOR UPDATE OF checkpoint;

-- name: LockPassForRedemptionByQRTokenHash :one
SELECT
    pass.id,
    pass.attendee_id,
    attendee.cycle_id,
    attendee.display_name,
    pass.status,
    application.status = 'accepted' AND application.decision_released_at IS NOT NULL AS eligible
FROM ats.passes AS pass
JOIN ats.attendees AS attendee ON attendee.id = pass.attendee_id
JOIN ats.applications AS application ON application.id = attendee.application_id
WHERE pass.qr_token_hash = sqlc.arg(qr_token_hash)
FOR UPDATE OF pass, attendee, application;

-- name: GetAttendeeEntitlementForRedemption :one
SELECT entitlement.allowed, entitlement.max_redemptions
FROM ats.attendee_entitlements AS entitlement
WHERE entitlement.attendee_id = sqlc.arg(attendee_id)
  AND entitlement.checkpoint_id = sqlc.arg(checkpoint_id)
  AND entitlement.cycle_id = sqlc.arg(cycle_id)
FOR UPDATE OF entitlement;

-- name: CountCommittedRedemptions :one
SELECT count(*)
FROM ats.redemptions AS redemption
WHERE redemption.attendee_id = sqlc.arg(attendee_id)
  AND redemption.checkpoint_id = sqlc.arg(checkpoint_id);

-- name: InsertRedemptionRequest :exec
INSERT INTO ats.redemption_requests (
    idempotency_key,
    scanner_user_id,
    checkpoint_id,
    qr_token_hash,
    pass_id,
    attendee_id,
    outcome,
    attendee_display_name,
    pass_status,
    checkpoint_name,
    redemption_id
)
VALUES (
    sqlc.arg(idempotency_key),
    sqlc.arg(scanner_user_id),
    sqlc.arg(checkpoint_id),
    sqlc.arg(qr_token_hash),
    sqlc.narg(pass_id),
    sqlc.narg(attendee_id),
    sqlc.arg(outcome),
    sqlc.narg(attendee_display_name),
    sqlc.narg(pass_status),
    sqlc.arg(checkpoint_name),
    sqlc.narg(redemption_id)
);

-- name: InsertRedemption :exec
INSERT INTO ats.redemptions (
    id,
    pass_id,
    attendee_id,
    checkpoint_id,
    cycle_id,
    ordinal,
    scanner_user_id,
    idempotency_key
)
VALUES (
    sqlc.arg(id),
    sqlc.arg(pass_id),
    sqlc.arg(attendee_id),
    sqlc.arg(checkpoint_id),
    sqlc.arg(cycle_id),
    sqlc.arg(ordinal),
    sqlc.arg(scanner_user_id),
    sqlc.arg(idempotency_key)
);

-- name: InsertRedemptionAudit :exec
INSERT INTO ats.audit_events AS audit_event (
    actor_user_id,
    event_type,
    subject_type,
    subject_id,
    metadata_json
)
VALUES (
    sqlc.arg(scanner_user_id),
    'redemption_recorded',
    'redemption',
    COALESCE(sqlc.narg(redemption_id), sqlc.arg(checkpoint_id)),
    jsonb_build_object(
        'scannerUserId', sqlc.arg(scanner_user_id)::text,
        'checkpointId', sqlc.arg(checkpoint_id)::text,
        'attendeeId', sqlc.narg(attendee_id)::text,
        'passId', sqlc.narg(pass_id)::text,
        'redemptionId', sqlc.narg(redemption_id)::text,
        'idempotencyKey', sqlc.arg(idempotency_key)::text,
        'outcome', sqlc.arg(outcome)
    )
);
