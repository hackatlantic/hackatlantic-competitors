# ATS load-test profiles

Production is never a load-test target. The release pipeline always runs the public health smoke test. Authenticated ATS and scanner profiles run manually against staging through **Staging ATS load profiles** (`.github/workflows/meaningful-load-test.yml`).

## Scanner profiles

| Profile | Workload | Purpose | Gate |
| --- | --- | --- | --- |
| `scanner-release` | 20 scanner identities, 1,500 distinct passes, 2–5 seconds between scans, approximately 7–9 minutes | Busy but realistic check-in or meal service | Lookup p95 <750 ms; redemption p95 <1,000 ms; system errors <1%; every distinct pass redeemed once |
| `scanner-spike` | 100 distinct passes at 5 scans/second for 20 seconds | Abrupt capacity spike | System errors <1%; latency is reported, not used as a release gate |
| `scanner-contention` | 100 attempts against one pass | Adversarial atomicity and idempotency proof | Exactly one `redeemed`, 99 `already_exhausted`, identical replay results, system errors <1% |

`already_exhausted`, `not_entitled`, and other redemption outcomes are domain results returned in an HTTP 200 body. They are not transport/server failures. The scripts count HTTP failures separately from unexpected domain outcomes.

The release profile consumes one distinct attendee pass per scan. Scanner identities are reused round-robin because real devices perform many scans; a scanner account is not created per attendee.

## Applicant profiles

| Profile | Workload | Purpose |
| --- | --- | --- |
| `applicant-sustained` | 50 applicants across 20 active users ramped over one minute, multiple draft saves, intermittent résumé upload, 20–45 second think times | Normal application intake |
| `applicant-deadline` | 25 fully prepared applications submitted evenly over one minute | Realistic deadline burst |
| `applicant-stress` | 100 applicants completing the entire lifecycle simultaneously | Explicitly non-realistic stress/capacity test |

The deadline fixture prepares drafts and résumés before k6 starts; the measured workload is therefore the submission spike rather than application setup.

## Running a profile

From GitHub Actions, dispatch **Staging ATS load profiles** and choose one fixed profile. The fixed topology prevents an ad-hoc stress value from accidentally becoming a release standard.

For local script validation with a staging-only fixture:

```bash
K6_SCANNER_PROFILE=release \
K6_SCANNER_VUS=20 \
K6_SCANNER_ITERATIONS=1500 \
k6 run \
  -e API_BASE_URL=https://staging-api.hackatlantic.ca \
  -e CHECKPOINT_ID="$CHECKPOINT_ID" \
  -e K6_SCANNER_FIXTURES=../../.tmp/k6-staging-fixture.json \
  tests/load/scanner.js
```

The workflow creates staging-only HMAC-authenticated identities, accepted attendees, passes, and checkpoints. Temporary administrator/scanner access is removed even when a test fails. Synthetic ATS ledger records remain in staging for auditability and use `hat_load` identifiers. Secrets, QR tokens, and scanner tokens are never uploaded with the sanitized k6 summary.

Redemption idempotency UUIDs include the staging fixture run identifier. Never
replace them with VU/iteration-only values: redemption requests are append-only,
and cross-run key reuse correctly returns HTTP 409 when a new pass is supplied.
