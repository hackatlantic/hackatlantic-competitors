# Deployment environments and secret inventory

Values below are names, never literal secrets.

| GitHub environment | Secrets | Variables |
| --- | --- | --- |
| `terraform-plan` | `HCP_TERRAFORM_TOKEN`; provider-specific read-only plan credentials when imports are activated | None |
| `staging` | `HCP_TERRAFORM_TOKEN`, `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`, development `CLERK_PUBLISHABLE_KEY` and `CLERK_SECRET_KEY`, synthetic-only `K6_QR_TOKEN`, `DISCORD_WEBHOOK_URL`, `RESEND_API_KEY` | `STAGING_API_URL`, `STAGING_WEB_URL`, `E2E_APPLICANT_EMAIL`, `E2E_ADMIN_EMAIL`, `E2E_SCANNER_EMAIL`, `K6_SCANNER_USER_ID`, `K6_CLERK_JWT_TEMPLATE`, `K6_CHECKPOINT_ID`, `ALERT_EMAIL_FROM`, `ADMIN_ALERT_EMAILS` |
| `production` | `HCP_TERRAFORM_TOKEN`, `DIGITALOCEAN_ACCESS_TOKEN`, `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID` | Production URLs |
| `backup` | `BACKUP_DATABASE_URL`, least-privilege Spaces credentials, `DISCORD_WEBHOOK_URL`, `RESEND_API_KEY`, `GRAFANA_OTLP_AUTHORIZATION` | `SPACES_ENDPOINT`, `SPACES_REGION`, `BACKUP_BUCKET`, `BACKUP_AGE_RECIPIENT`, `ALERT_EMAIL_FROM`, `ADMIN_ALERT_EMAILS`, `GRAFANA_OTLP_ENDPOINT` |
| `disaster-recovery` | backup read credentials, `BACKUP_AGE_IDENTITY`, `DISCORD_WEBHOOK_URL`, `RESEND_API_KEY`, `GRAFANA_OTLP_AUTHORIZATION` | `SPACES_ENDPOINT`, `SPACES_REGION`, `BACKUP_BUCKET`, `ALERT_EMAIL_FROM`, `ADMIN_ALERT_EMAILS`, `GRAFANA_OTLP_ENDPOINT` |

Repository-level secrets used by unprivileged pull-request checks should be avoided. Dependabot and forked pull requests never receive protected environment secrets. Pull-request Terraform plans use the `terraform-plan` environment, require approval from a designated infrastructure reviewer, prevent self-approval, and must use read-only provider credentials. Deployment credentials remain isolated in `staging` and `production`. HCP Terraform sensitive variables hold provider/runtime configuration; GitHub stores only the token needed to execute the workspace.

The current HCP automation token expires November 6, 2026. Rotate it in all three Terraform-capable environments before November 1, revoke the predecessor after successful plan verification, and never copy its value into issue, PR, or workflow logs.

DigitalOcean runtime secrets include database URLs, Clerk issuer/audience/JWKS settings, QR and claim peppers, Spaces credentials, CORS origins, and Grafana OTLP credentials. They are encrypted App Platform values and sensitive Terraform input. No secret is a Terraform output.
