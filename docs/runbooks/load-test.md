# ATS load-test profiles

Production is never a load-test target. The release pipeline always runs the public health smoke test. Authenticated ATS and scanner profiles run manually against staging through **Staging ATS load profiles** (`.github/workflows/meaningful-load-test.yml`).

## Scanner profiles

| Profile | Workload | Purpose | Gate |
| --- | --- | --- | --- |
| `scanner-release` | 20 scanner identities, 1,500 distinct passes, 2–5 seconds between scans; finishes when all passes are consumed | Existing release benchmark, duration varies with latency | Lookup p95 <750 ms; redemption p95 <1,000 ms; system errors <1%; every distinct pass redeemed once |
| `scanner-repeatability` | 20 identities for a full 10 minutes, 2–5 second paired jitter, up to 3,500 fresh passes; select three repetitions | Repeatability, not higher account counts | Same latency/error targets; completed-scan count must match the ledger, with zero duplicates or ordinals above one |
| `scanner-spike` | 100 distinct passes at 5 scans/second for 20 seconds | Abrupt capacity spike | System errors <1%; latency is reported, not used as a release gate |
| `scanner-contention` | 100 attempts against one pass | Adversarial atomicity and idempotency proof | Exactly one `redeemed`, 99 `already_exhausted`, identical replay results, system errors <1% |

`already_exhausted`, `not_entitled`, and other redemption outcomes are domain results returned in an HTTP 200 body. They are not transport/server failures. The scripts count HTTP failures separately from unexpected domain outcomes.

The release profile consumes one distinct attendee pass per scan. Scanner identities are reused round-robin because real devices perform many scans; a scanner account is not created per attendee.

## Applicant profiles

| Profile | Workload | Purpose |
| --- | --- | --- |
| `applicant-sustained` | 50 applicants across 20 active users ramped over one minute, two distinct draft revisions, 512 KiB résumé uploads for half the applicants even when optional, 20–45 second think times | Normal application intake |
| `applicant-deadline` | 250 fully prepared applications, 25 submissions/minute for 10 minutes using an arrival-rate executor | Realistic deadline burst |
| `applicant-stress` | 100 applicants completing the entire lifecycle simultaneously | Explicitly non-realistic stress/capacity test; enforces correctness and `<1%` transport/server errors while reporting, rather than gating on, normal-traffic latency targets |

The deadline fixture prepares drafts and résumés before k6 starts; the measured workload is therefore the submission spike rather than application setup.

Sustained acceptance criteria: form and draft p95 <1,000 ms, submission p95
<2,000 ms, fixed-size upload wall-clock p95 <3,000 ms, HTTP errors <1%, and at
least 99% complete journeys. Deadline uses the same submission and journey
targets and requires zero dropped scheduled arrivals. Stress reports each
operation separately without applying normal-traffic latency gates.

Post-run SQL checks inspect only fixture-owned accounts: submitted answers must
match the final revision, applications must be unique per account/cycle, and
required synthetic uploads must have the expected 524,288-byte size and SHA-256.
Only aggregate counts are published, never answers, emails, tokens, or QR codes.
The upload check validates persisted metadata; it is not a backup/restore test.

## Running a profile

From GitHub Actions, dispatch **Staging ATS load profiles** and choose one fixed
profile and one or three repetitions. Repetitions run serially with independent
fixture IDs. Run the different workloads separately so one does not contaminate
another. The fixed topology prevents an ad-hoc stress value from becoming a
release standard. Environment branch protection still applies; do not weaken it
or replace a protected branch to get a test to run without authorization.

For local script validation with a staging-only fixture:

```bash
K6_SCANNER_PROFILE=release \
K6_SCANNER_VUS=20 \
K6_SCANNER_ITERATIONS=1500 \
k6 run \
  -e API_BASE_URL=https://hackatlantic-api-staging-5c4l8.ondigitalocean.app \
  -e CHECKPOINT_ID="$CHECKPOINT_ID" \
  -e K6_SCANNER_FIXTURES=../../.tmp/k6-staging-fixture.json \
  tests/load/scanner.js
```

The workflow creates staging-only HMAC-authenticated identities, accepted attendees, passes, and checkpoints. Temporary administrator/scanner access is removed even when a test fails. Synthetic ATS ledger records remain in staging for auditability and use `hat_load` identifiers. Secrets, QR tokens, and scanner tokens are never uploaded with the sanitized k6 summary.

The scripts refuse any API origin or database project other than the known
staging targets. Requests do not follow redirects. Longer runs select overlapping
short-lived synthetic tokens; the API's ten-minute maximum token lifetime is
unchanged. Each scanner identity is reused across many distinct attendees.

Artifacts contain `profile-summary.json`, `benchmark-context.json`, and an
aggregate ledger report. Context records the API SHA/digest, instance size/count,
region, test commit, time window and PDF checksum. A mid-test deployment change
invalidates comparison. Completed scans/minute uses the recorded window including
graceful completion and verification overhead, not peak instantaneous throughput.

For connection-pool evidence, query these existing Grafana metrics over the exact
recorded window, selecting `service_name="hackatlantic-ats-api"` and
`deployment_environment_name="staging"`:

- `db_client_connections_open`
- `db_client_connections_in_use`
- `db_client_connections_max`

Save the range-query results with the report. These gauges are sampled every
30 seconds, so a sampled maximum is not a guarantee that no shorter spike
occurred. Mark missing telemetry as unavailable, never zero.

GitHub-hosted runners do not give this workflow a fixed physical test region.
This is recorded as uncontrolled. Do not use two such runs as a causal latency
improvement claim without a controlled runner location and identical payload,
API instance, and workload. The older 100-user stress result used a different PDF
and does not constitute an equivalent baseline for this 512 KiB workload.

Redemption idempotency UUIDs include the staging fixture run identifier. Never
replace them with VU/iteration-only values: redemption requests are append-only,
and cross-run key reuse correctly returns HTTP 409 when a new pass is supplied.
