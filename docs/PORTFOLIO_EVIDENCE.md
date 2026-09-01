# Portfolio evidence checklist

Store redacted artifacts under `docs/evidence/` with a date and commit SHA.

- [ ] Runtime architecture and release pipeline diagrams
- [ ] Staging and production HCP workspace screenshots
- [ ] Resource count from `terraform state list` for both environments
- [ ] Production plan showing no unexpected delete or replacement
- [ ] Forked-PR run proving protected secrets are unavailable
- [ ] Required branch checks and CODEOWNER protection screenshot
- [ ] GHCR digest, SPDX SBOM, and verified attestation
- [ ] Staging and production `/versionz` responses showing the same SHA
- [ ] Broken staging deployment alert and measured rollback duration
- [ ] API and ATS Grafana dashboard screenshots
- [ ] Discord and email test alerts
- [ ] k6 release-gate summary at 20 concurrent scanners, plus separate 100-pass spike and same-pass contention reports
- [ ] Backup inventory and redacted monthly restore report
- [ ] Cloudflare DNS parity and DNSSEC validation

Capture values in `docs/evidence/measurements.md` before writing résumé bullets:

```text
Baseline release lead time:
Final release lead time:
Terraform-managed staging resources:
Terraform-managed production resources:
Scanner test VUs:
Scanner lookup p95:
Redemption p95:
Load-test error rate:
Latest measured RPO:
Latest restore RTO:
Rollback duration:
Released images with SBOM and provenance / total released images:
```

Google XYZ bullet templates:

- Reduced production release lead time from **X to Y** by implementing digest-pinned GitHub Actions promotion across DigitalOcean and Vercel.
- Provisioned **N resources across two isolated environments** using Terraform and HCP remote state, eliminating manually configured infrastructure drift.
- Sustained **25 concurrent scanner clients at X ms p95 latency and below 1% errors** by enforcing k6 performance gates and optimizing from OpenTelemetry evidence.
- Established a **24-hour RPO and X-minute tested RTO** through client-encrypted PostgreSQL backups and approved automated restore drills.
- Secured **100% of released API images** with vulnerability gates, SPDX SBOMs, and verifiable Sigstore/GitHub provenance.
