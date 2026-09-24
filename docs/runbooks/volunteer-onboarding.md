# Volunteer scanner onboarding

## Share in the private volunteer Discord

After the migration, API and web releases are deployed, share:

> Volunteers: open https://apply.hackatlantic.ca/volunteer, sign in or create an account, and enter your name as it appears on the schedule. You do not need to apply to the hackathon. An admin will approve scanner access. After approval, return to the same page, select **Check approval**, then **Open scanner**.

The URL is a request page, not a bearer invitation or a credential. Forwarding it cannot grant a role. Account authentication remains required even though intake is closed.

## Admin approval

1. Open **Admin → Access → Volunteer requests** and refresh.
2. Match the claimed real name to your private volunteer schedule. Do not publish the schedule in the app or repository.
3. Confirm the signed-in account belongs to that volunteer, using a Discord reply/DM if necessary. A real name is self-reported, not identity proof. Duplicate names remain separate requests; the account email helps disambiguate without collecting emails beforehand.
4. Check the account-verification box and click **Approve scanner access**. Never approve a list automatically from matching names alone.
5. Decline unfamiliar requests. Use **Revoke scanner access** for a mistaken approval or after the event.

The existing email-based grant/revoke flow remains under **Manage access by email**. Requests are one per account and immutable; repeated submissions cannot reset a rejected or revoked request. To reconsider one, verify the person again and use the existing email-based access controls. Queue decisions are one-way (pending → approved/rejected; approved → revoked), so an old approval cannot be replayed after revocation. Access does not expire automatically; revocation is a separate action.

## Safety and deployment

- Apply migration `000015_volunteer_requests.sql` before deploying the API and web changes. Keep the feature unpublished until all three are ready.
- Only authenticated users can request or read their own status. Only admins can list/review requests; legacy organizer and scanner roles cannot approve.
- Approval writes the decision, scanner-only role, and audit records in one transaction. Stale decisions return 409; refresh before retrying. No application, acceptance, RSVP, or attendee pass is created.
- The queue is paginated (50 per page). The public request page is noindex. Request data is private to the API runtime and not readable through Supabase public roles.
- No maximum of ten volunteers is assumed. A forwarded link may generate unapproved requests, but cannot grant access. One request per authenticated account bounds repeat submissions; this is not a general anti-bot system.
- If rollback is needed, roll back API/web and leave the additive table. Existing scanner authorizations still use the previous grant/revoke UI. Revoke any unwanted grants separately.

## Before Saturday

Use a staging-only volunteer account and TEST pass. Test request → approve → sign in/refresh → camera permission → choose check-in → scan → verify name → confirm → scan again (already recorded) → choose a meal → confirm that meal once → revoke scanner access → verify subsequent scans are forbidden.

Do not scan a production attendee pass for a rehearsal. Android/iPhone browser-camera testing is still required; automated components mock the camera and do not establish physical QR readability. The ordinary web pass does not depend on Wallet approval.
