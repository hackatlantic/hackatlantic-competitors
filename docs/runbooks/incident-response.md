# Incident response runbook

Use the **Operational Alert Test** workflow after configuring or rotating alert
credentials. It requires both Discord and Resend delivery to succeed and emits
a Grafana marker when OTLP credentials are present. Run it separately against
the `staging`, `backup`, and `disaster-recovery` environments because GitHub
environment credentials are isolated.

## Triage

1. Acknowledge the alert in Discord/email and name an incident lead.
2. Note environment, first failure time, affected journey, deployed SHA/digest, and request IDs.
3. Inspect the API overview, ATS operations, recent deployments, and synthetic history.
4. Classify availability, latency, authorization/privacy, database, storage, authentication, or DNS.

## Containment

- Bad release: execute the rollback runbook.
- Elevated 5xx/latency: stop releases, inspect route-template metrics and database pool saturation, then reduce traffic or scale within the approved cost ceiling.
- Authorization/privacy concern: disable the affected route or role assignment path and treat it as a security incident.
- Database corruption/loss: stop writes and invoke approved disaster recovery.
- DNS/email issue: use the DNS migration rollback and verify DNSSEC state.

## Communication and closure

Post status at detection, containment, recovery, and closure. Never paste applicant PII, résumé metadata, bearer tokens, QR/claim URLs, database connection strings, or raw production rows. After recovery, capture time-to-detect, time-to-contain, time-to-recover, customer impact, root cause, and owners/dates for corrective actions.
