# Rollback runbook

The **Staging Rollback Drill** workflow has two explicitly selected modes. Enter
`ROLLBACK STAGING` when dispatching either one. Do not run a drill during an
incident, another deployment, or a benchmark.

## Deliberately unhealthy staging candidate

Select `unhealthy-candidate` on a reviewed branch permitted by the staging
environment. This mode targets only `hackatlantic-api-staging` and its exact
DigitalOcean default hostname; it has no production job or credentials.

1. Confirm staging is active and ready, with identical desired/active specs and
   no deployment already running. Save the original spec privately on the runner.
2. Change only the candidate's readiness path to a confirmed nonexistent route.
   Keep its immutable digest, environment variables, liveness probe, and size.
3. Require the candidate to fail specifically with
   `ContainerHealthChecksFailed`, recording injection-to-detection duration.
4. Always request rollback to the original deployment, with `skip_pin: true` so
   the rehearsal does not disable future deployments. Require the original
   digest, `/readyz` probe, public readiness, and version to return within five
   minutes of the rollback request.
5. Verify a **non-deploying promotion canary** is skipped. A contract test also
   checks that the real release's production job depends on staging success.

The canary does not execute or approve a real production release. The report
separates detection time from recovery time. If the previous deployment kept
serving traffic, this proves failed-candidate containment and configuration
recovery, not recovery from an applicant-facing outage. Endpoint samples cannot
prove there was no sub-sample interruption. It is not a database restore test.

The workflow uploads only `staging-fault-report.json`; never upload the private
app spec, provider response bodies, credentials, or applicant data. The private
spec is deleted at job completion. If rollback fails or takes over five minutes,
the saved spec is resubmitted, the drill remains failed, and an operator must
verify the active deployment and desired configuration before releasing again.
Do not cancel the runner while it is restoring staging.

App Platform may send a deployment-failed email, but receipt must be confirmed
separately; this drill does not claim an alert was delivered just because the
provider rejected a candidate. See [DigitalOcean's health-check behavior](https://docs.digitalocean.com/products/app-platform/how-to/manage-health-checks/)
and [rollback pinning semantics](https://docs.digitalocean.com/reference/pydo/reference/apps/create_rollback/).

## Healthy-release rollback

The default `previous-release` mode activates the most recent superseded digest,
measures activation time, restores the original digest, and requires `/readyz`
to pass. It needs a distinct prior digest and a working staging custom hostname.
It is separate from proving that an unhealthy candidate is rejected.

The release workflow automatically asks DigitalOcean to restore the prior active deployment when production API smoke tests fail. This should restore code and configuration within five minutes. It does not reverse database migrations.

## API

1. Declare the incident and stop further releases using the production concurrency group.
2. Identify the last known-good deployment ID and its immutable image digest.
3. Use DigitalOcean App Platform rollback, then poll `/readyz` and `/versionz`.
4. Verify applicant sign-in, ATS authorization, scanner lookup, and redemption.
5. Reconcile Terraform’s image digest to the known-good digest in a reviewed follow-up. Do not leave intentional drift unexplained.

## Frontend

Use Vercel instant rollback to the last known-good deployment. Verify `apply.hackatlantic.ca`, Clerk redirect URLs, and API CORS. Because frontend production uses a promoted prebuilt artifact, its deployment ID is recorded in the workflow summary.

## Database

Never run a down migration during automated rollback. If a forward migration is defective, ship a forward corrective migration that remains compatible with both API versions. Restore from backup only for confirmed data loss/corruption and follow the disaster-recovery runbook.

Record detection time, rollback start, healthy time, root cause, affected users, and corrective actions without copying applicant PII into the incident record.
