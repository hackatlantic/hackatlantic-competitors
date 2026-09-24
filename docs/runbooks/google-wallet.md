# Google Wallet event passes

Status: implemented behind a disabled-by-default API flag. Google issuer setup,
test-device verification and publishing approval are required before rollout.
No Google Play app or database migration is needed.

## Attendee flow

1. Organizer releases acceptance; attendee confirms their current RSVP.
2. Organizer releases their pass separately.
3. Attendee opens **Event pass**, then **Add to Google Wallet**.
4. The Go API verifies the signed-in owner, active configured event, released
   acceptance, confirmed RSVP and active pass. It signs a save link locally.
5. The browser opens Google's save flow. The attendee chooses their Google
   account and saves the ticket. Only this action sends ticket data to Google.
6. Volunteers scan the same `qr_v1` credential using the existing scanner.

The ticket contains a pass identifier, QR credential and display name, not an
email, application answers, resume or claim credential. Event logistics and
branding come from the pre-created Google Event Ticket Class.

## Google account setup

Hack Atlantic issuer ID (provided by the account owner): `3388000000023208272`.
Draft production class ID: `3388000000023208272.hackatlantic_2026`.
Created and retrieved successfully through Google's API; status is `draft`, not
approved. The public-only template is retained in `google-wallet-event-class.json`.
The organizer-supplied schedule confirms check-in/networking starts at 10:00 AM
ADT on September 26; this is the event start and doors-open time. The opening
ceremony follows at 12:30 PM. Sunday's closing ceremony ends at 2:45 PM, followed
by wrap-up. The organizer explicitly confirmed the event finishes September 27
at 3:00 PM ADT (`2026-09-27T15:00:00-03:00`). These times are set on the draft class.
The dedicated service account is
`hackatlantic-wallet@potent-density-506819-v4.iam.gserviceaccount.com`.
Its downloaded JSON was locally verified to match this account; do not copy that
file into the repository. Google authentication, issuer read access and draft-class
creation were verified successfully. With the account owner's explicit approval,
disabled staging settings were encrypted into `API_WALLET_ENV_JSON` on `staging`
and `API_WALLET_STAGING_ENV_JSON` on `terraform-plan` and `terraform-drift`.
Production secrets are untouched. No real attendee objects were created, and
Wallet remains disabled. Uploading a GitHub secret is not a runtime deployment.

The existing Google Cloud project used for sign-in can be reused, but its OAuth
client ID/secret are **not** Wallet signing credentials.

1. Enable Google Wallet API in the selected Cloud project.
2. Create an issuer in the [Google Pay & Wallet Console](https://pay.google.com/business/console/).
   The account owner must review and accept Google's terms. Record the issuer ID.
3. Create a dedicated Cloud service account for Wallet. No project-wide Owner or
   Editor role is needed. Add its email to the Wallet issuer with **Developer** access.
   Granting that access and creating its private key require the account owner's approval.
4. Download its JSON key securely. Store the complete JSON in the **Go API runtime's
   encrypted secret configuration**. Never paste it into chat, source control,
   a Vercel `NEXT_PUBLIC_*` variable, browser code, a report or an issue.
5. Create an **Event Ticket Class**, e.g. `<issuer-id>.hackatlantic_2026` in the Wallet
   Console. Use Hack Atlantic's logo, event name, UNB Head Hall Atrium location,
   and the organizer-confirmed event schedule. Current web ticket check-in is
   September 26, 2026 at 10:00 ADT (`2026-09-26T10:00:00-03:00`); the confirmed
   event end is September 27 at 15:00 ADT. Follow Google's class review process.
6. Add explicit demo test accounts. First validate with synthetic staging attendees
   and a separate staging issuer class. Do not export real applicant records as fixtures.
7. Complete the issuer business profile and request publishing access. Demo mode
   restricts saving to issuer admins/developers and registered test accounts.

## Runtime configuration

Set these on the Go API, not the Next.js frontend:

| Variable | Value |
| --- | --- |
| `GOOGLE_WALLET_ENABLED` | `true` only after completing the setup/test gate |
| `GOOGLE_WALLET_ISSUER_ID` | Google's numeric issuer ID |
| `GOOGLE_WALLET_CLASS_ID` | Full, existing approved `issuer.class` ID |
| `GOOGLE_WALLET_CYCLE_SLUG` | `hackatlantic-2026` for this event |
| `GOOGLE_WALLET_SERVICE_ACCOUNT_JSON` | Encrypted runtime secret containing the JSON key |
| `APP_BASE_URL` | `https://apply.hackatlantic.ca`, or the HTTPS staging origin |

With the flag unset/false, no key is parsed and the Wallet button is hidden.
With it enabled, malformed configuration fails startup without printing secrets.
Preserve these settings in the deployment's authoritative configuration so a
later infrastructure rollout does not accidentally remove them.
For the DigitalOcean/HCP Terraform deployment, HCP uses local execution mode and
stores state/locks only. GitHub Actions supplies the authoritative inputs through
protected environment secrets. Store the five Wallet settings as a JSON string
map in `API_WALLET_ENV_JSON` on the `staging` or `Production` GitHub environment.
Release maps it to the sensitive Terraform `api_wallet_env` input, which only
permits Wallet keys and merges them without replacing existing `api_env` settings.
An absent secret defaults to `{}` and leaves existing configuration unchanged.
Keep `GOOGLE_WALLET_ENABLED=false` until the environment's enablement gate is met.

The shared `terraform-plan` and `terraform-drift` environments use matching
`API_WALLET_STAGING_ENV_JSON` and `API_WALLET_PRODUCTION_ENV_JSON` secrets so each
root sees only its own Wallet settings. Keep these copies synchronized when
rotating keys or changing enablement. Do not place keys in repository-level
secrets or modify the existing `TFVARS_*_JSON` payloads for Wallet setup.
The app-environment module publishes map entries as runtime secrets.
Never commit `.tfvars` files or pass key contents on a command line. The Docker Compose
production definition also forwards the optional settings (do not run Docker
just to configure the cloud deployment).

## API contract and security

- `GET /v1/attendee/pass` adds `googleWalletAvailable` for feature visibility.
- `POST /v1/attendee/pass/google-wallet` accepts no attendee ID or QR payload.
  It returns `{ "saveUrl": "https://pay.google.com/gp/v/save/<signed-JWT>" }`.
- Existing Clerk authentication and applicant authorization apply. The database
  read validates owner, cycle, current decision, RSVP and release; cached browser
  state is not authority. Ineligible passes return 404. Disabled/signing failures
  return 503 and do not prevent use of the web pass.
- Responses use `Cache-Control: no-store`. Never log save URLs, JWTs or QR values,
  include them in telemetry, persist them in local storage, or put them in public reports.
- JWTs use RS256, `aud=google`, `typ=savetowallet`, the configured origin, and a
  ten-minute `exp`. Do not rely on Google enforcing that expiry as a revocation
  mechanism. A copied link/QR is a bearer credential; scanner checks remain authoritative.
- Stable Wallet object IDs derive from pass IDs. Repeated saves use the same object;
  reissue creates a different one. The maximum JWT length is 1800 characters.
  Very long optional display names are omitted rather than cut off.

## Limits and release gate

After PR checks pass, a branch rehearsal can use the existing Release workflow
with `staging_only=true` from the permitted `staging` branch. This is explicitly
API-only: it skips the production-configured Vercel candidate and browser journeys,
targets only the staging API resource, retains API smoke/scanner checks, and skips
the production job entirely. It is not proof of frontend or Wallet device success.
Do not approve or trigger a normal production release as a substitute for this test.

A separate `3388000000023208272.hackatlantic_2026_test` class is approved for
synthetic device tests. It is labeled TEST and is not an admission ticket.
Class approval is not the same as issuer publishing approval. Production remains
draft/disabled. A local synthetic save proves signing and Wallet rendering only;
it does not replace authenticated staging eligibility and scan tests.

This version does not push updates to already-saved Wallet cards. Revoked/replaced
passes may still look active in Wallet, but the online scanner rejects their old
QR. Ask attendees to reopen their web pass and save the replacement after reissue.
Changing RSVP alone does not revoke an already-issued pass in the existing backend;
organizers must explicitly revoke it when appropriate. Wallet export does require
the current RSVP to remain confirmed. Saving a card is not a check-in.

The attendee can view a saved card without connectivity. **Volunteer scanning
still requires connectivity** for entitlement and duplicate-redemption checks.
Do not promise offline check-in or screenshot/copy prevention.

Before production enablement:

- Run unit, HTTP and frontend tests; run the database-backed RSVP lifecycle suite in CI.
- On a real Android device, save a synthetic pass and scan it at an authorized
  checkpoint; confirm that the decoded QR is identical to the web ticket.
- Check duplicate save, failed/retried save, denied ownership and unconfirmed RSVP.
- Revoke and reissue the synthetic pass: old Wallet QR rejected, new QR accepted.
- Verify layout, event class schedule/logo, demo test permissions and publishing approval.
- Keep the existing web QR as fallback. Set `GOOGLE_WALLET_ENABLED=false` to roll back
  new exports without changing passes, attendees, RSVP data or scanner behavior.

## References

### Isolated staging API rehearsal

The existing **Staging ATS load profiles** workflow has a `wallet-device` profile.
Run it only from the protected `staging` branch, supplying a local RSA public key
(3072 bits or stronger) in `wallet_delivery_public_key`; keep the private key local.
This is a bounded functional rehearsal, not a performance benchmark.

It checks the known staging database and TEST class, preserves the deployed image,
briefly enables Wallet, and creates one synthetic attendee plus temporary staff
identities. It exercises authenticated export eligibility, repeated saves, matching
web/Wallet QR values, first/duplicate redemption, revocation, replacement and
attendee-level redemption limits. Existing cycles and application forms are not edited.

Always-run steps remove temporary staff access and restore the disabled Wallet
configuration with the original image. If restoration fails or the job is forcibly
terminated, inspect staging immediately and restore `GOOGLE_WALLET_ENABLED=false`
before another release. The workflow shares the staging release concurrency lock.
Restoration verifies readiness, the unchanged version and a false Wallet feature
flag. A public disabled-export response of 504 is retained as an explicit warning
and a **failed 503 HTTP contract** in the evidence, not counted as a successful 503.
A returned save link, enabled flag, or unexpected status fails restoration validation.

Only an encrypted synthetic-pass handoff and sanitized results are uploaded, with
one-day artifact retention. No service-account key, database password or scanner
session is included. Synthetic ledger records remain in staging. An unused test
checkpoint and replacement pass are retained for the subsequent physical-device
test; they grant no production access. Automated API success alone does not prove
Android camera scanning. The real save-and-scan check remains a separate release gate.

### Official documentation

- [Authentication and service accounts](https://developers.google.com/wallet/tickets/events/getting-started/auth/rest)
- [Event ticket object](https://developers.google.com/wallet/reference/rest/v1/eventticketobject)
- [JWT fields](https://developers.google.com/wallet/reference/rest/v1/Jwt)
- [Web save links and size limit](https://developers.google.com/wallet/tickets/events/web)
- [Official button assets and brand rules](https://developers.google.com/wallet/tickets/events/resources/brand-guidelines)
- [Publishing approval](https://developers.google.com/wallet/tickets/events/test-and-go-live/request-publishing-access)

The checked-in English-Canada condensed button is Google's unmodified SVG from
`https://developers.google.com/static/wallet/download-assets/add-to-wallet-svg.zip`.
