# Current-form applicant validation — 2026-09-03

Follow-up to the [original performance campaign](performance-validation.md).
The user approved publishing the current nine-question form in staging and
repeating the sustained and deadline applicant profiles. The earlier results
are retained; the scanner contention failure is not retested or reclassified.

**The current-form sustained profile and final deadline rerun passed.** The
initial deadline attempt failed in the harness and is retained below.
Sanitized source aggregates and artifact hashes are in the
[companion JSON](current-form-applicant-validation-2026-09-03.json).

## Scope and form identity

Staging published immutable form version **2** (`ddbcded1-5128-4a39-9d00-0d12b688523b`):
nine questions, optional résumé, and conditional hardware-equipment input.
The served form matches the approved schema in migration `000013`, fingerprint
`2cbcf487148f2c5ec14aad434fdb316ca488705784c6dc595efdf407c11c0a19`.
The helper reads that schema without running the migration or moving existing
drafts. Previous forms and application records remain in place.

No production database access, API deployment, scanner test, locking change,
pool resizing, or infrastructure change was part of this follow-up. Both API
endpoints initially reported commit `21b1a585a75d6995a66a5261571a23eb9d6d72a2`.

## Sustained applicant journeys — passed

[Run 33723965114](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33723965114)
used 50 distinct applicants across 20 VUs, a staggered start, 20–45-second pauses,
two draft revisions, and a fixed 512 KiB PDF for half the applicants.

| Operation | Measured p95 | Acceptance target |
| --- | ---: | ---: |
| Load form | 264.325 ms | <1,000 ms |
| Create draft | 360.036 ms | <1,000 ms |
| Save draft | 475.332 ms | <1,000 ms |
| Upload résumé, including body transfer | 399.600 ms | <3,000 ms |
| Submit application | 412.085 ms | <2,000 ms |

- **50/50 completed journeys**, 275 HTTP requests, zero HTTP failures.
- **25 submitted with a résumé and 25 without**, confirmed in the database.
- Zero lost answers, duplicate applications, wrong-form applications, or
  résumé metadata/checksum mismatches. Every application used the new form.
- Recorded window: 06:36:18.233–06:45:18.478 UTC (540.245 seconds, including
  completion/verification overhead). Form and deployed API stayed unchanged.

## Deadline submissions

### Initial run — not accepted

[Run 33724749486](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33724749486)
completed **250/250 submissions** at **424.750 ms submission p95**, with zero
HTTP failures and no dropped iterations. Nevertheless, the workflow failed:

- At the ten-minute boundary, k6 scheduled iteration 251 with no corresponding
  fixture and aborted with exit code 108. Successful request thresholds alone
  do **not** make that execution a passing run.
- The SQL verifier exited before producing a ledger artifact. Its detailed
  process error was suppressed to keep fixture data private. Reconstructing
  the current-form expectation array measured **132,491 bytes**, exceeding
  Linux's 128 KiB per-argument limit. The old code passed that entire array in
  one `psql -v` argument. This explains the larger-form-specific verifier risk;
  the suppressed log alone does not conclusively prove its original error code.

The first run's persistence checks remain **unverified**, not zero-loss claims.
Its results remain in the companion evidence, rather than being overwritten.

### Bounded harness corrections and fresh rerun

Only the test harness changed: large expectation JSON now travels as a
base64-encoded SQL literal through `psql` standard input, avoiding argument
limits. A boundary guard allows only index 250 after 599.9 seconds with exactly
250 fixtures. It makes no HTTP request; early fixture exhaustion or later
out-of-range indices still abort. An explicit counter limits ignored boundary
callbacks to one. The 250 applicant workload, rate, duration, latency/error
thresholds, database checks, API build, and instance configuration are unchanged.

All 19 local regression tests passed, including initialization of all seven k6
profiles, large-payload round-trip/escaping, and boundary guard tests.

### Final deadline rerun — passed

[Run 33726354766](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33726354766)
used fresh fixtures and the same nine-question form, API build, instance, and
25-submissions/minute, ten-minute workload:

- **250/250 completed submissions**, **435.915 ms submission p95** (<2,000 ms).
- **250 HTTP requests, zero failures, zero dropped iterations**.
- SQL verified **250 submitted applications**, all using form version 2, with
  **zero lost answers, duplicates, wrong-form applications, or résumé mismatches**.
- This is a submission-only benchmark: all drafts were prepared beforehand and
  all 250 omitted the optional résumé. Upload performance belongs to the sustained
  run above, not this result.
- k6 counted 251 iterations: 250 real submissions and **one separately recorded
  no-request endpoint callback**. No extra attendee or submission is claimed.
- Recorded window: **07:06:34.485–07:16:36.565 UTC** (602.080 seconds including
  completion/verification overhead). API deployment and form remained unchanged.

The entire workflow, including SQL verification and access cleanup, passed.
The successful larger-payload verifier on this fresh run supports the stdin fix;
it does not retroactively verify the earlier failed run's database records.

## Interpretation

These are API journeys authenticated with isolated synthetic staging identities,
not browser-rendering, real Clerk sign-in, or every answer-branch tests. The
answers exercise the hardware-project/equipment path. SQL verifies persisted
résumé metadata and checksums, not a separate object-download/restore drill.

The older staging form required all applicants to upload; this form does not.
Runner geography is uncontrolled. Differences from the earlier p95 results
are **not a controlled before/after optimization claim**. This follow-up does
not fix or invalidate the original hot-pass contention failure or the recovery
drill exceeding its five-minute end-to-end target.

Connection-pool telemetry was not collected for this follow-up; unavailable
values are not zero utilization. Existing telemetry ingestion was not disabled.

## Provenance and cleanup

Sustained and initial deadline test commit:
`0820f3d669892fed8e1e26fe967a0e959e280919`.
Final deadline harness commit: `a2646744cbea78a2179fefa9af3ec73d42a6ae3b`.
The staging API ran digest
`sha256:affbccdda09a774498ddba395111ead18b0649a040d93269c9600bc843c15b6f`,
one `apps-s-1vcpu-0.5gb` instance in `tor`. PDF SHA-256:
`6ea3a1ad97ce843114c043efd4ccdb53122f7ef8275882edb838ebbd19454fea`.

- All three workflows completed temporary staff/allowlist cleanup successfully.
- Removed temporary staging branch policy **58990816**. Only original policies
  **58772100** (`main`) and **58772112** (`staging`) remain. Production protections
  were not edited.
- Final public checks returned `ready` for both staging and production, still on
  API commit `21b1a585a75d6995a66a5261571a23eb9d6d72a2`.
- The approved nine-question staging form remains published; old forms and
  synthetic test records remain intact. No applicant records were copied from
  production, and no test-record purge was performed.
- No temporary Grafana reader was created for this follow-up. Normal ingestion
  and alerts were not changed.
- PR #109 was draft and unmerged when this follow-up was published. Its later
  finalization merges the testing tools and sanitized reports, not a claim that
  every campaign target passed. Production promotion requires separate approval.
