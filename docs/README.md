# Implementation guide

This directory is the handoff contract for milestone-based implementation.
Read these files before making changes:

1. [ARCHITECTURE.md](ARCHITECTURE.md)
2. [PRODUCT_LIFECYCLE.md](PRODUCT_LIFECYCLE.md)
3. [DOMAIN_MODEL.md](DOMAIN_MODEL.md)
4. [API_CONVENTIONS.md](API_CONVENTIONS.md)
5. [SECURITY.md](SECURITY.md)
6. [MILESTONES.md](MILESTONES.md)
7. [AGENT_HANDOFF.md](AGENT_HANDOFF.md)
8. [DEPLOYMENT.md](DEPLOYMENT.md)
9. [PLATFORM_ENGINEERING.md](PLATFORM_ENGINEERING.md)
10. [SLO.md](SLO.md)
11. [PORTFOLIO_EVIDENCE.md](PORTFOLIO_EVIDENCE.md)
12. [SECRETS_AND_ENVIRONMENTS.md](SECRETS_AND_ENVIRONMENTS.md)
13. [runbooks/load-test.md](runbooks/load-test.md)

## Authority

When sources conflict, use this order:

1. The current milestone and explicit maintainer instructions.
2. Architecture and security invariants in this directory.
3. The checked-in OpenAPI contract and migrations.
4. Existing implementation details.

Do not silently reinterpret a domain term. Update the relevant document in the
same change when a reviewed decision changes.

## Locked decisions

- Next.js is a frontend, not an application backend.
- Go owns all business rules and all PostgreSQL access.
- Clerk authenticates applicants, reviewers, organizers, and scanners.
- PostgreSQL stores application authorization and is the complete product
  source of truth.
- PostgreSQL may be hosted by Supabase or another provider without changing
  the application architecture.
- Applicants use Clerk accounts so drafts, submissions, and decisions belong
  to a stable identity.
- Accepted applications create attendees inside the same system; there is no
  external ATS import.
- Email delivery uses an external provider through a transactional outbox.
- QR and claim secrets are opaque, random, revocable, and stored only as
  hashes.
- The Wallet QR remains usable independently of a live Clerk session.
- A checkpoint is the scannable/redeemable unit.
- Application authorization roles and attendee entitlements solve different
  problems.
- Redemptions are enforced atomically by Go and PostgreSQL.
