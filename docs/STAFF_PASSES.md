# Staff passes and overnight re-entry

## Volunteer and organizer experience

1. Share `https://apply.hackatlantic.ca/volunteer` in the volunteer Discord.
2. Each volunteer signs in, enters their scheduled name and requests access.
3. An admin matches the request to the schedule and approves it.
4. The volunteer opens **My event pass**. A named Volunteer pass is issued
   automatically. Existing allowlisted admins use the same page and get an
   Organizer pass. No application, RSVP, new admin account or manual issuance.

The pass link is available from the volunteer page, scanner and admin sidebar.
Existing approved volunteers also qualify; they do not need another approval.
Issuance happens on the first authenticated visit, not as a background email job.
Passes expire on September 27, 2026 at 3 PM ADT (18:00 UTC).

## At the door

Security can check the name, prominent staff label and event dates visually.
For a live validity check, an authorized scanner selects **Verify pass / re-entry**,
scans the QR, checks the name, then selects **Scan next pass**. This can be repeated
and does not consume a check-in or meal. It also works with attendee passes.

Scanning a staff pass while a check-in/meal is selected still produces a staff
verification only. It never grants an attendee meal entitlement or records a
redemption. Regular attendee check-in/meal confirmation is unchanged.

Visual checks cannot detect revoked access, copied screenshots or impersonation.
Compare the name with the person/roster; use a live scan when uncertain. A scan
validates the credential, not the physical identity of its presenter. The scanner
requires network access; a saved ticket alone is not evidence of current access.

## Security and implementation

- `POST /v1/staff/pass` uses only the authenticated owner, ignoring caller-supplied
  identity or role. Responses are `Cache-Control: no-store`.
- Live database eligibility requires admin email allowlisting, or an approved
  volunteer request **and** the scanner role. Pending/rejected/revoked requests,
  scanner grants alone and legacy organizer roles do not qualify.
- `ats.staff_passes` stores one ID and credential hash per user/event. A unique
  constraint and an atomic issuance audit make retries and concurrent page loads
  safe. No application, RSVP, attendee, email or redemption rows are created.
- QR credentials are derived server-side, using the existing keyed mechanism;
  neither raw credentials nor hashes appear in logs or audit metadata.
- Every staff QR lookup rechecks eligibility and expiry. Removing scanner access,
  revoking approval or removing admin allowlisting invalidates scanning immediately.
  No staff QR is accepted by the attendee-only redemption ledger or claim route.
- Authenticated display names identify admins (account email is the fallback if
  their profile has no name). Volunteers use their admin-reviewed request name.

## Deployment and recovery

Apply forward-only migration `000016_staff_passes.sql` before deploying the API;
deploy the frontend after the API is healthy. This is an additive private table
with SELECT/INSERT only for `hackatlantic_app`, no Data API access, and no changes
to existing attendee passes. Test an approved volunteer and an existing admin,
then verify each synthetic staging QR twice without redemption.

Rollback to the previous API/frontend together if necessary; leave the additive
table intact. Old code neither reads nor issues staff passes. Do not delete or
edit an applied migration or rotate the attendee QR pepper to disable this feature.
Wallet integrations are not required for these web passes.
