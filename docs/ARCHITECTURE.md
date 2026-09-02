# Architecture

## System context

```text
Applicant ────────┐
Reviewer ─────────┤
Organizer ────────┼─ HTTPS ─> Next.js UI ─ HTTPS ─> Go API ─> PostgreSQL
Scanner ──────────┘                    Clerk JWT ────┘

Go API ─ transactional outbox ─> Email provider
Go API ─ signed artifacts ─> Apple Wallet / Google Wallet
```

## Responsibilities

### Next.js

- Render the application form, applicant dashboard, review dashboard,
  organizer administration, attendee pass, and scanner experiences.
- Use Clerk's frontend integration for applicant and staff sign-in.
- Obtain a Clerk session token and send it as a bearer token.
- Call the Go API through `lib/api.ts`.
- Never connect to PostgreSQL or a managed-database SDK.
- Never implement authoritative application state, decisions, redemptions, or
  authorization logic.

### Go API

- Verify Clerk session tokens and authorize every actor.
- Own form versions, application drafts, submission, review, and decisions.
- Convert accepted applications into attendees.
- Issue, revoke, and resolve claim and QR credentials.
- Resolve checkpoint eligibility and redemption limits.
- Commit redemptions atomically.
- Enqueue transactional emails and deliver them through a provider adapter.
- Produce audit events and operational reports.
- Generate Apple and Google Wallet artifacts.
- Own every application database query.

### PostgreSQL

- Store the entire product lifecycle and enforce relational invariants.
- Apply forward-only, reviewed migrations from `api/migrations`.
- Prevent invalid duplicates even if application code is incorrect.
- Remain portable across managed PostgreSQL providers.

The current production provider is the existing Supabase project, used only as
managed PostgreSQL. The browser does not use Supabase Auth, database APIs,
Realtime, Storage, Edge Functions, or the Data API. Production must disable
the Supabase Data API and keep all ATS objects in the unexposed `ats` schema.

### Clerk

- Authenticate applicants, admins, and scanners.
- Maintain sessions and verified authentication identities.
- Not store application answers, authorization roles, reviews, decisions,
  attendee eligibility, redemption state, or pass data.

### Email provider

- Deliver submission confirmations, released decisions, and pass links.
- Receive only the recipient and template data required for delivery.
- Not determine whether an application was submitted or a decision exists.
- Be called asynchronously from a PostgreSQL-backed outbox.

## Deployment units

- The root Next.js application deploys independently.
- `api/Dockerfile` produces the Go API container.
- Production PostgreSQL is managed separately.
- `docker-compose.yml` is for local infrastructure and integration work.

Suggested public origins:

```text
https://pass.hackatlantic.ca
https://api.hackatlantic.ca
```

Production CORS must allow only known frontend origins. HTTPS is required for
camera access and every credential-bearing request.

## Package boundaries

Go packages under `api/internal` own domain-specific behavior:

- `auth`: authentication middleware and identity context.
- `users`: Clerk-linked users and application authorization roles.
- `applications`: forms, drafts, submissions, and application state.
- `reviews`: reviewer assignments and evaluations.
- `decisions`: decisions, release, and accepted-applicant conversion.
- `notifications`: transactional email outbox and provider adapters.
- `attendees`: accepted-applicant lifecycle and event participation.
- `activities`: optional schedule metadata that is not inherently redeemable.
- `checkpoints`: scannable/redeemable event operations.
- `entitlements`: attendee access and limit resolution.
- `passes`: claim links and QR credentials.
- `redemptions`: transactional checkpoint redemption.
- `wallet`: Apple and Google Wallet adapters.
- `database`: connections and generated query adapters.
- `httpapi`: HTTP routing, middleware, serialization, and errors.

Domain packages must not depend on HTTP request or response types. The
`httpapi` package adapts transport models to domain operations.

## Dependency direction

```text
cmd/server
  -> httpapi
      -> domain services
          -> database interfaces
              -> PostgreSQL adapters
```

Wallet and email provider code are adapters behind interfaces. Core
application, decision, and redemption behavior must be testable without
network access.

## Observability

Each request includes or records only the operational data needed to debug and measure the service:

- a generated or accepted request ID in the response and structured application logs;
- low-cardinality route template, status class, and duration metrics;
- deployment and build information.

Authenticated user IDs, raw request URLs, query strings, resumes, answers, JWTs, and QR/claim credentials are deliberately excluded from telemetry labels and metric attributes.

Health endpoints:

- `/healthz` means the process is alive.
- `/readyz` means required dependencies are available.

Once PostgreSQL is introduced, readiness must fail when the database cannot be
reached.
