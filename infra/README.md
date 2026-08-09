# Infrastructure activation guide

Terraform describes four isolated roots:

- `environments/staging`: new DigitalOcean, Supabase, and private résumé-storage resources.
- `environments/production`: existing production resources, imported before any apply.
- `global`: Cloudflare DNS, Vercel, GitHub protection, and deployment environments.
- `observability`: Grafana Cloud dashboards, SLO alerts, and Discord/email routing.

Each root uses a separate HCP Terraform workspace in **local execution mode**. HCP stores state and locks it; GitHub Actions performs plans and applies. The workspaces are `hackatlantic-ats-staging`, `hackatlantic-ats-production`, `hackatlantic-ats-global`, and `hackatlantic-ats-observability`.

## Non-negotiable production import gate

Never run a production apply against an empty state. Known existing resources use reviewed Terraform `import` blocks so the first protected plan shows adoption rather than creation. The currently confirmed imports are the production Supabase project, DigitalOcean API app, Vercel project, GitHub branch protection, and existing GitHub environments.

Storage, uptime, environment-variable, and DNS resources must be inventoried before their import blocks are added. If an imperative recovery import is ever required, use the exact provider IDs confirmed in the account UI:

```text
terraform -chdir=infra/environments/production init -backend-config=backend.hcl
terraform -chdir=infra/environments/production import supabase_project.database oizbfvfcownivwsrzlml
terraform -chdir=infra/environments/production import module.platform.digitalocean_app.api 852abc0e-75a9-45a8-ba13-57a5bbe50017
terraform -chdir=infra/environments/production import module.platform.digitalocean_spaces_bucket.resumes <region>,<existing-resume-bucket>
terraform -chdir=infra/environments/production import module.platform.digitalocean_spaces_bucket.backups[0] <region>,<existing-backup-bucket>
```

Import IDs must be confirmed from the provider documentation and account UI. Import the existing Supabase settings, uptime check, uptime alert, Vercel project/domain/environment variables, Cloudflare zone/records, and GitHub settings when they already exist. Do not guess IDs.

The import is accepted only when:

1. `terraform plan` contains no deletion or replacement.
2. Every field difference is understood and documented.
3. Conftest reports no policy violations.
4. Both administrators review the plan.

`prevent_destroy` protects production applications, Supabase projects, résumé storage, and backup storage. It is a final brake, not a substitute for reviewing a plan.

## Protected workflow inputs

Because these workspaces use local execution, HCP stores only state and locks. Protected GitHub environments supply provider credentials and one JSON tfvars document per root. Plan and drift credentials are read-only where the provider supports that scope. Providers that expose only account-level automation tokens use short-lived credentials isolated in the protected `terraform-plan` and `terraform-drift` environments; deployment environments receive mutation credentials only for their own environment.

The root payload secrets are `TFVARS_GLOBAL_JSON`, `TFVARS_STAGING_JSON`, `TFVARS_PRODUCTION_JSON`, and `TFVARS_OBSERVABILITY_JSON`. Provider secrets are `DIGITALOCEAN_TOKEN`, `SUPABASE_ACCESS_TOKEN`, `CLOUDFLARE_API_TOKEN`, `VERCEL_API_TOKEN`, and `TERRAFORM_GITHUB_TOKEN`. The workflow materializes the selected root payload with mode `0600` on the ephemeral runner and removes it after the plan or apply.

During phased activation, a root without its `TFVARS_*_JSON` payload is reported as an explicit skipped notice. Configured roots still plan independently, so pending Grafana or Cloudflare onboarding does not conceal drift in the active application infrastructure.

`api_env` must include the values documented in `.env.production.example`, including database, Clerk, pass peppers, Spaces, CORS, and OTLP configuration. Staging uses a Clerk development instance and synthetic identities only.

## Safe activation order

1. Create HCP Terraform organization/workspaces and configure local execution.
2. Create the staging Supabase project and Clerk development instance.
3. Apply staging and verify the fresh migration path.
4. Import existing production infrastructure and obtain a no-replacement plan.
5. Apply `observability`; test Discord and email contact points.
6. Inventory all GoDaddy records, populate `global/dns_records`, and set `manage_cloudflare = true` only after parity review.
7. Import Vercel and GitHub resources, then apply global settings with DNSSEC disabled.
8. Complete the DNS cutover runbook. Enable Cloudflare DNSSEC only after propagation.
9. Enable required checks after one successful run of every workflow so the exact check names exist.

Terraform controls provider resources; Go forward-only migrations remain the sole authority for the `ats` schema.
