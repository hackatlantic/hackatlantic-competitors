# Agent handoff

This repository will be implemented milestone by milestone by an external
coding agent and reviewed before the next milestone starts.

## Before changing code

1. Read every document linked from `docs/README.md`.
2. Read the milestone being assigned and restate its boundaries.
3. Inspect the worktree and preserve unrelated user changes.
4. Inspect the current OpenAPI contract and all migrations.
5. Identify uncertainties that would change the schema or security model.

Do not start a later milestone to make the current one appear complete.

## Implementation rules

- The Go API is the only component that accesses PostgreSQL.
- Do not add Supabase Auth, Supabase Edge Functions, or a browser database SDK.
- Do not add an external ATS import or integration boundary; this repository is
  the ATS and PostgreSQL is authoritative from draft through redemption.
- Keep HTTP details in `internal/httpapi`.
- Keep domain logic independently testable.
- Enforce applicant ownership and admin/scanner permissions in Go. Admin is
  derived exclusively from PostgreSQL's admin email allowlist matched to
  Clerk's verified primary email.
- Treat published form versions and submitted applications as immutable unless
  an explicit audited workflow says otherwise.
- Keep review content and unreleased decisions out of applicant models.
- Use a transactional outbox for submission confirmations, decisions, and pass
  emails; do not call the email provider inside a state-changing transaction.
- Use explicit Go transactions for redemption behavior.
- Enforce important rules with database constraints as well as application
  validation.
- Use parameterized SQL and context deadlines.
- Update OpenAPI with implemented endpoint changes.
- Add migrations; do not edit an already-applied migration after review.
- Never log or commit credentials, raw QR values, claim tokens, or PII.
- Do not make Apple or Google Wallet availability a prerequisite for the web
  pass.

## Application lifecycle rule

The core lifecycle is:

```text
draft -> submitted -> accepted | waitlisted | rejected
```

Submission, decision release, and accepted-attendee conversion are server-side
state transitions. Acceptance creates one attendee linked to the application
and user. Retries must not create duplicates. There is no accepted-attendee
import.

## Attendee entitlement rule

`attendee_entitlements` is an override, not a staff role and not a redemption
record. Attendee roles such as `mentor` may drive entitlement provisioning but
must not be confused with staff authorization roles. Resolve access as:

```text
explicit attendee entitlement
    else checkpoint defaults
```

An explicit `allowed = false` wins over a checkpoint default allow. A mentor
dinner should default to denied; mentor attendees receive an explicit allow
with the intended maximum. The API, never the frontend, calculates effective
access.

## Required milestone report

At the end of each milestone provide:

```text
Outcome
- What now works.

Files and contracts
- Important implementation files.
- Migrations added.
- OpenAPI operations added or changed.

Verification
- Exact commands run and results.
- Integration/manual checks.

Security and data review
- Authorization boundaries touched.
- Applicant/reviewer data exposure reviewed.
- Secrets or PII handling.
- Concurrency or transaction guarantees.

Remaining issues
- Known limitations.
- Deferred work, mapped to a later milestone.
- Decisions that require maintainer review.
```

Do not claim a check passed if it was not run. State environmental blockers
precisely.

## Reviewer checklist

The reviewer should verify:

- scope matches the assigned milestone;
- frontend code cannot bypass Go;
- authentication and authorization are distinct;
- schema constraints match documented rules;
- error and success outcomes match OpenAPI;
- secrets and PII are absent from logs and fixtures;
- concurrency behavior is tested where state can be consumed;
- operational failure modes are explicit;
- documentation changed when architecture changed.
