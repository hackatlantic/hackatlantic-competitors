-- name: UpsertClerkUser :one
INSERT INTO ats.users (clerk_user_id, primary_email, display_name)
VALUES ($1, $2, $3)
ON CONFLICT (clerk_user_id) DO UPDATE
SET primary_email = EXCLUDED.primary_email,
    display_name = EXCLUDED.display_name,
    updated_at = now()
RETURNING id, clerk_user_id, primary_email, display_name, created_at, updated_at;

-- name: EnsureApplicantRole :exec
INSERT INTO ats.user_roles (user_id, role)
VALUES ($1, 'applicant')
ON CONFLICT (user_id, role) DO NOTHING;

-- name: ListUserRoles :many
SELECT role
FROM ats.user_roles
WHERE user_id = $1
ORDER BY role;

-- name: UserEmailIsAdmin :one
SELECT EXISTS (
    SELECT 1
    FROM ats.admin_email_allowlist
    WHERE normalized_email = lower(btrim($1))
) AS is_admin;

-- name: AssignUserRole :exec
INSERT INTO ats.user_roles (user_id, role, created_by)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, role) DO NOTHING;

-- name: IdentityUserExists :one
SELECT EXISTS (
    SELECT 1 FROM ats.users WHERE id = $1
) AS user_exists;

-- name: LookupScannerUserByEmail :many
SELECT u.id, u.clerk_user_id,
    EXISTS (SELECT 1 FROM ats.user_roles r WHERE r.user_id = u.id AND r.role = 'scanner') AS scanner_access,
    EXISTS (SELECT 1 FROM ats.admin_email_allowlist a WHERE a.normalized_email = lower(btrim(u.primary_email))) AS is_admin
FROM ats.users u
WHERE lower(btrim(u.primary_email)) = sqlc.arg('email')::text
ORDER BY u.id
LIMIT 2;

-- name: GrantScannerRole :one
WITH inserted AS (
    INSERT INTO ats.user_roles (user_id, role, created_by)
    VALUES (sqlc.arg('user_id'), 'scanner', sqlc.arg('created_by'))
    ON CONFLICT (user_id, role) DO NOTHING
    RETURNING user_id
)
SELECT EXISTS (SELECT 1 FROM inserted) AS changed;

-- name: RevokeScannerRole :one
WITH removed AS (
    DELETE FROM ats.user_roles
    WHERE user_id = sqlc.arg('user_id')
      AND role = 'scanner'
    RETURNING user_id
)
SELECT EXISTS (SELECT 1 FROM removed) AS changed;

-- name: InsertScannerRoleAudit :exec
INSERT INTO ats.audit_events (actor_user_id, event_type, subject_type, subject_id, metadata_json)
VALUES (
    sqlc.arg('actor_user_id'),
    sqlc.arg('event_type'),
    'user',
    sqlc.arg('subject_id'),
    jsonb_build_object('role', 'scanner')
);

-- name: GetUserIDByClerkUserID :one
SELECT id
FROM ats.users
WHERE clerk_user_id = $1;

-- name: CountOrganizerRoles :one
SELECT count(*)
FROM ats.user_roles
WHERE role = 'organizer';

-- name: AcquireOrganizerBootstrapLock :exec
SELECT pg_advisory_xact_lock(740159346019);

-- name: UserHasOrganizerRole :one
SELECT EXISTS(
    SELECT 1
    FROM ats.user_roles
    WHERE user_id = $1
      AND role = 'organizer'
);

-- name: InsertBootstrapOrganizerAuditEvent :exec
INSERT INTO ats.audit_events (event_type, subject_type, subject_id, metadata_json)
VALUES ('organizer_bootstrapped', 'user', $1, '{}'::jsonb);
