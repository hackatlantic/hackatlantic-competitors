# Event-day check-in

## Before arrivals

1. Release accepted decisions and ask attendees to confirm attendance.
2. Issue passes for confirmed attendees shortly before the event. RSVP alone does not issue a pass.
3. On **Check-in**, click **Enable entrance check-in** if the active event has no scanning choices yet. This enables **Main entrance**, one entry per attendee, immediately; it does not release passes or change RSVPs. Retries return the same entrance without resetting its rules. Existing custom entrances and meal choices are left untouched; configure those through the admin checkpoint API when needed. Decide separately how returning attendees will be recognized (for example, wristbands). Meal choices must match the actual schedule; the UI does not create them automatically.
4. Add volunteers through **Manage volunteers**. Volunteers must sign in with their own Scanner or Admin account.
5. Test a real ticket on two phones. Verify lookup, check-in, duplicate rejection, camera permission, and the venue internet connection.

## Attendees

The dashboard offers the next action: **Confirm attendance**, a confirmed/waiting-for-pass message, then **View event pass** once the API reports an active pass. Applicants can change their attendance, with confirmation before declining. Changing RSVP does not itself revoke a previously issued pass; organizers retain explicit pass revocation controls.

## Volunteers

1. Open **Scanner**, answer **What are you scanning for?** once (automatically selected if only one choice is active), and tap **Scan ticket**.
2. The camera captures and verifies the QR automatically. **Ready to confirm** is not a recorded scan.
3. Check the attendee name and selected activity, then tap **Confirm Main entrance**, **Confirm Saturday lunch**, or the corresponding configured choice.
4. Admit the person or hand over their meal only after **Recorded** appears. Tap **Scan next attendee** to reopen the camera; the selected choice is retained. One scan records only that choice, not the entire weekend.

Manual code entry is available under **Camera not working? Enter a code**. The scanner needs internet. An attendee may present a screenshot, but a copied QR is a bearer credential; use an identity check if required by your event policy.

If a request fails after a scan was submitted, **Retry scan** reuses the same idempotency key. Never treat a failed network request as confirmation. Revoked, invalid, exhausted, disallowed, and out-of-window passes remain backend-enforced failures.

## Organizers

**Check-in** (the existing `/organizer/operations` URL) provides scanner and volunteer links, two attendance numbers, name/email attendee search, and recent check-ins. No Event day/Event setup tabs, activity metadata, export controls, or scan-rule forms are shown. Confirmed RSVPs come from the active event independently of scanning configuration and application opening dates. Only that event's scanning choices are shown. If several choices exist, **Show check-ins for** selects which to count; the page never guesses which is the entrance. Repeated uses count once per person. **Refresh** refreshes the snapshot; it is not a live feed.

Recent activity shows up to five matches from the API's latest hundred scans, not a complete attendance list. An active event without checkpoints has zero check-ins; failed or missing aggregate fields remain unavailable rather than being presented as zero. Attendance and reconciliation export endpoints remain admin-authorized in the API for exceptional reporting needs, but are not part of the everyday UI.

Per-person exceptions remain in **Application detail → Event access**, reachable through **Find an attendee**. Exceptions require loading existing access and confirming the change. Allowed uses are a total allowance, not an increment; restoring usual rules never deletes prior scans. Removing configuration and export controls from Check-in does not remove backend permissions, audit history, or scanning safeguards.

## Validation

- Component coverage: RSVP states, pass availability/errors, automatic camera lookup, single explicit check-in, repeated camera callbacks, idempotent retry, denied access, revoked/invalid/used passes, operations views, attendee search, and access confirmations.
- Database integration coverage in `api/migrations/event_operations_test.go`: repeated scans versus distinct attendees, current confirmed RSVP totals, and exclusion of a superseded decision's response.
- First-run integration coverage in `api/migrations/checkin_setup_test.go`: RSVP totals before setup, closed application dates, concurrent setup retries, audit uniqueness, existing-rule preservation, and active-event isolation. Run with the existing integration CI job; compilation alone does not execute these database checks.
- Responsive preview fixtures use synthetic data only; they do not replace the two-device staging rehearsal.
