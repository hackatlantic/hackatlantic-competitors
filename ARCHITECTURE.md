# Architecture

This is the public technical overview. Detailed product language, security invariants, deployment settings, and runbooks live in [`docs/`](docs/README.md).

## Design principles

1. The Go API is the sole authority for authorization, lifecycle state, and database access.
2. Clerk proves identity; PostgreSQL-backed roles determine HackAtlantic access.
3. PostgreSQL constraints and transactions protect pass and redemption invariants under retries and concurrent scanners.
4. PII and bearer credentials never belong in browser telemetry, dashboards, or logs.
5. Staging is synthetic-only; the exact image digest tested there is promoted to production.

## Runtime containers and trust boundaries

```mermaid
flowchart TB
  subgraph Internet
    Applicant[Applicant]
    Staff[Admin or scanner]
  end
  subgraph Vercel
    Web[Next.js web application]
  end
  subgraph DigitalOcean App Platform
    API[Go API]
    Migration[Pre-deploy migration job]
  end
  subgraph Managed services
    Auth[Clerk identity and sessions]
    DB[(Supabase PostgreSQL: ats schema)]
    Storage[Private Spaces: PDF resumes]
    Metrics[Grafana Cloud: metrics and alerts]
  end
  Applicant --> Web
  Staff --> Web
  Web -->|Bearer JWT| API
  Web --> Auth
  API -->|verify identity| Auth
  API --> DB
  API --> Storage
  API -. metrics only .-> Metrics
  Migration --> DB
```

The browser never receives database, Spaces, or migration credentials. Clerk does not store application answers, roles, decisions, pass state, or redemption state. Grafana receives low-cardinality operational data only once the API exporter is configured.

## Release path

```mermaid
flowchart LR
  PR[Pull request] --> Checks[Tests, security, Terraform plan]
  Checks --> Merge[Reviewed merge to main]
  Merge --> Build[Build one GHCR image digest]
  Build --> Stage[Staging: migrate, deploy, smoke test]
  Stage --> Gate{Production gate}
  Gate -->|approved| Prod[Production: promote same digest]
  Gate -->|not approved| Stop[No production change]
  Prod --> Verify[Smoke test and deployment record]
  Verify -->|failure| Rollback[Restore prior API deployment]
```

The rollback restores application code/configuration, not a database migration. Schema changes are forward-only and must remain compatible through the release.

## Scanner redemption sequence

```mermaid
sequenceDiagram
  participant Scanner
  participant API as Go API
  participant DB as PostgreSQL
  Scanner->>API: POST /v1/scans/lookup
  API->>DB: Resolve pass and entitlement
  DB-->>API: Eligible attendee context
  API-->>Scanner: Lookup result
  Scanner->>API: POST /v1/redemptions with idempotency key
  API->>DB: Transactional redemption attempt
  alt entitlement available
    DB-->>API: Commit redemption
    API-->>Scanner: redeemed
  else already used, exhausted, or not entitled
    DB-->>API: Stable domain outcome
    API-->>Scanner: Structured outcome
  end
```

The redemption transaction—not the scanner UI—decides whether a redemption can occur. This is why concurrent requests cannot create an over-redemption.

## Source of truth

| Concern | Authority |
| --- | --- |
| Identity and sessions | Clerk |
| Roles, applications, decisions, passes, redemptions | Go API plus PostgreSQL |
| Database schema | Forward-only Go migrations in `api/migrations/` |
| Infrastructure | Terraform with separate HCP Terraform state per root |
| Public API contract | `openapi/openapi.yaml` |
| Operational evidence | `docs/evidence/` and workflow artifacts |

For package boundaries and the complete data model, continue to [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/DOMAIN_MODEL.md](docs/DOMAIN_MODEL.md).
