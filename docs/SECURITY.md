# Security invariants

## Trust boundaries

- The browser is untrusted.
- QR content is an untrusted bearer credential until verified.
- Clerk proves applicant and staff identity; it does not prove application
  authorization by itself.
- The Go API is the only application database principal.
- Email and Wallet providers are external systems.

## Authenticated requests

The API must verify Clerk JWT signature, issuer, expiry, not-before time, and
authorized party/audience as configured. JWKS must be cached. After identity
verification, the API must resolve a local user and check the required
`user_roles`.

Applicants may access only their own applications and only released decisions.
Reviewer access is limited to assigned/permitted applications. Scanners never
receive application answers, decisions, or review data. Organizers control
privileged role assignments.

Production CORS must use an explicit allowlist. Credentialed requests must not
use `Access-Control-Allow-Origin: *`.

JSON request bodies are capped at 1 MiB before decoding.

## Attendee secrets

Claim tokens and QR credentials must:

- contain at least 128 bits of cryptographically secure randomness;
- use distinguishable prefixes and versions;
- be separate values with separate purposes;
- be stored as keyed hashes or hashes plus an application pepper;
- support revocation and reissue;
- never appear in logs, analytics, error reports, or database query output.

QR and claim HMAC peppers are distinct standard-base64 secrets containing at
least 32 random bytes. They are server-only and require a migration plan before
rotation because existing pass hashes depend on them.

Public claim URLs contain bearer material. Reverse proxies and hosting layers
must redact `/v1/claim/*` and frontend `/claim/*` request paths from access logs,
analytics, traces, and error reporting. The Go API does not emit request URLs.
When deployed behind a proxy, configure `TRUSTED_PROXY_CIDRS`; forwarded client
addresses are ignored unless the immediate peer is explicitly trusted.

Do not encode names, emails, attendee IDs, eligibility, or redemption state in
the QR. Apple and Google passes carry the same opaque QR credential.

## Database

- Use a restricted application role, not a database owner.
- Require TLS in production.
- Apply parameterized queries only.
- Set query and transaction timeouts.
- Keep migrations in source control.
- Back up production and test restore procedures before the event.
- Do not rely on frontend checks or RLS as the only authorization layer.

### Supabase and privileged access

- Supabase is managed PostgreSQL only. Disable its Data API in production; it
  must not expose the `ats` schema, its tables, functions, or migration ledger.
- The migration owner is distinct from the restricted Go API database role.
  `PUBLIC`, `anon`, `authenticated`, and `service_role` receive no ATS access.
- Admin access is stored in PostgreSQL's `ats.admin_email_allowlist` and
  derived on every authenticated request by exact, case-insensitive comparison
  with the verified primary email retrieved from Clerk. The browser cannot
  assert an email or role. Database changes take effect on the next request.

## Redemption

Redemption limit enforcement must happen in one database transaction. A
frontend lookup followed by a separate update is not valid. Concurrent scans
of the same pass must serialize and produce at most the permitted number of
redemptions.

The audit record includes the staff identity, checkpoint, attendee, pass,
timestamp, and request/idempotency identifier.

## Application and review privacy

Applications can contain sensitive personal information. API response models
must be actor-specific:

- applicants see their own answers and released decision;
- admins see submitted applications, private reviews, decisions, and
  operationally required data;
- scanners see only minimal attendee verification data.

Reviewer identity, notes, scores, recommendations, and unreleased decisions
must never appear in applicant responses or emails.

Submitted answers cannot be mutated without an explicit organizer reopen
operation and audit event. Decision history is append-only.

Resumes are accepted only as PDF files no larger than 5 MiB. The Go API checks
the extension, declared media type, and PDF signature before writing to a
private storage bucket. Supabase's service-role credential remains server-only;
the browser receives resume bytes only from an authenticated, authorized API
endpoint. Production should add malware scanning and a documented retention
policy before applications open.

## Personally identifiable information

Return the minimum information required by each screen. A scanner may need a
display name and optional verification photo; it does not need an email,
application answers, or unrelated profile details.

Operational logs should use stable IDs rather than names or emails.

## Secrets

Local examples contain placeholders only. Production secrets belong in the
deployment platform's secret store. Never commit:

- database credentials;
- Clerk secret keys or session tokens;
- email provider credentials;
- token peppers;
- Apple certificates or private keys;
- Google service-account credentials.
