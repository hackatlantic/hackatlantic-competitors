# HackAtlantic ATS

HackAtlantic's application tracking and event operations system.

This repository is a small monorepo:

- `app/`, `components/`, and `lib/` contain the Next.js frontend.
- `api/` contains the Go API and database migrations.
- `openapi/` contains the HTTP contract.
- `docs/` contains the architecture and implementation handoff.

Clerk authenticates applicants, admins, and scanners. The
frontend calls the Go API, which owns the full application, decision, attendee,
pass, and redemption lifecycle. The Go API is the only application component
allowed to access PostgreSQL.

## Start here

Read [docs/README.md](docs/README.md) before implementing a milestone.

## Local commands

```text
npm install
npm run dev
npm run api:dev
npm run api:migrate
npm run api:seed-intake
npm run lint
npm run build
npm run api:test
```

On Windows machines where Avast HTTPS inspection is enabled, start the
frontend with `npm run dev:windows`. It makes Node trust the already-installed
Avast root certificate without disabling TLS verification.

Local PostgreSQL and the API are described in `docker-compose.yml`. Copy
`.env.example` and `api/.env.example` when configuring a local environment.

## Development intake fixture

After the local database is running and migrated, seed the active published form
with one cross-platform Compose command:

```text
docker compose run --rm --entrypoint /seed-intake migrate
```

The fixture is idempotent. It refuses to replace a different active cycle.

## Current status

Milestones 1–7 are implemented. The repository now covers application intake,
private drafts and submission, organizer/reviewer workflows, decisions and
attendee conversion, pass issuance/revocation/reissue, mobile QR scanning,
atomic entitlement-aware redemption, and organizer event operations/exports.

The next planned product slice is Milestone 8: Apple Wallet and Google Wallet
provider adapters. Email outbox records are transactional; production provider
delivery workers and the Milestone 9 readiness work remain to be completed.
