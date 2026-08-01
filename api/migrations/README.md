# Migrations

Forward-only PostgreSQL migrations belong here. Every schema change must be
reviewed alongside its rollback or recovery notes.

The initial schema is delivered in the database milestone described in
`docs/MILESTONES.md`.

## Operational rules

- Migrations are forward-only. Never edit a committed migration after it may
  have been applied.
- The runner stores a SHA-256 checksum with every new migration and rejects a
  mismatch before executing subsequent migrations.
- Legacy rows without a checksum must be manually attested after comparing a
  backup/reviewed artifact with the deployed schema; the runner fails closed
  until then.
- The migration owner owns the `ats` schema. Production uses a distinct,
  restricted API database role and must not make Supabase Data API roles
  owners of ATS objects.
- Restore from backup or add a reviewed forward repair after a failed deployed
  migration; do not edit an applied file.

## Tests

`npm run api:test` runs unit tests without PostgreSQL. PostgreSQL integration
tests have the `integration` build tag and run against a disposable Compose
service through `npm run api:test:integration`.
