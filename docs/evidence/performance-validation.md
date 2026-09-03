# Performance validation campaign

Campaign started on 2026-09-02 (America/Toronto; artifact timestamps use UTC).
Test harness and evidence are in PR #109. The user approved a temporary exact
`codex/performance-validation` staging branch policy; production was not changed.
The campaign is in progress. Only completed results below are achieved results.

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
| Scanner repetitions 1–3 | 20 identities, 10 min each, fresh distinct passes, 2–5 s pacing | Pending |
| Applicant sustained | 50 applicants / 20 VUs; 512 KiB PDF uploads | Passed: 50/50 complete; form 281 ms, draft 468 ms, submit 443 ms, upload 425 ms p95 |
| Deadline | 25 submissions/min for 10 min; 250 prepared drafts | Passed: 250/250 submitted; 418 ms submit p95; no dropped arrivals |
| HTTP contention | 100 attempts, one pass, same-key replays | Pending |
| Separate spike | 100 distinct passes over 20 s, 20 identities | Pending |
| Deliberately unhealthy staging candidate | Detect, block non-deploying promotion canary, restore prior healthy configuration | Implemented and contract-tested; live drill pending |

Dispatch `33692976839` was rejected by GitHub's staging branch protection before
any job steps or load ran. It is not a benchmark or an application failure.
The temporary branch exception must be removed after the campaign, preserving
the original `main` and `staging` policies.

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
- Grafana 30-second samples: sustained max in-use 1/5 (20%), deadline 4/5 (80%).
  These are sampled maxima, not proof of instantaneous peak utilization or
  attribution of every connection to k6.
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
