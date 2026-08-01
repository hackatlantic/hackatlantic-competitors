# API conventions

## Contract

`openapi/openapi.yaml` is the implemented HTTP contract. Add an endpoint to the
contract when its handler is added, and test the response against the contract.

All application endpoints are versioned under `/v1`. System endpoints remain
unversioned.

## Authentication

Authenticated endpoints require:

```text
Authorization: Bearer <Clerk session JWT>
```

Go verifies the token before resolving the local `users` record and its
`user_roles`. Applicant endpoints enforce ownership in addition to the
`applicant` role. The public authorization model is `applicant`, `admin`, and
`scanner`. Admin includes all ATS, review, operations, pass, and scanner
capabilities. The Go API derives admin only from PostgreSQL's
`admin_email_allowlist` and Clerk's verified primary email. Database changes
take effect on the next authenticated request and require no API restart.

Public pass-link endpoints may use a revocable claim token. A staff or
applicant identity is never accepted as a substitute for a required pass/claim
credential, and a claim credential does not grant application access.

## Resource outline

This is the planned resource outline. The checked-in OpenAPI contract identifies
the endpoints implemented in the current milestone.

```text
GET    /v1/me

GET    /v1/application-cycles/active
GET    /v1/application-forms/current
POST   /v1/applications
GET    /v1/applications/mine
PUT    /v1/applications/{applicationId}/draft
POST   /v1/applications/{applicationId}/submit
POST   /v1/applications/{applicationId}/withdraw
GET    /v1/applications/{applicationId}/decision

GET    /v1/reviewer/assignments
GET    /v1/reviewer/applications/{applicationId}
PUT    /v1/reviewer/applications/{applicationId}/review
POST   /v1/reviewer/applications/{applicationId}/review/submit

GET    /v1/admin/applications
GET    /v1/admin/applications/{applicationId}
POST   /v1/admin/applications/{applicationId}/assignments
POST   /v1/admin/applications/{applicationId}/decisions
POST   /v1/admin/decisions/{decisionId}/release
PUT    /v1/admin/users/{userId}/roles/scanner
DELETE /v1/admin/users/{userId}/roles/scanner
POST   /v1/admin/applications/{applicationId}/reopen

GET    /v1/admin/attendees
GET    /v1/admin/attendees/{attendeeId}
PATCH  /v1/admin/attendees/{attendeeId}

GET    /v1/checkpoints
POST   /v1/admin/checkpoints
PATCH  /v1/admin/checkpoints/{checkpointId}

PUT    /v1/admin/attendees/{attendeeId}/entitlements/{checkpointId}
DELETE /v1/admin/attendees/{attendeeId}/entitlements/{checkpointId}

POST   /v1/admin/attendees/{attendeeId}/passes
POST   /v1/admin/passes/{passId}/revoke
POST   /v1/admin/passes/{passId}/reissue

GET    /v1/attendee/pass
GET    /v1/claim/{claimToken}
GET    /v1/claim/{claimToken}/apple-wallet
POST   /v1/claim/{claimToken}/google-wallet

POST   /v1/scans/lookup
POST   /v1/redemptions
GET    /v1/admin/redemptions
```

## JSON

- Use camelCase property names.
- Use UUID strings for application identifiers.
- Use RFC 3339 UTC timestamps.
- Use strings, not numeric codes, for stable domain outcomes.
- Do not return raw database or provider errors.
- Never include reviewer notes or unreleased decisions in applicant models.
- Return form-version and draft `lockVersion` values where concurrency matters.

## Errors

Errors use one envelope:

```json
{
  "code": "not_entitled",
  "message": "This attendee is not entitled to Mentor Dinner.",
  "requestId": "01J...",
  "details": {}
}
```

`details` is optional and must not contain secrets.

Suggested status behavior:

- `400`: malformed request.
- `401`: missing or invalid authentication.
- `403`: authenticated but unauthorized.
- `404`: resource not found or a public secret does not resolve.
- `409`: conflicting resource state.
- `422`: semantically invalid input.
- `429`: rate limited.
- `500`: unexpected server failure.
- `503`: required dependency unavailable.

Expected scan outcomes such as `already_exhausted` should normally be a
successful response with an outcome field, because the request was valid and
the scanner needs structured attendee/checkpoint context.

## Idempotency

Every mutation that may be retried must accept an idempotency key. Redemption
requires a client-generated UUID in the body or `Idempotency-Key` header. The
same key with the same operation returns the original result; reusing it with a
different operation is a conflict.

Draft saving uses optimistic concurrency. The client sends the last observed
`lockVersion`; a stale version receives `409` with enough metadata to prompt a
reload instead of overwriting newer answers.

## Pagination

List endpoints use opaque cursor pagination:

```json
{
  "items": [],
  "nextCursor": null
}
```

Do not expose database offsets as durable cursors.
