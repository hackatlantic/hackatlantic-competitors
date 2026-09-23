# Contributing to HackAtlantic ATS

Thanks for contributing. Small, reviewed changes are safer here because this repository handles applicant data and event credentials.

## Before you start

1. Check open pull requests and issues so work is not duplicated.
2. Branch from the current `main` using a descriptive prefix, such as `feature/`, `fix/`, `docs/`, or `codex/` for assisted work.
3. Keep each pull request focused. Do not mix a visual redesign with migrations, Terraform, or dependency upgrades.
4. Never commit `.env` files, Terraform state, provider tokens, Clerk keys, résumé files, real applicant data, QR credentials, or claim links.

## Local workflow

Follow the setup in [README.md](README.md). Before opening a pull request, run the checks that match your change:

```bash
npm run lint
npm run build
npm run test:components
npm run api:test
```

Use `npm run api:test:integration` when changing database access, migrations, roles, lifecycle behavior, or redemption. Run `npm run test:e2e` when changing a critical browser journey. Use a staging-only workflow for k6; never load-test production.

## Change-specific rules

| Change | Required practice |
| --- | --- |
| API endpoint | Update `openapi/openapi.yaml`, add tests, preserve error conventions |
| Migration | Add one forward-only file; no destructive rewrite of prior migrations |
| Authorization | Prove the permitted and denied paths in tests |
| Frontend form | Keep server validation authoritative and make errors accessible |
| Terraform or workflow | Review the complete plan/diff; never paste provider secrets in code or logs |
| Dashboard or alert | Use route templates and bounded labels; exclude PII and bearer material |
| Documentation | Update the relevant runbook or architecture source in the same PR |

## Pull requests

Use a clear title that explains the outcome. In the description, include what changed and why, verification run, operational risk, rollback notes for production-impacting changes, and redacted screenshots only when useful.

CODEOWNERS review is required for workflows, Terraform, policy, migrations, and backup/restore scripts. The protected production release is a separate decision; an approved code PR is not permission to deploy production manually.

## Reporting a bug or security concern

Use the team’s private channel for security issues. Do not open a public issue containing personal data, session tokens, claim URLs, access credentials, or a reproduction that could expose applicant records. See [docs/SECURITY.md](docs/SECURITY.md).
