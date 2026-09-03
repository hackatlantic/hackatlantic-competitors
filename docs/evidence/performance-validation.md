# Performance validation campaign

> Follow-up: staging form alignment and applicant reruns were subsequently
> approved. See [current nine-question form evidence](current-form-applicant-validation-2026-09-03.md)
> for the new measurements and retained failed first deadline run. The original
> campaign below is preserved; its old-form measurements are not relabelled.

Campaign started on 2026-09-02 (America/Toronto; artifact timestamps use UTC).
Test harness and evidence are in PR #109. The user approved a temporary exact
`codex/performance-validation` staging branch policy; production was not changed.
Execution and cleanup are complete. **Not all acceptance criteria passed:**
the HTTP contention test failed, the current production form was not exercised,
and fault detection plus restoration exceeded the proposed five-minute target.
PR #109 remains a draft; these results do not authorize a production deployment.

## Verification completed

- Local workload contracts and initialization of all seven k6 profiles pass.
- The PostgreSQL integration suite passed in CI run `33692977849`, including
  100 synchronized attempts against one single-use pass (one redeemed, 99
  exhausted), an unrelated pass completing while a redemption is paused, and
  checkpoint configuration updates waiting for in-flight redemption.
- No API locking, schema, instance size, or production settings were changed.

## Live campaign

| Test | Configuration | Result |
| --- | --- | --- |
| Scanner repetition 1 | 20 identities, 10 min, fresh distinct passes, 2–5 s pacing | Passed: 2,857 passes; lookup 278 ms / redemption 517 ms p95; zero failures in 5,714 requests |
| Scanner repetition 2 | Same workload and deployed API; fresh passes | Passed: 2,865 passes; lookup 257 ms / redemption 506 ms p95; zero failures in 5,730 requests |
| Scanner repetition 3 | Same workload and deployed API; fresh passes | Passed: 2,794 passes; lookup 344 ms / redemption 588 ms p95; zero failures in 5,588 requests |
| Applicant sustained | 50 applicants / 20 VUs; 512 KiB PDF uploads | Passed: 50/50 complete; form 281 ms, draft 468 ms, submit 443 ms, upload 425 ms p95 |
| Deadline | 25 submissions/min for 10 min; 250 prepared drafts | Passed: 250/250 submitted; 418 ms submit p95; no dropped arrivals |
| HTTP contention | 100 attempts, one pass, same-key replays | **Failed:** 88/300 HTTP 5xx; ledger still exactly one redemption |
| Separate spike | 100 distinct passes over 20 s, 20 identities | Passed: 100 unique redemptions, 200 requests, zero failures or dropped arrivals |
| Deliberately unhealthy staging candidate | Detect, block non-deploying promotion canary, restore prior healthy configuration | Containment/restoration passed: detection 692.167 s, restoration 23.807 s after rollback request; **five-minute end-to-end target not met** |

Dispatch `33692976839` was rejected by GitHub's staging branch protection before
any job steps or load ran. It is not a benchmark or an application failure.
The temporary branch exception was removed after verified restoration. Only the
original `main` and `staging` deployment branch policies remain.

### Completed applicant evidence

- [Sustained run 33706214681](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33706214681):
  300 measured HTTP requests, zero failures; SQL verified 50 submissions, no lost
  answers, no duplicate applications, and 50 matching persisted resumes.
- [Deadline run 33706868836](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33706868836):
  250 measured submit requests, zero failures; SQL verified all 250 submissions,
  no lost answers, no duplicates, and all 250 prepared resumes matching.
- Active staging form required a resume: **all** sustained applicants uploaded,
  rather than the optional-upload half selected when the form permits it.
  Deadline drafts and uploads were prepared before the measured submit window.
- Subsequent scanner context exposed the existing staging form as version 1,
  **three questions**, resume required. These applicant results validate the
  staging API journeys, **not parity with the current nine-question, optional-
  resume production form**. This gap required separate approval, subsequently
  granted for the [current-form follow-up](current-form-applicant-validation-2026-09-03.md).
- Grafana 30-second samples, filtered by source timestamp to exclude preparation:
  sustained max in-use 1/5 (20%). Deadline samples recorded zero in-use at their
  sample instants; its sub-second, 25/minute queries fall between samples, so this
  **does not mean the database was unused**. An initial unfiltered range included
  a preparation sample of 4/5; it is not a deadline-workload utilization result.
  Sampled maxima do not establish instantaneous peak utilization or attribution
  of every connection to k6.
- Both runs used API commit `21b1a585a75d6995a66a5261571a23eb9d6d72a2`, image
  `sha256:affbccdda09a774498ddba395111ead18b0649a040d93269c9600bc843c15b6f`,
  one `apps-s-1vcpu-0.5gb` instance in `tor`, and a five-connection API pool.
  No deployment changed during measurement. GitHub-hosted runner geography is
  uncontrolled; these are not same-region network benchmarks.
- Resume size 524,288 bytes; SHA-256
  `6ea3a1ad97ce843114c043efd4ccdb53122f7ef8275882edb838ebbd19454fea`.
  Upload latency includes sending the request body, unlike k6's default HTTP
  request duration. Initial runs did not record form ID/version metadata; the
  harness now records that metadata for later runs.

The older 100-simultaneous-applicant stress run `32664233051` recorded one
failed upload and 6,150.656 ms overall p95, but did not preserve its failing HTTP
status or error code. That root cause cannot be established retrospectively
from the available summary. Its workload and PDF size differ from this campaign;
the new passing results are **not** a before/after optimization claim and do not
prove that old instantaneous stress case was fixed.

### Scanner repetition evidence

[Run 33707641714](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33707641714)
uses the same deployed API and size as the applicant runs. Repetition 1 completed
2,857 scans at 282.35 scans/minute over a 607.123-second recorded window (including
graceful completion/verification). SQL verified 2,857 distinct attendees and no
over-limit redemptions. Lookup p95 was 277.895 ms and redemption p95 516.944 ms.
These are per-repetition percentiles, not a pooled campaign percentile.

Repetition 2 completed 2,865 scans at 283.28 scans/minute over 606.825 seconds,
with lookup p95 257.416 ms, redemption p95 505.842 ms, and zero failures across
5,730 requests. Its ledger likewise reported no duplicate or over-limit entries.
Both runs reached 5/5 in-use database connections in timestamp-filtered
30-second samples; low latency does not mean the scanner workload never fills
the pool.

Repetition 3 completed 2,794 scans at 276.13 scans/minute over 607.100 seconds,
with lookup p95 343.818 ms, redemption p95 587.984 ms, and zero failures across
5,588 requests. Its ledger verified zero duplicate/over-limit redemptions.

Across the three ten-minute measurements: **8,516 distinct passes and 17,032
HTTP requests**, with zero observed failures, duplicate redemptions, or
over-limit redemptions. The **worst per-run** p95 was 343.818 ms for lookup and
587.984 ms for redemption; do not describe these as pooled percentiles or
extrapolate a guarantee beyond the tested workload. Throughput was 276–283
completed scans/minute with twenty scanner clients and 2–5-second pauses.

The first attempt of repetition 3 failed before fixture setup or k6 execution:
Terraform initialized its backend, then its state-output lookup reported
`Name has already been taken` for the existing workspace. Only the failed job
was rerun (workflow attempt 2), with the same test commit and workload. Successful
repetitions 1 and 2 were not rerun or discarded. Preserve this setup failure in
the campaign history; it is not an application latency/error measurement.

Sanitized aggregate evidence, including raw pool gauge sample arrays, is retained
in [the campaign JSON](performance-validation-2026-09-03.json). Null values mean
not measured or unavailable. Workflow artifacts have a separate 30-day lifetime.

### Contention failure and separate distinct-pass spike

[Contention run 33711277613](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33711277613)
sent 100 lookups, 100 initial redemption attempts and 100 same-key replays,
sharing twenty scanner identities. It observed 88 HTTP 5xx responses out of 300
(29.33%). Initial outcomes were one `redeemed`, 76 `already_exhausted`, and 23
failed HTTP responses; 88 replay comparisons failed. The ledger verified one
redemption and exactly 100 distinct request records, so replay traffic did not
create extra request entries. **That persistence result does not turn the failed
HTTP/idempotency acceptance test into a pass.** Overall request p95 was
16,230.568 ms; this is an adversarial, unpaced hot-pass workload, not the release
performance standard.

The service serializes the same pass and uses a fifteen-second transaction
deadline with a five-connection pool. Queueing/timeouts are a hypothesis
consistent with the observed latency, not an established root cause: the
response summary does not contain the server's underlying error. No timeout,
locking, pool size, or error threshold was changed to make this test green.
Further targeted diagnosis and any API change require a separate reviewed fix.

[Distinct-pass spike run 33711427592](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33711427592)
scheduled 100 scans at five/second for twenty seconds. It completed 100 distinct
redemptions with zero failures in 200 requests and no dropped arrivals. It is
**not** a claim of 100 simultaneous scanner devices.

### Unhealthy staging candidate and restoration

[Drill 33711524311](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33711524311)
changed only staging's readiness-probe path to a nonexistent route. The image,
liveness probe, application code, credentials and instance size were unchanged.
DigitalOcean rejected the candidate with `ContainerHealthChecksFailed`.

| Milestone | UTC time / measured duration |
| --- | --- |
| Fault submitted | 2026-09-03 03:29:25.064 |
| Health-check rejection detected | 03:40:57.231; **692.167 s** after submission |
| Rollback requested | 03:40:57.885 |
| Original digest and readiness verified restored | 03:41:21.692; **23.807 s** after rollback request |
| Submission to verified restoration | **716.628 s (11 min 56.628 s)** |

The prior healthy deployment continued serving: zero unhealthy results in 112
readiness samples. This proves rejection/containment and restoration of desired
configuration, **not recovery from a user-visible outage**. The workflow checks
restoration within five minutes of the rollback request and passed, but the
broader proposed five-minute submission-to-restoration criterion **did not**.
Do not omit detection time from a claim about total recovery time.

Restoration verified the original digest, a new active deployment, `/readyz` in
both active and desired configuration, the original public API commit, and no
rollback pin that could prevent later deployments. The non-deploying promotion
canary was skipped; its verification job and the contract check of the real
release dependency passed. No production deployment was attempted, and no
incident-notification delivery claim follows from this test.

### Cleanup and remaining decisions

- Restored staging; final public staging and production checks both returned
  `ready` and API commit `21b1a585a75d6995a66a5261571a23eb9d6d72a2`.
- Removed only temporary deployment branch policy `58974493`; original policies
  `58772100` (`main`) and `58772112` (`staging`) remain. Production protections
  were not edited.
- Revoked the temporary Grafana evidence-reader token and stopped the local
  collector. Normal telemetry ingestion and existing alerts were not disabled.
- Fixture jobs cleaned up temporary scanner access/admin allowlist changes.
  Synthetic benchmark records remain in staging; this was not a database purge.
- PR #109 stays a draft. Staging form alignment and applicant reruns were later
  approved and are reported in the follow-up. A narrowly scoped investigation
  of the failed hot-pass HTTP test remains separate; passing everyday scanner
  results do not conceal that failure.

## Required evidence before making claims

Keep the k6 summary, workload/deployment context, SQL aggregate validation and
Grafana connection-pool range samples together for each run. Report individual
repetition results, not just the best result. Unexpected transport errors,
missing completions, token expiry, dropped arrivals and persistence mismatches
must remain visible.

For recovery, record candidate start, failure detection, rollback request,
restored digest/readiness, and both detection and recovery durations. The drill
uses a non-deploying canary plus a contract check of the real production job's
staging dependency; it does not dispatch production. A rejected candidate that
never received traffic demonstrates containment, not recovery from a user
outage. Database restore RTO and delivered incident alerts remain separate,
unproven results in this campaign.
