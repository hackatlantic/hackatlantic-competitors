# Domain model

This document defines the initial product language and relational model. UUID
primary keys and UTC timestamps are assumed unless stated otherwise.

## Identity and authorization

### users

An authenticated person known to the product.

```text
id                 uuid primary key
clerk_user_id      text unique not null
primary_email      text not null
display_name       text
created_at         timestamptz
updated_at         timestamptz
```

Clerk is authoritative for authentication. The local user ID is authoritative
for application relationships and audit records. Email changes must be
reconciled from a verified Clerk identity; clients cannot assert them.

### user_roles

Application-wide authorization assignments:

```text
user_id      uuid references users(id)
role         text
created_at   timestamptz
created_by   uuid nullable references users(id)

primary key (user_id, role)
```

Initial roles:

- `applicant`: draft and submit an application, view a released decision, and
  access an accepted-attendee pass;
- `admin`: complete ATS, private reviews, decisions, staff access, passes,
  event operations, and QR scanning;
- `scanner`: resolve and redeem passes at checkpoints.

New users receive `applicant`. The Go API derives `admin` from the
PostgreSQL-backed `admin_email_allowlist`, matched against Clerk's verified
primary email on every request. Scanner is an additive database role that an
admin can grant or revoke.

### admin_email_allowlist

Database-backed admin authorization that can change without a deployment or
API restart.

```text
id                 uuid primary key
normalized_email   text unique, lowercase
created_by         uuid nullable references users(id)
created_at         timestamptz
```

## Application intake

### application_cycles

An application period for one HackAtlantic event.

```text
id                    uuid primary key
slug                  text unique
name                  text
applications_open_at  timestamptz
applications_close_at timestamptz
active                boolean
created_at            timestamptz
updated_at            timestamptz
```

Only one active cycle is needed initially, but lifecycle data must be scoped to
a cycle so future events do not overwrite history.

### application_forms

An immutable published form version for a cycle.

```text
id             uuid primary key
cycle_id       uuid references application_cycles(id)
version        integer
schema_json    jsonb
published_at   timestamptz nullable
created_by     uuid references users(id)
created_at     timestamptz

unique (cycle_id, version)
```

`schema_json` describes sections, stable question keys, labels, types,
validation, and applicant-visible help. Once published and referenced by an
application, the schema is immutable. A changed form creates a new version.

### applications

```text
id                    uuid primary key
cycle_id              uuid references application_cycles(id)
form_id               uuid references application_forms(id)
applicant_user_id     uuid references users(id)
status                text
submitted_at          timestamptz nullable
withdrawn_at          timestamptz nullable
current_decision      text nullable
decision_released_at  timestamptz nullable
lock_version          integer
created_at            timestamptz
updated_at            timestamptz
applicant_email_snapshot text nullable

unique (cycle_id, applicant_user_id)
```

Initial status values are `draft`, `submitted`, `withdrawn`, `accepted`,
`waitlisted`, and `rejected`. `lock_version` supports optimistic concurrency
for draft saving so an older browser tab cannot silently overwrite a newer
draft.

Only draft applications can be edited by applicants. Submission validates
required answers against the referenced form version and transitions state in
one transaction.

The applicant's verified Clerk primary email is shown read-only and copied to
`applicant_email_snapshot` when the application is submitted. Applicants do
not retype an identity field that Clerk already verifies. `school` is a
required versioned form answer so its wording can evolve with future forms.

### application_resumes

Private PDF resume metadata. The PDF bytes live in private object storage and
are never exposed through a public bucket URL.

```text
application_id      uuid primary key references applications(id)
object_key          text unique
original_filename   text
media_type          text (application/pdf only)
byte_size           bigint (maximum 5 MiB)
sha256              bytea
uploaded_at         timestamptz
updated_at          timestamptz
```

An applicant can upload or replace a resume only while their application is a
draft. Submission is rejected when its form version declares
`resumeRequired: true` and no resume exists. Admins retrieve the PDF through an
authorized Go API endpoint; the ATS embeds that response in its detail and
review screens.

### application_answers

```text
application_id   uuid references applications(id)
question_key     text
value_json       jsonb
updated_at       timestamptz

primary key (application_id, question_key)
```

Values are JSON to support strings, numbers, Booleans, arrays, and structured
answers. Go validates the value against the question definition. Unknown
question keys and invalid types are rejected.

An optional immutable submission snapshot may be introduced if needed for
audit or performance, but the initial schema must guarantee submitted answers
cannot be edited outside an explicit audited reopen workflow.

## Review and decision

### review_assignments

```text
application_id      uuid references applications(id)
reviewer_user_id    uuid references users(id)
assigned_by         uuid references users(id)
assigned_at         timestamptz

primary key (application_id, reviewer_user_id)
```

Assignments determine a reviewer's accessible queue unless organizers
explicitly configure a broader review policy.

### reviews

```text
id                  uuid primary key
application_id      uuid references applications(id)
reviewer_user_id    uuid references users(id)
status              text
score_json          jsonb
recommendation      text nullable
internal_notes      text nullable
submitted_at        timestamptz nullable
created_at          timestamptz
updated_at          timestamptz

unique (application_id, reviewer_user_id)
```

Review status is `draft` or `submitted`. Review rubrics are validated in Go.
Review data is internal and never exposed through applicant or scanner APIs.

### decisions

Append-only decision history:

```text
id                  uuid primary key
application_id      uuid references applications(id)
outcome             text
internal_reason     text nullable
decided_by          uuid references users(id)
decided_at          timestamptz
released_by         uuid nullable references users(id)
released_at         timestamptz nullable
supersedes_id       uuid nullable references decisions(id)
created_at          timestamptz
```

Outcomes are `accepted`, `waitlisted`, and `rejected`. Applications cache their
current outcome and release timestamp for efficient authorization and display,
but decisions preserve the audit history. The service updates both inside one
transaction.

An applicant sees an outcome only after `released_at` is set. Releasing a
decision also enqueues its email.

### audit_events

```text
id             uuid primary key
actor_user_id  uuid nullable references users(id)
event_type     text
subject_type   text
subject_id     uuid
metadata_json  jsonb
request_id     text nullable
created_at     timestamptz
```

Audit metadata must not contain credentials or unnecessary application
answers. Important events include submission, reopening, review submission,
role changes, decision recording/release, attendee creation, pass changes, and
redemption corrections.

## Email delivery

### email_outbox

```text
id                   uuid primary key
event_type           text
recipient_user_id    uuid nullable references users(id)
recipient_email      text
template_key         text
template_data_json   jsonb
dedupe_key           text unique
status               text
attempt_count        integer
available_at         timestamptz
provider_message_id  text nullable
last_error_code      text nullable
sent_at              timestamptz nullable
created_at           timestamptz
updated_at           timestamptz
```

Initial statuses are `pending`, `processing`, `sent`, and `failed`. State
changes such as submission and decision release enqueue an outbox row in the
same transaction. Workers claim rows safely and make delivery idempotent.

## Accepted applicants and passes

### attendees

An accepted application becomes exactly one attendee.

```text
id               uuid primary key
cycle_id         uuid references application_cycles(id)
application_id   uuid unique references applications(id)
user_id          uuid references users(id)
display_name     text
email            text
created_at       timestamptz
updated_at       timestamptz

unique (cycle_id, user_id)
```

`display_name` and `email` are event-operation snapshots. Identity still links
to `users`. The acceptance transaction must be retry-safe.

### attendee_roles

Event classifications such as `hacker`, `mentor`, `judge`, or `sponsor`:

```text
attendee_id   uuid references attendees(id)
role          text
created_at    timestamptz

primary key (attendee_id, role)
```

Attendee roles may drive entitlement provisioning. They are not application
authorization roles and do not grant reviewer, organizer, or scanner access.

### passes

```text
id                    uuid primary key
attendee_id           uuid references attendees(id)
credential_hash       bytea unique
claim_token_hash      bytea unique nullable
status                text
issued_at             timestamptz
revoked_at            timestamptz nullable
replaced_by_pass_id   uuid nullable references passes(id)
```

Only one active pass per attendee is enforced by a partial unique index. Raw
QR and claim credentials are returned only when issued and are never stored.
An authenticated attendee can retrieve their pass; a revocable claim token may
support emailed pass links. The QR itself is independent of Clerk.

## Event operations

### activities

Optional schedule metadata such as a workshop or meal period. Not every
activity needs scanning. A checkpoint may reference an activity.

### checkpoints

A scannable or redeemable operation such as event entry, Saturday lunch,
mentor dinner, swag pickup, or a capacity-controlled workshop.

```text
id                       uuid primary key
cycle_id                 uuid references application_cycles(id)
activity_id              uuid nullable
slug                     text
name                     text
opens_at                 timestamptz nullable
closes_at                timestamptz nullable
default_allowed          boolean
default_max_redemptions  integer
active                   boolean
created_at               timestamptz
updated_at               timestamptz

unique (cycle_id, slug)
check (default_max_redemptions >= 0)
```

### attendee_entitlements

Per-attendee checkpoint overrides:

```text
attendee_id       uuid references attendees(id)
checkpoint_id     uuid references checkpoints(id)
allowed           boolean
max_redemptions   integer
created_at        timestamptz
updated_at        timestamptz

primary key (attendee_id, checkpoint_id)
check (max_redemptions >= 0)
```

Effective access:

1. Use an explicit attendee entitlement when one exists.
2. Otherwise use the checkpoint defaults.
3. If `allowed` is false, ignore the maximum and deny.
4. Allow another redemption only while committed usage is below the effective
   maximum.

For mentor dinner, the checkpoint defaults to denied and mentors receive an
explicit `allowed = true`, `max_redemptions = 1` entitlement. The API never
trusts a frontend-provided attendee role or eligibility result.

### redemptions

```text
id                    uuid primary key
pass_id               uuid references passes(id)
attendee_id           uuid references attendees(id)
checkpoint_id         uuid references checkpoints(id)
ordinal               integer
scanner_user_id       uuid references users(id)
idempotency_key       uuid unique
redeemed_at           timestamptz

unique (attendee_id, checkpoint_id, ordinal)
```

Redemptions are append-only. A future correction mechanism records an explicit
void/audit event rather than deleting history.

## Redemption transaction

One transaction must:

1. hash and resolve the QR credential;
2. lock the active pass;
3. load the active checkpoint and validate its window;
4. resolve the entitlement override or checkpoint defaults;
5. count existing valid redemptions;
6. insert the next ordinal using the idempotency key;
7. commit and return the authoritative outcome.

Expected outcomes include `redeemed`, `already_exhausted`, `not_entitled`,
`outside_window`, `invalid_pass`, and `revoked_pass`.
