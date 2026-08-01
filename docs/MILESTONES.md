# Milestones

Implement and review one implementation milestone at a time. A milestone is
complete only when its acceptance criteria and tests pass and its
contract/document changes are included.

The first product phase is the HackAtlantic application intake and review
system. Passes and event operations begin only after that lifecycle works.

Implementation status: Milestones 0–7 are complete and covered by the Go,
PostgreSQL integration, frontend lint, and production-build suites. Milestone 8
is the next planned vertical slice.

## Milestone 0 — Monorepo foundation

Scope:

- Preserve and identify the Next.js frontend.
- Add the Go API package layout.
- Add a compilable server with liveness/readiness endpoints.
- Add frontend API-client infrastructure.
- Add local PostgreSQL Compose configuration.
- Add initial OpenAPI and implementation documentation.

Acceptance:

- `npm run lint`
- `npm run build`
- `npm run api:test`
- Go formatting and vet pass.
- OpenAPI and Compose parse as YAML.

## Product phase 1 — Application intake and review

### Milestone 1 — Identity and ATS database foundation

Scope:

- Add migrations for users, user roles, cycles, form versions, applications,
  answers, review assignments, reviews, decisions, audit events, and email
  outbox.
- Configure `pgx`, connection pooling, query timeouts, and graceful close.
- Configure `sqlc` with reproducible generation.
- Make `/readyz` verify PostgreSQL.
- Verify Clerk JWTs with cached JWKS.
- Resolve/create local users and enforce applicant, reviewer, organizer, and
  scanner roles.

Acceptance:

- A clean database migrates from zero.
- Existing Clerk users are reconciled idempotently.
- New users receive only the applicant role.
- Privileged roles cannot be self-assigned.
- Missing, expired, malformed, and wrong-origin tokens are rejected.
- Readiness fails while PostgreSQL is unavailable.
- Schema constraints match `DOMAIN_MODEL.md`.
- Integration tests run against local PostgreSQL.

### Milestone 2 — Applicant form, drafts, and submission

Scope:

- Seed or publish the initial versioned application form.
- Build applicant account onboarding and dashboard.
- Build the application form from the published schema.
- Save typed answers as drafts with optimistic concurrency.
- Validate and submit a complete application.
- Enqueue and deliver submission-confirmation email.

Acceptance:

- An applicant can create only one application per active cycle.
- Drafts survive logout and another session.
- A stale browser tab cannot overwrite a newer draft.
- Server validation rejects unknown, mistyped, or incomplete answers.
- Submission locks applicant editing and records `submitted_at`.
- Submission retry is idempotent.
- Confirmation email is queued transactionally.
- Applicants cannot read another applicant's data.

### Milestone 3 — Organizer and reviewer workflow

Scope:

- Build organizer application search, filters, and detail view.
- Add reviewer role management and assignments.
- Build reviewer queue and application view.
- Save and submit structured reviews.
- Keep reviewer notes and recommendations internal.

Acceptance:

- Reviewers see only permitted or assigned applications.
- Scanners and ordinary applicants cannot access review endpoints.
- Review draft and submission behavior is tested.
- Organizer filters operate on server-side authorized queries.
- Applicant API models never include review fields.
- Role and assignment changes create audit events.

### Milestone 4 — Decisions, release, and attendee conversion

Scope:

- Add attendees and attendee-role migrations linked to applications and users.
- Record accepted, waitlisted, and rejected decisions.
- Separate internal decision recording from applicant-visible release.
- Display released decisions in the applicant dashboard.
- Enqueue decision emails transactionally.
- Convert an accepted application into exactly one attendee.
- Seed initial attendee roles from reviewed policy.

Acceptance:

- Unreleased decisions are invisible to applicants.
- Decision history is append-only and audited.
- Releasing a decision is retry-safe.
- Acceptance creates exactly one attendee even under retries.
- Waitlist and rejection do not create attendees.
- Decision recording, attendee creation, decision release, email outbox, and
  audit events each commit in their documented transaction boundaries.
- Reviewer notes never enter decision emails.

## Product phase 2 — Passes and event operations

### Milestone 5 — Pass issuance and web pass

Scope:

- Add remaining event migrations for activities, checkpoints, attendee
  entitlements, passes, and redemptions.
- Generate separate claim and QR secrets and store only hashes.
- Let accepted applicants view their web QR pass.
- Revoke and reissue passes.
- Send pass links through the email outbox.

Acceptance:

- Only accepted attendees can receive passes.
- Only one active pass exists per attendee.
- Reissue invalidates the old pass.
- Claim and QR credentials cannot substitute for each other.
- The QR resolves independently of a Clerk session.
- Secret values are absent from logs and database records.
- Public credential endpoints are rate limited.

### Milestone 6 — Scanner and atomic redemption

Scope:

- Add scan lookup and redemption endpoints.
- Build mobile scanner UI and manual lookup fallback.
- Enforce checkpoint windows, effective entitlements, and maximum redemptions.
- Record immutable audit details and idempotent outcomes.

Acceptance:

- Concurrent tests prove a maximum cannot be exceeded.
- An idempotent retry returns the original result.
- Scanner responses distinguish invalid, revoked, denied, exhausted, outside
  window, and redeemed outcomes.
- Scanner endpoints cannot access application/review data.
- The scanner works on current iOS and Android browsers over HTTPS.

### Milestone 7 — Event administration and operations

Scope:

- Build checkpoint, entitlement, pass, and redemption administration.
- Add CSV export and operational counts.
- Add safe pass revocation/reissue workflows.

Acceptance:

- Sensitive actions require confirmation and authorization.
- Exports do not expose credential hashes or application answers.
- Audit data answers who redeemed what, where, and when.

### Milestone 8 — Wallet providers

Scope:

- Generate signed Apple Wallet passes.
- Generate Google Wallet save links/objects.
- Keep provider-specific code behind adapters.

Acceptance:

- Both pass types use the authoritative opaque QR credential.
- Certificates and provider credentials come only from secret storage.
- Real-device tests pass.
- Provider failures do not prevent use of the web QR pass.

## Product phase 3 — Production hardening

### Milestone 9 — Reliability and event readiness

Scope:

- Load and concurrency testing.
- Monitoring, alerting, email-delivery visibility, backups, and restore drill.
- Application deadline, decision release, connectivity, and manual fallback
  runbooks.
- Production privacy and security review.

Acceptance:

- Target application-submit and scanner throughput are recorded.
- Email failures can be retried without repeating state transitions.
- Backup restoration is demonstrated.
- Venue connectivity fallback is rehearsed.
- On-call ownership and event-day procedures are documented.
