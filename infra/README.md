# Infrastructure activation guide

Terraform describes four isolated roots:

- `environments/staging`: new DigitalOcean, Supabase, and private résumé-storage resources.
- `environments/production`: existing production resources, imported before any apply.
- `global`: Cloudflare DNS, Vercel, GitHub protection, and deployment environments.
- `observability`: Grafana Cloud dashboards, SLO alerts, and Discord/email routing.

Each root uses a separate HCP Terraform workspace in **local execution mode**. HCP stores state and locks it; GitHub Actions performs plans and applies. Create workspaces named `hackatlantic-staging`, `hackatlantic-production`, `hackatlantic-global`, and `hackatlantic-observability`.

## Non-negotiable production import gate

Never run a production apply against an empty state. First create an import-only branch and import the existing resources using their provider IDs:

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

## Workspace variables

Store provider tokens as sensitive environment variables in HCP Terraform: `DIGITALOCEAN_TOKEN`, `SUPABASE_ACCESS_TOKEN`, `CLOUDFLARE_API_TOKEN`, `GITHUB_TOKEN`, `VERCEL_API_TOKEN`, and `TF_VAR_grafana_service_account_token`. Store root input variables as sensitive Terraform variables when they contain URLs with credentials or application secrets.

`api_env` must include the values documented in `.env.production.example`, including database, Clerk, pass peppers, Spaces, CORS, and OTLP configuration. Staging uses a Clerk development instance and synthetic identities only.

## Safe activation order

1. Create HCP Terraform organization/workspaces and configure local execution.
2. Create the staging Supabase project and Clerk development instance.
3. Apply staging and verify the fresh migration path.
4. Import existing production infrastructure and obtain a no-replacement plan.
5. Apply `observability`; test Discord and email contact points.
6. Inventory all GoDaddy records and populate `global/dns_records`.
7. Import Vercel and GitHub resources, then apply global settings with DNSSEC disabled.
8. Complete the DNS cutover runbook. Enable Cloudflare DNSSEC only after propagation.
9. Enable required checks after one successful run of every workflow so the exact check names exist.

Terraform controls provider resources; Go forward-only migrations remain the sole authority for the `ats` schema.
