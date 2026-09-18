# Event-day check-in

## Before arrivals

1. Release accepted decisions and ask attendees to confirm attendance.
2. Issue passes for confirmed attendees shortly before the event. RSVP alone does not issue a pass.
3. In **Operations → Event setup**, configure an active **Main entrance** scan point. Choose its opening window and allowed uses. Use one use for initial check-in; decide separately how returning attendees will be recognized (for example, wristbands).
4. Add volunteers through **Manage volunteers**. Volunteers must sign in with their own Scanner or Admin account.
5. Test a real ticket on two phones. Verify lookup, check-in, duplicate rejection, camera permission, and the venue internet connection.

## Attendees

The dashboard offers the next action: **Confirm attendance**, a confirmed/waiting-for-pass message, then **View event pass** once the API reports an active pass. Applicants can change their attendance, with confirmation before declining. Changing RSVP does not itself revoke a previously issued pass; organizers retain explicit pass revocation controls.

## Volunteers

1. Open **Scanner**, choose a scan point once (automatically selected if only one is active), and tap **Scan ticket**.
2. The camera captures and verifies the QR automatically. **Ready to check in** is not a recorded entry.
3. Check the attendee name and tap **Check in at …**.
4. Admit the person only after **Checked in** appears. Tap **Scan next attendee** to reopen the camera; the selected point is retained.

Manual code entry is available under **Camera not working? Enter a code**. The scanner needs internet. An attendee may present a screenshot, but a copied QR is a bearer credential; use an identity check if required by your event policy.

If a request fails after check-in was submitted, **Retry check-in** reuses the same idempotency key. Never treat a failed network request as confirmation. Revoked, invalid, exhausted, disallowed, and out-of-window passes remain backend-enforced failures.

## Organizers

**Event day** provides scanner/access links, unique attendance at a selected point, current confirmed RSVPs for that point's event cycle, recent successful scans, and attendance export. It does not sum entrance, meals and swag into a unique attendance total. Repeated uses increase scan counts, not unique people. **Refresh overview** refreshes the snapshot; it is not a live feed.

Recent activity shows up to ten matches from the API's latest hundred scans; use exports for full history. Missing new aggregate fields on an older API render as unavailable, never as zero or a substitute scan count. Deploy the additive API reporting change before relying on these totals.

Activity schedules are optional and hidden under **Event setup**. Per-person exceptions live in **Application detail → Event access**; Operations provides a name/email search linking to that record. Exceptions require loading existing access and confirming the change. Allowed uses are a total allowance, not an increment; restoring usual rules never deletes prior scans.

## Validation

- Component coverage: RSVP states, pass availability/errors, automatic camera lookup, single explicit check-in, repeated camera callbacks, idempotent retry, denied access, revoked/invalid/used passes, operations views, attendee search, and access confirmations.
- Database integration coverage in `api/migrations/event_operations_test.go`: repeated scans versus distinct attendees, current confirmed RSVP totals, and exclusion of a superseded decision's response.
- Responsive preview fixtures use synthetic data only; they do not replace the two-device staging rehearsal.
