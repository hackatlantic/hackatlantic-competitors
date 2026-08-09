# Release runbook

## Preconditions

- All required pull-request checks are green and the PR has a CODEOWNER approval.
- Staging contains only synthetic data.
- HCP Terraform workspaces are in local-execution mode and hold required variables.
- GHCR package is visible to DigitalOcean or App Platform has read credentials.
- The second administrator is available for the production approval.

## Normal release

1. Merge the reviewed PR to `main`.
2. Record the release workflow start time.
3. Confirm the image step publishes a digest, SPDX SBOM, and valid attestation.
4. Confirm staging’s PRE_DEPLOY migration succeeds.
5. Confirm `/versionz` returns the merged SHA and both the public-smoke and 25-concurrent-scanner k6 gates pass.
6. A different administrator reviews and approves the production environment.
7. Confirm production `/versionz` reports the same image SHA used by staging.
8. Confirm the prebuilt Vercel deployment is promoted without a rebuild.
9. Check applicant sign-in, current form, admin queue, scanner checkpoint list, and synthetic status.
10. Record SHA, digest, duration, result, and dashboard links in the release summary.

Do not bypass a failed migration, attestation, privacy check, or staging SLO gate. Database migrations are forward-only and require expand/contract compatibility with the previous and new API versions.
