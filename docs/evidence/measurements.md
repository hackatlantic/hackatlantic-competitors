# Platform measurements

Record only redacted, reproducible measurements. Do not convert a failed or
invalidated run into a résumé claim.

## Baseline before run-scoped idempotency keys

| Measurement | Value |
| --- | --- |
| Workflow run | `31668949811` |
| API commit under test | `8fb0870dffcfe6d99df57687c8d38fef7c1a3c0a` |
| Profile | Scanner release topology: 20 VUs, 1,500 distinct passes |
| Lookup p95 | 3.07 s |
| Redemption p95 | 3.52 s |
| HTTP failure rate | 6% |
| Failure classification | 180 HTTP 409 idempotency conflicts; no evidence of 401 or 5xx responses |
| Correctness verification | Redemption ledger verification passed |
| Use in final claims | **Invalidated.** Keys were derived only from VU and iteration, so later fixture runs reused append-only idempotency records with different passes. |

## Closeout branch verification

| Measurement | Value |
| --- | --- |
| Commit | `9a3d635` |
| Component/regression tests | 7 passed, including cross-run idempotency uniqueness |
| Go verification | Race-enabled unit tests and `go vet` passed |
| PostgreSQL verification | Docker/PostgreSQL 17 integration suite and all migrations passed |
| Recovery command verification | PostgreSQL 17.11 dump/delete/restore drill recovered 22 ATS tables and 12 migration records from a 74,546-byte custom-format dump |
| Frontend verification | ESLint, TypeScript, and optimized Next.js build passed |
| Dependency audit | 0 npm vulnerabilities after upgrading transitive `nanoid` to 3.3.18 |
| k6 validation | All six fixed profiles parsed with pinned `grafana/k6:1.7.0` |

## Verified staging scanner measurements — September 2, 2026

These are successful synthetic staging runs, not production uptime measurements.
Both use 20 scanner identities, 1,500 distinct passes, and 2–5 second pacing.

| Measurement | Earlier release | Instrumentation release, attempt 1 |
| --- | --- | --- |
| Workflow run | [33654433935](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33654433935) | [33687509195](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33687509195/attempts/1) |
| API Git SHA | `2a3b073ebf8425297474db632bb15383c9d70e52` | `21b1a585a75d6995a66a5261571a23eb9d6d72a2` |
| Lookup p95 | 336.12 ms | 272.54 ms |
| Redemption p95 | 592.85 ms | 510.49 ms |
| HTTP failures | 0 / 3,000 | 0 / 3,000 |
| Ledger verification | 1,500 atomic entries; no duplicate attendees | 1,500 atomic entries; no duplicate attendees |

The earlier run lasted 5m22.9s. The instrumentation release's first attempt had
misconfigured exporter authentication; its passing application tests do **not**
prove telemetry ingestion. Its configuration-only rerun reuses image digest
`sha256:affbccdda09a774498ddba395111ead18b0649a040d93269c9600bc843c15b6f`.
Do not attribute differences between these runs to a controlled optimization.

### Corrected ingestion rollout — attempt 2

[Release 33687509195, attempt 2](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/33687509195/attempts/2)
reused that same digest, passed the staging gate in **5m15.9s**, and recorded:

- 20 scanner identities and 1,500 distinct passes.
- Lookup p95 **304.37 ms**; redemption p95 **558.23 ms**.
- **0 / 3,000** HTTP failures.
- **1,500 atomic ledger entries**, no duplicate attendees.

The production API apply and exact-digest health verification both passed, and
Grafana received production telemetry for the same Git SHA. The overall workflow
was nevertheless marked failed because its final frontend promotion returned
Vercel's specific HTTP 409: that deployment was **already current production**.
Do not present this attempt as a completely green workflow. The applicant portal
returned HTTP 200 afterward; the retry regression is covered separately.

### Applicant stress result — August 23, 2026

The sanitized `applicant-stress-32664233051/profile-summary.json` artifact from
[run 32664233051](https://github.com/hackatlantic/hackatlantic-competitors/actions/runs/32664233051)
records 100 iterations, 499 HTTP requests, 0.2004% HTTP failures, and 6,150.66 ms
overall HTTP request p95. All 100 application creations and draft saves passed;
99 résumé uploads and 99 submissions succeeded, with one upload failure.
The workflow passed its configured error/check thresholds, but this is not a
zero-failure result or evidence of acceptable everyday applicant latency.

## Measurements still to establish

Populate remaining fields only from successful workflow artifacts and actual
recovery/availability observations. A configured target is not an achieved SLO.

```text
Release workflow run:
Released Git SHA:
Staging API image digest:
Production API image digest:
Baseline release lead time:
Final release lead time:
Terraform-managed staging resources:
Terraform-managed production resources:
Scanner release-gate VUs:
Scanner release-gate duration:
Scanner lookup p95:
Scanner redemption p95:
Scanner transport/server error rate:
Scanner duplicate or over-limit redemptions:
100-pass spike lookup p95:
100-pass spike redemption p95:
100-pass spike error rate:
Same-pass contention redeemed outcomes:
Same-pass contention already-exhausted outcomes:
Latest measured RPO:
Latest restore RTO:
Rollback duration:
Released images with SBOM and provenance / total released images:
```
