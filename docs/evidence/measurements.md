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

## Final release measurements

Populate these fields only from successful workflow artifacts after the closeout
commit is merged and the exact digest is deployed.

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
