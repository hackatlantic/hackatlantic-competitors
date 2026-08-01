# Product lifecycle

This repository implements the complete HackAtlantic application tracking
system and its downstream event operations.

## Primary lifecycle

```text
Clerk account
  -> application draft
  -> submitted application snapshot
  -> admin review workflow
  -> accepted | waitlisted | rejected
  -> accepted application becomes attendee
  -> attendee pass issued
  -> event and meal redemptions
```

There is no external application tracking system and no attendee import
boundary. PostgreSQL is authoritative from the first saved draft through the
last event redemption.

## Actors

### Applicant

- Creates and authenticates with a Clerk account.
- Creates one application for the active application cycle.
- Saves draft answers over multiple sessions.
- Submits before the configured deadline.
- Sees submission state and released decisions.
- Uses the same account to view an attendee pass after acceptance.

### Admin

- Authenticates with Clerk.
- Receives admin only when Clerk's verified primary email is in PostgreSQL's
  admin email allowlist.
- Configures the application cycle and published form definition.
- Searches, filters, and reviews submitted applications.
- Records structured scores, recommendations, and internal notes.
- Reviews applications and records/releases decisions.
- Manages attendees, passes, checkpoints, entitlements, and staff access.
- Can use the QR scanner and redemption workflow.

### Scanner

- Authenticates with Clerk.
- Resolves and redeems attendee passes at authorized checkpoints.
- Cannot access application answers or reviewer notes.

## Application state

Initial states:

```text
draft
submitted
withdrawn
accepted
waitlisted
rejected
```

Rules:

- Only `draft` can be edited by the applicant.
- Submission validates the complete form and records `submitted_at`.
- Submitted answers are stable for review. Reopening requires an explicit
  organizer action and audit event.
- Decisions are internal until explicitly released.
- Applicants see only released decisions.
- Acceptance creates exactly one attendee for the application.
- Waitlisting and rejection do not create attendees.
- A released decision is changed only by a new audited decision action, never
  by silently overwriting history.

The implementation may introduce an internal `under_review` workflow state,
but it must not be required merely to assign or review an application.

## Form lifecycle

The initial product does not need a drag-and-drop form builder. A reviewed form
schema can be seeded or managed through organizer tooling.

Every application references an immutable published form version. Draft and
submission validation use that version so later form edits cannot reinterpret
historic answers.

Question keys remain stable within a form lineage. Answers are stored by
application and question key using typed JSON values.

## Review lifecycle

```text
submitted application
  -> draft review
  -> submitted review
  -> admin decision
  -> decision release
```

Reviewer notes and scores are internal. They must never be included in
applicant responses, emails, exports intended for applicants, or scanner APIs.

## Acceptance transaction

Recording an acceptance must atomically:

1. create a decision-history record;
2. update the application's current decision state;
3. create or resolve exactly one attendee linked to the application and user;
4. write an audit event.

Releasing that decision is a separate atomic action that:

1. marks the current decision and application as released;
2. enqueues the appropriate decision email in the transactional outbox;
3. writes an audit event.

Pass issuance can occur in the same workflow or a later operation, but retries
must not create duplicate attendees or passes.

## Email lifecycle

Email is a delivery channel, not the source of truth. PostgreSQL records:

- which template/event should be sent;
- recipient and non-secret template data;
- attempt count and delivery state;
- provider message identifier;
- timestamps and safe error summaries.

Submission confirmation and released decision actions enqueue email within the
same database transaction as the state change. A worker sends queued messages
and records provider results. Provider outages must not roll back an already
committed application or decision.

## Pass independence

An accepted applicant can view their pass while authenticated. Email may also
contain a revocable pass link. The QR credential itself resolves a pass without
a Clerk session so Apple Wallet, Google Wallet, and the scanner continue to
work independently after issuance.
