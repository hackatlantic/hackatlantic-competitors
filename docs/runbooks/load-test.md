# Scanner load-test runbook

The release pipeline runs `tests/load/smoke.js` first. The manually dispatched **Meaningful staging load test** workflow creates staging-local synthetic identities, then runs a concurrent applicant submission burst and a distinct-pass scanner redemption burst against staging. Production is never a load-test target.

For an ad-hoc 100-user verification, dispatch `.github/workflows/meaningful-load-test.yml` with `concurrent_users=100`. It exercises API authentication, PostgreSQL draft/submission transactions, PDF resume storage, pass issuance, QR lookup, and atomic redemption. The controlled benchmark distributes 100 different accepted-attendee passes round-robin across 20 scanner identities. Temporary staff access is removed even when a test fails; synthetic ATS ledger records remain in staging for auditability and are recognizable by their `hat_load` run identifiers.

Terraform generates `LOAD_TEST_AUTH_SECRET` only for staging. The Go API refuses to start with that setting in any other deployment environment. The workflow reads the sensitive value from HCP Terraform state and locally creates HMAC-authenticated tokens that expire within ten minutes. Normal Clerk tokens continue through the unchanged Clerk JWT verifier. No load-test endpoint, password flow, browser session, or production bypass exists.

The workflow creates 20 scanner identities, one accepted attendee and active pass per virtual user, and a shared checkpoint. Neither the staging HMAC secret nor any QR or scanner token is printed or uploaded in the sanitized result artifact.

## Local execution

Run the public smoke profile:

```bash
k6 run -e API_BASE_URL=http://localhost:8080 tests/load/smoke.js
```

Run the scanner profile only against a synthetic environment:

```bash
k6 run \
  -e API_BASE_URL=https://staging-api.hackatlantic.ca \
  -e CHECKPOINT_ID="$CHECKPOINT_ID" \
  -e K6_SCANNER_FIXTURES=../../.tmp/k6-staging-fixture.json \
  tests/load/scanner.js
```

The gate fails when lookup p95 exceeds 500 ms, redemption p95 exceeds 750 ms, or the HTTP failure rate reaches 1%. Domain outcomes such as `already_exhausted` remain valid responses and do not hide transport/server failures.
