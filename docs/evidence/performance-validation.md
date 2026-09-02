# Performance validation campaign

Status on 2026-09-02: test-harness implementation in PR #109; live measurements
pending staging authorization. Proposed thresholds are not achieved results.

## Verification completed

- Local workload contracts and initialization of all seven k6 profiles pass.
- The PostgreSQL integration suite passed in CI run `33692977849`, including
  100 synchronized attempts against one single-use pass (one redeemed, 99
  exhausted), an unrelated pass completing while a redemption is paused, and
  checkpoint configuration updates waiting for in-flight redemption.
- No API locking, schema, instance size, or production settings were changed.

## Live runs still required

| Test | Configuration | Result |
| --- | --- | --- |
| Scanner repetitions 1–3 | 20 identities, 10 min each, fresh distinct passes, 2–5 s pacing | Pending |
| Applicant sustained | 50 applicants / 20 active VUs; half upload a 512 KiB PDF | Pending |
| Deadline | 25 submissions/min for 10 min; 250 prepared drafts | Pending |
| HTTP contention | 100 attempts, one pass, same-key replays | Pending |
| Separate spike | 100 distinct passes over 20 s, 20 identities | Pending |
| Deliberately unhealthy staging candidate | Detect, block promotion, restore prior healthy deployment | Not implemented or executed in this campaign yet |

Dispatch `33692976839` was rejected by GitHub's staging branch protection before
any job steps or load ran. It is not a benchmark or an application failure.
Staging currently permits `main` and `staging`, not the test branch.

## Required evidence before making claims

Keep the k6 summary, workload/deployment context, SQL aggregate validation and
Grafana connection-pool range samples together for each run. Report individual
repetition results, not just the best result. Unexpected transport errors,
missing completions, token expiry, dropped arrivals and persistence mismatches
must remain visible.

For recovery, the existing rollback workflow switches between healthy releases;
it does not yet prove unhealthy-candidate detection or promotion blocking. The
next drill must record candidate start, failure detection, rollback request,
restored digest/readiness, and both detection and recovery durations. Restrict
all mutations to the exact staging app and restore configuration in a finally
path. Verify the production promotion job remains blocked. A rejected candidate
that never received traffic demonstrates containment, not recovery from a user
outage. Database restore RTO is a separate unproven result.
