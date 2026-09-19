# Grafana essentials

## Scope

One API dashboard and three production alerts. Metrics go directly from the
existing Go API to Grafana Cloud over HTTPS; no collector, VM, Kubernetes,
Cloudflare cutover, or frontend changes are required.

The dashboard shows traffic, non-5xx responses, latency, scanner latency,
in-flight requests, database pool usage, reporting builds, and heartbeat age.
Non-5xx responses are **not** a measurement of uptime or successful business
operations. Missing data never means the API is healthy.

The production alerts are:

- Sustained server errors: over 5%, with at least five errors and 20 requests
  during a five-minute window, sustained for five minutes.
- Slow scanning: redemption p95 over 1,000 ms with at least 20 redemptions in
  five minutes, sustained for five minutes. The 750 ms target remains an
  optimization goal; this warning uses the realistic profile's upper ceiling.
- Missing telemetry: no heartbeat received for ten minutes. Check the public
  readiness endpoint before declaring an outage; exporter credentials can fail
  independently of the API.

Idle periods do not trigger latency/error alerts. Query execution errors remain
visible as alert errors. Notifications go directly to the HackAtlantic contact
point; this module does not replace the stack's global notification policy.

## Connect the existing API

### Setup checkpoint — September 2, 2026

The signed-in stack is `https://calmbamboo2335.grafana.net` in
`prod-ca-east-0`. Its account page showed **Cloud Free** with a 14-day trial
allowance; no paid upgrade was selected.

Confirmed connection values (not credentials):

- Prometheus data-source **UID**: `grafanacloud-prom`. The display name is
  `grafanacloud-calmbamboo2335-prom`; do not substitute the name for the UID.
- OTLP metrics endpoint:
  `https://otlp-gateway-prod-ca-east-0.grafana.net/otlp/v1/metrics`.
- OTLP stack ID: `1816095` (distinct from the Prometheus tenant ID).
- Confirmed alert recipient: `adebowale.ca@gmail.com`. Grafana permits managed
  email contact points only for members of its organization, so a second
  administrator recipient requires an accepted Grafana invitation before it can
  be added through Terraform.

Created the stack-scoped `hackatlantic-api-metrics` access policy with only
`metrics:write`, and the `hackatlantic-terraform` service account (numeric ID
`18`) with Editor and Alerting's Write via Provisioning API roles. The ingestion
policy now has a deployment token stored only in the protected
`API_OBSERVABILITY_ENV_JSON` inputs. Never place credentials in this document.

**Provisioned:** the original `HackAtlantic/hackatlantic-ats-observability`
workspace (`ws-xTtgfRfR3kMx2VeT`, local execution) now manages four resources:
the platform folder, API dashboard, email contact point, and three-rule alert
group. The final live Terraform plan reported **No changes**. Production and
staging infrastructure workspaces were not modified.

Dashboard: [HackAtlantic API Overview](https://calmbamboo2335.grafana.net/d/hackatlantic-api/hackatlantic-api-overview).
Its eleven panels have been verified with real staging telemetry. Grafana reported **Test notification sent successfully**
for the approved email recipient on September 2, and inbox delivery was
confirmed. Temporary bootstrap credentials are not deployment
credentials and must not be installed in CI. Both bootstrap tokens were revoked
after the final no-drift check; the existing HCP GitHub Actions token was left
unchanged. The temporary saved plan was removed; remote state remains intact.

**Rollout:** PR #107 is merged and API revision
`21b1a585a75d6995a66a5261571a23eb9d6d72a2` deployed successfully. The initial
ingestion header incorrectly used the Prometheus tenant ID. It was corrected to
the OTLP instance ID above, and an empty OTLP request returned HTTP 200 before
the replacement secret was installed. Release `33687509195`, attempt 2, reuses
the already-built digest to roll out this configuration correction.

Live staging verification confirmed fresh heartbeat samples, all eleven
dashboard panels, templated routes, pool counts, scanner latency histograms,
and the exact Git SHA in `target_info`. Production verification then confirmed
the same Git SHA, a fresh heartbeat, HTTP counters, and pool gauges. All three
production alert queries returned zero (healthy). The reviewed activation plan
updated only the rule group: **0 additions, 1 in-place change, 0 deletions**.
All three alerts are enabled; the post-apply Terraform plan reported **No changes**
with `enable_alerts=true`. These are live checks, not inferred from unit tests.

Production scanner graphs can legitimately be empty until actual scanner traffic
occurs. Staging exercised and populated both scanner series; no production
attendee records were created to manufacture dashboard data.

The configuration-only rerun successfully updated and verified the production
API, but its final Vercel step returned HTTP 409 because that frontend was already
current production. The applicant portal remained HTTP 200. The promotion wrapper
recognizes only that exact pinned-CLI response as an idempotent success; other
conflicts, auth errors, additional errors, and interrupted commands still fail.

The second email recipient still requires an accepted Grafana organization
invitation. This does not prevent alerting to the primary confirmed recipient.
Automated observability plans/drift are **not activated**: their protected
`TFVARS_OBSERVABILITY_JSON` and appropriate read credentials still need setup.
Do not install short-lived bootstrap credentials into CI or describe a skipped
plan job as a successful live Terraform plan.

### Deployment steps

1. Sign into Grafana Cloud and select/create the intended stack. Confirm its
   plan and usage limits; do not start a paid upgrade for this setup.
2. In the stack's OpenTelemetry connection instructions, obtain its actual
   OTLP/HTTP endpoint and a stack-scoped token with **metrics:write only**.
   This ingestion token is separate from the Grafana configuration token.
3. Set these keys in the separate protected `API_OBSERVABILITY_ENV_JSON` map:

   - `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT`: the stack OTLP endpoint ending in
     `/v1/metrics` (for example, the displayed `/otlp` base plus `/v1/metrics`).
   - `OTEL_EXPORTER_OTLP_METRICS_HEADERS`: Grafana's generated
     `Authorization=Basic …` header, stored as a secret.
   - `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`.

   Keep `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`,
   and `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` unset/empty for metrics-only operation.
   A nonempty global endpoint enables all three signals. Do not put ingestion
   credentials in a NEXT_PUBLIC variable or in source control.
   Construct the Basic header with the **OTLP instance ID**, not the Prometheus
   tenant ID. Validate a POST of `{"resourceMetrics":[]}` with
   `Content-Type: application/json` to the metrics endpoint; require HTTP 200
   without printing the header. This tests authentication without inventing
   heartbeat or application metrics.
4. Update this dedicated secret in `staging`, `Production`, `terraform-plan`,
   and `terraform-drift`, without overwriting `TFVARS_*_JSON` or unrelated
   credentials. The roots merge it through sensitive `api_observability_env`.
   The App Platform module already marks all API environment
   values SECRET. Deploy through the existing digest-pinned release pipeline.
   Do not hand-edit DigitalOcean without updating the managed source of truth.
5. After roughly two collection intervals (30 seconds each), verify in Grafana
   Explore that `hackatlantic_telemetry_up` and the HTTP metrics appear with
   `deployment_environment_name="staging"` and the expected `service_version`.
   Confirm Grafana's resource promotion includes `service_name` and
   `deployment_environment_name`. Missing labels require correcting ingestion
   settings, not accepting an empty dashboard as success.
6. Promote the verified image/config to production through the usual approval.
   Confirm production appears separately and the reported version is correct.
   To change only configuration on an already-successful release, rerun its
   staging job and dependent production job. This retains the original image
   digest and re-executes the staging gates.

`service_version` currently reports `staging` or `production`; use
`target_info.vcs_ref_head_revision` and `/versionz` for the exact deployed Git SHA.
Grafana histogram quantiles are bucket estimates over a rolling window, so they
need not equal k6's per-request percentiles. Large admin fixture-creation batches
can also affect the overall API p95 without representing scanner latency.

The SDK exports only route templates, bounded methods/status classes, timing,
counts, build metadata, and random process identifiers. No SQL text, names,
emails, application answers, résumé metadata, request URLs, IPs, JWTs, or claim
credentials are attached to request telemetry. HTTP trace propagation accepts
TraceContext, not baggage. Claim routes do not create server spans. Logs remain
in DigitalOcean for this first rollout.

## Provision dashboard and alerts

Use `infra/observability` with the existing HCP workspace
`HackAtlantic/hackatlantic-ats-observability`. Supply the real stack URL,
Prometheus data-source UID, and confirmed administrator email addresses.
These workspaces use local execution: HCP stores state and locks, while the
runner supplies variables. HCP workspace variables are not passed to local
Terraform runs. Supply `grafana_service_account_token` in the protected
`TFVARS_OBSERVABILITY_JSON` payload used by the authorized runner, alongside
`grafana_url`, `prometheus_data_source_uid`, and `admin_alert_emails`.
Keep its permissions limited to folders, dashboards, contact points, and alert
rules. Discord is optional; its webhook belongs in that protected payload too.
Never commit the payload. Plan/drift environments require separately scoped
read credentials before activating their observability jobs.

After creating a folder, verify the provisioning service account can read and
edit its dashboards. If a first post-create read returns 403, inspect the folder
ACL and retry a read-only plan before broadening permissions. During bootstrap,
the dashboard already existed and its configuration matched exactly; clearing
its failed-creation marker retained it, and a state-only apply saved its URL.
Do not apply a replacement plan merely to recover from a transient read error.

```sh
terraform -chdir=infra/observability init -backend-config=backend.hcl.example
terraform -chdir=infra/observability plan
# Review the entire plan, then apply only the reviewed configuration.
```

Before the first plan, inspect existing state and import matching resources if
they already exist. If the advanced dashboard or synthetic resources were
previously managed, retain `enable_operations_dashboard=true` and/or
`enable_synthetics=true`; moved blocks preserve addresses, but setting a flag
false would still propose deletion. Reject unintended deletions/replacements.
The removed global notification-policy block relinquishes ownership without
deleting an existing policy.

Start with the advanced options disabled in a new workspace. Do not show backup
or restore panels as operational before they have real measurements.
External synthetic checks are a separate opt-in requiring actual probe IDs and
synthetic monitoring credentials. A telemetry heartbeat is not their substitute.

## Acceptance checks — required before calling it live

- Production heartbeat, traffic, route labels, database counts, and deployed
  version are visible in Grafana, not merely present in repository JSON.
- Visit readiness and exercise a normal staging application/scanner journey;
  route labels remain templates, never IDs or raw URLs.
- Use Grafana's contact-point **Test** action and confirm email delivery
  (and Discord if enabled). A successful save is not delivery evidence.
- After ingestion, labels, and delivery pass, set `enable_alerts=true` in the
  protected observability inputs. Review and apply a plan that only unpauses
  the three rules. Do not enable the missing-telemetry alert during bootstrap.
  Retain `enable_alerts=true` in subsequent authorized operator inputs; the
  module default deliberately remains false for safe first-time bootstrap.
- Inspect each alert query in Explore: idle/no-5xx traffic evaluates to zero;
  missing telemetry evaluates to one after ten minutes. Do not break production
  to test alerts. Use a temporary staging-scoped rule/exporter pause if approved.
- Record the stack/dashboard URL, deployed SHA, test timestamp, recipient
  confirmation, and a screenshot with no applicant data.

## Troubleshooting / rollback

**No metrics:** check endpoint suffix, metrics token scope, API exporter errors,
resource labels, and Grafana time range. Never print the authorization header.

**Server errors:** inspect DigitalOcean logs using request IDs and compare the
last release. Roll back through the existing pipeline if release-related.

**Scanner latency:** compare pool in-use/max and route timing before changing
database locks or instance sizes. k6 is the controlled benchmark, not a
production stress workload.

**Disable export:** remove the signal-specific endpoint/header from the managed
API environment and redeploy. The application works without Grafana. Alerting
must be paused deliberately during a planned exporter shutdown.

## Cost boundary

No additional DigitalOcean service is provisioned. Start on Grafana's free
allowance and review ingestion/series usage after a day and again after a week.
Metrics-only, bounded route labels, and a 30-second collection interval limit
volume; do not assume an unlimited free service or enable paid overages.
Cloudflare remains optional: leave working DNS and TLS alone unless a concrete
WAF, CDN, or DNS-management requirement justifies a separate migration.
