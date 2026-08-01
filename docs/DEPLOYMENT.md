# Deployment

## Recommended topology

- Deploy the Next.js application to Vercel or from the root `Dockerfile`.
- Deploy the Go API from `api/Dockerfile` as a persistent container.
- Use the existing Supabase project strictly as managed PostgreSQL.
- Run `/migrate` as a one-shot release job before starting the new API image.
- Deploy email delivery and Wallet adapters as separate workers when those
  milestones are implemented.

The frontend and API should use separate HTTPS hostnames, for example
`apply.hackatlantic.ca` and `api.hackatlantic.ca`. HTTPS is required for mobile
camera access on the scanner route.

## Database connections

Use two database credentials:

1. `MIGRATION_DATABASE_URL` is the migration-owner direct connection.
2. `DATABASE_URL` is the restricted runtime application role.

When an IPv4-only host must authenticate to Supabase through the shared
session pooler with the project `postgres` credential, set
`DATABASE_ROLE=hackatlantic_app`. The API executes and verifies `SET ROLE` on
every newly opened connection before it enters the pool. Grant that role only
the `ats` schema privileges required by the API.

For Supabase, prefer its direct connection for migrations and for a persistent
Go service when the host supports IPv6. On an IPv4-only host, use Supavisor
session mode on port 5432 for the Go API. Do not use transaction mode unless
pgx prepared statements are explicitly disabled and tested. Always require
TLS with `sslmode=require` or certificate verification supported by the host.

The frontend must not use Supabase keys or its Data API. Keep the `ats` schema
private and accessible only to the migration owner and restricted API role.

## Resume storage

Create a private DigitalOcean Space using Standard Storage with its CDN
disabled. Create a limited access key scoped only to that Space with
read/write/delete access. Configure `SPACES_ENDPOINT`, `SPACES_REGION`,
`SPACES_BUCKET`, `SPACES_ACCESS_KEY_ID`, and `SPACES_SECRET_ACCESS_KEY` only on
the Go service. Never expose either Spaces credential as a `NEXT_PUBLIC_*`
value or send it to the browser. Objects are uploaded with a private ACL and
admins view PDFs through the authorized API stream.

The API validates the `.pdf` extension, PDF signature and trailer, and the 5
MiB size limit before upload. The Space is defense-in-depth object storage, not
an authorization boundary; keep its file listing and every stored object
private.

Local Docker development uses the persistent `hackatlantic-resumes` volume and
does not require a Supabase Storage bucket. Production admins view PDFs through
the authorized API stream, so the bucket must remain private.

## Clerk production setup

Create a Clerk production instance and production domain. Do not deploy the
local `pk_test_` or `sk_test_` values. Configure:

- `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY`
- `CLERK_SECRET_KEY`
- `CLERK_ISSUER_URL`
- `CLERK_JWKS_URL`
- `CLERK_AUTHORIZED_PARTIES` with the exact frontend origin

Also add the production frontend URL to Clerk's redirects and allowed origins.
Admin email access is stored in `ats.admin_email_allowlist`. The initial
migration seeds the two founding admins. Subsequent additions and removals take
effect on the next authenticated API request without restarting any service.

Until the admin-access UI is added, operators can manage the list with the
migration-owner connection:

```sql
INSERT INTO ats.admin_email_allowlist (normalized_email)
VALUES (lower(btrim('new-admin@example.com')))
ON CONFLICT (normalized_email) DO NOTHING;

DELETE FROM ats.admin_email_allowlist
WHERE normalized_email = lower(btrim('former-admin@example.com'));
```

## Secrets and runtime configuration

Start from `.env.production.example`, but store values in the hosting
provider's secret manager rather than a committed file. Generate the two pass
peppers independently:

```text
openssl rand -base64 32
openssl rand -base64 32
```

Do not deploy the predictable localhost peppers from `.env`. Pepper rotation
requires a pass migration/reissue plan. Configure `TRUSTED_PROXY_CIDRS` only
for known proxy networks; otherwise leave it empty.

Public claim URLs contain bearer credentials. Disable or redact access logging,
analytics, tracing, and error reporting for `/claim/*` and `/v1/claim/*` paths.

## Container deployment

The provider-neutral production Compose file can be validated and used with:

```text
docker compose --env-file .env.production -f docker-compose.production.yml config
docker compose --env-file .env.production -f docker-compose.production.yml up -d --build
```

On managed hosts, deploy the `web` and `api` build contexts separately and run
the `migrate` image command as the release phase. Never run the development
seed against production.

## First production release

1. Create restricted database and migration-owner credentials.
2. Configure production Clerk and DNS.
3. Create the private resume Space and store every value from
   `.env.production.example` in provider secrets.
4. Build the frontend with the final public API URL and Clerk publishable key.
5. Run migrations once and verify `/readyz` against Supabase.
6. Deploy the API, then the frontend.
7. Sign in with each seeded admin email and verify `/v1/me` returns `admin`.
8. Test applicant submission, organizer review/release, pass issuance, and a
   physical-phone camera scan over HTTPS.
9. Verify CORS rejects an unrecognized origin and that logs contain no bearer
   credentials or applicant answers.

## Release checks

- `npm run lint`
- `npm run build`
- `go -C api test -p 1 ./...`
- `go -C api vet ./...`
- `npm run api:test:integration`
- `GET /healthz` returns `ok`
- `GET /readyz` returns `ready`

Email provider delivery, operational monitoring/alerting, backup restoration,
and Apple/Google Wallet issuance remain deployment gates for their respective
milestones; they do not block testing the ATS lifecycle in a staging environment.
