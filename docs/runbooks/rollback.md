# Rollback runbook

The **Staging Rollback Drill** workflow exercises DigitalOcean's real rollback
API without leaving staging on an old release. It activates the most recent
superseded digest, measures activation time, restores the original digest, and
requires `/readyz` to pass. Enter `ROLLBACK STAGING` when dispatching it.

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
