# Deployment environments and secret inventory

Values below are names, never literal secrets.

| GitHub environment | Secrets | Variables |
| --- | --- | --- |
| `terraform-plan` | `HCP_TERRAFORM_TOKEN`, four `TFVARS_*_JSON` root payloads, `DIGITALOCEAN_TOKEN`, read-only `SPACES_ACCESS_KEY_ID` and `SPACES_SECRET_ACCESS_KEY`, and other provider credentials (read-only where available; otherwise short-lived and environment-gated) | None |
| `terraform-drift` | Same root payloads and provider credentials as `terraform-plan`; alert delivery credentials | None |
| `staging` | `HCP_TERRAFORM_TOKEN`, `TFVARS_STAGING_JSON`, `DIGITALOCEAN_TOKEN`, `SPACES_ACCESS_KEY_ID`, `SPACES_SECRET_ACCESS_KEY`, `SUPABASE_ACCESS_TOKEN`, `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`, development `CLERK_PUBLISHABLE_KEY` and `CLERK_SECRET_KEY`, synthetic-only `K6_QR_TOKEN`, `DISCORD_WEBHOOK_URL`, `RESEND_API_KEY` | `STAGING_API_URL`, `STAGING_WEB_URL`, `E2E_APPLICANT_EMAIL`, `E2E_ADMIN_EMAIL`, `E2E_SCANNER_EMAIL`, `K6_SCANNER_USER_ID`, `K6_CLERK_JWT_TEMPLATE`, `K6_CHECKPOINT_ID`, `ALERT_EMAIL_FROM`, `ADMIN_ALERT_EMAILS` |
| `production` | `HCP_TERRAFORM_TOKEN`, `TFVARS_PRODUCTION_JSON`, `DIGITALOCEAN_ACCESS_TOKEN`, `SPACES_ACCESS_KEY_ID`, `SPACES_SECRET_ACCESS_KEY`, `SUPABASE_ACCESS_TOKEN`, `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID` | Production URLs |
| `backup` | `BACKUP_DATABASE_URL`, least-privilege Spaces credentials, `DISCORD_WEBHOOK_URL`, `RESEND_API_KEY`, `GRAFANA_OTLP_AUTHORIZATION` | `SPACES_ENDPOINT`, `SPACES_REGION`, `BACKUP_BUCKET`, `BACKUP_AGE_RECIPIENT`, `ALERT_EMAIL_FROM`, `ADMIN_ALERT_EMAILS`, `GRAFANA_OTLP_ENDPOINT` |
| `disaster-recovery` | backup read credentials, `BACKUP_AGE_IDENTITY`, `DISCORD_WEBHOOK_URL`, `RESEND_API_KEY`, `GRAFANA_OTLP_AUTHORIZATION` | `SPACES_ENDPOINT`, `SPACES_REGION`, `BACKUP_BUCKET`, `ALERT_EMAIL_FROM`, `ADMIN_ALERT_EMAILS`, `GRAFANA_OTLP_ENDPOINT` |

Repository-level secrets used by unprivileged pull-request checks should be avoided. Dependabot and forked pull requests never receive protected environment secrets. Pull-request Terraform plans use the `terraform-plan` environment, require approval from either designated infrastructure owner, permit the initiating owner to approve, and must use read-only provider credentials. Production deployment approval remains a separate second-administrator gate. Deployment credentials remain isolated in `staging` and `production`.

The HCP Terraform workspaces use Local execution: HCP stores encrypted state and coordinates locking, while protected GitHub environments inject the Terraform variables and provider credentials used by GitHub Actions. The `terraform-plan` environment receives read-only credentials only; credentials capable of mutating staging or production remain in their deployment environments.

The current HCP, Vercel, Supabase, and fine-grained GitHub Terraform credentials expire November 6, 2026. Rotate them in every environment where each credential is installed before November 1, revoke the predecessors after successful plan verification, and never copy their values into issue, PR, or workflow logs.

DigitalOcean runtime secrets include database URLs, Clerk issuer/audience/JWKS settings, QR and claim peppers, Spaces credentials, CORS origins, and Grafana OTLP credentials. They are encrypted App Platform values and sensitive Terraform input. No secret is a Terraform output.
