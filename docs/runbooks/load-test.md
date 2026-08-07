# Scanner load-test runbook

The release pipeline runs `tests/load/smoke.js` first and then gates production on `tests/load/scanner.js` at 25 concurrent virtual users for five minutes.

## One-time staging fixture

1. Use a staging-only Clerk scanner identity and grant it the database-backed `scanner` role.
2. Create a Clerk JWT template for load testing (recommended name: `k6-load`) with a lifetime of at least 10 minutes. Keep Clerk's required default session claims; do not add applicant data.
3. Create a staging-only activity and checkpoint whose window covers release testing.
4. Create and accept a synthetic applicant, issue its active pass, and grant enough checkpoint redemptions for repeated tests. Never use a real applicant or production QR token.
5. Store the synthetic fixture as protected staging configuration:
   - secret `K6_QR_TOKEN`
   - variable `K6_CHECKPOINT_ID`
   - variable `K6_SCANNER_USER_ID`
   - variable `K6_CLERK_JWT_TEMPLATE`

The preceding Playwright journey signs the scanner identity in. k6 then uses the protected Clerk backend key to mint a short-lived load-test JWT from that active session. Neither the Clerk key nor the QR token is printed.

## Local execution

Run the public smoke profile:

```bash
k6 run -e API_BASE_URL=http://localhost:8080 tests/load/smoke.js
```

Run the scanner profile only against a synthetic environment:

```bash
k6 run \
  -e API_BASE_URL=https://staging-api.hackatlantic.ca \
  -e SCANNER_TOKEN="$SCANNER_TOKEN" \
  -e QR_TOKEN="$QR_TOKEN" \
  -e CHECKPOINT_ID="$CHECKPOINT_ID" \
  tests/load/scanner.js
```

The gate fails when lookup p95 exceeds 500 ms, redemption p95 exceeds 750 ms, or the HTTP failure rate reaches 1%. Domain outcomes such as `already_exhausted` remain valid responses and do not hide transport/server failures.
