"use client";

import { useAuth } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { EventDayOverview } from "@/components/event-day-overview";
import { type FormEvent, useMemo, useState } from "react";
import {
  ApiError,
  createApiClient,
  type CreateOrganizerActivityRequest,
  type CreateOrganizerCheckpointRequest,
  type OrganizerActivity,
  type OrganizerCheckpoint,
  type OrganizerApplication,
  type OrganizerRedemptionCount,
  type UpdateOrganizerActivityRequest,
  type UpdateOrganizerCheckpointRequest,
} from "@/lib/api";

type OrganizerEventOperationsProps = {
  initialActivities: OrganizerActivity[];
  initialCheckpoints: OrganizerCheckpoint[];
  initialCounts: OrganizerRedemptionCount[];
  currentCycleId?: string;
};

type ActivityFormState = {
  id: string | null;
  cycleId: string;
  slug: string;
  name: string;
  startsAt: string;
  endsAt: string;
};

type CheckpointFormState = {
  id: string | null;
  cycleId: string;
  activityId: string;
  slug: string;
  name: string;
  opensAt: string;
  closesAt: string;
  defaultAllowed: boolean;
  defaultMaxRedemptions: string;
  active: boolean;
};

type PendingDeletion =
  | { kind: "activity"; id: string; name: string }
  | { kind: "checkpoint"; id: string; name: string }
  | null;

type BusyAction =
  | "activity"
  | "checkpoint"
  | "deletion"
  | "attendance-export"
  | "reconciliation-export"
  | null;

const emptyActivityForm: ActivityFormState = {
  id: null,
  cycleId: "",
  slug: "",
  name: "",
  startsAt: "",
  endsAt: "",
};

const emptyCheckpointForm: CheckpointFormState = {
  id: null,
  cycleId: "",
  activityId: "",
  slug: "",
  name: "",
  opensAt: "",
  closesAt: "",
  defaultAllowed: true,
  defaultMaxRedemptions: "1",
  active: true,
};

function organizerErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    if (error.status === 401) {
      return "Your session has ended. Sign in again before managing event operations.";
    }

    if (error.status === 403) {
      return "Organizer access is required to manage event operations.";
    }

    if (error.status >= 500) {
      return "The event operations service is temporarily unavailable. Try again.";
    }
  }

  return error instanceof Error ? error.message : fallback;
}

function dateTimeLocalValue(value?: string | null): string {
  if (!value) {
    return "";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return localDate.toISOString().slice(0, 16);
}

function optionalTimestamp(value: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) {
    return undefined;
  }

  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) {
    throw new Error("Enter a valid date and time.");
  }

  return date.toISOString();
}

function displayTimestamp(value?: string | null): string {
  if (!value) {
    return "No redemptions recorded";
  }

  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(timestamp);
}

function activityFormFrom(activity: OrganizerActivity): ActivityFormState {
  return {
    id: activity.id,
    cycleId: activity.cycleId,
    slug: activity.slug,
    name: activity.name,
    startsAt: dateTimeLocalValue(activity.startsAt),
    endsAt: dateTimeLocalValue(activity.endsAt),
  };
}

function checkpointFormFrom(checkpoint: OrganizerCheckpoint): CheckpointFormState {
  return {
    id: checkpoint.id,
    cycleId: checkpoint.cycleId,
    activityId: checkpoint.activityId ?? "",
    slug: checkpoint.slug,
    name: checkpoint.name,
    opensAt: dateTimeLocalValue(checkpoint.opensAt),
    closesAt: dateTimeLocalValue(checkpoint.closesAt),
    defaultAllowed: checkpoint.defaultAllowed,
    defaultMaxRedemptions: String(checkpoint.defaultMaxRedemptions),
    active: checkpoint.active,
  };
}

function replacementById<T extends { id: string }>(items: T[], next: T): T[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) {
    return [...items, next];
  }

  return items.map((item) => (item.id === next.id ? next : item));
}

export function OrganizerEventOperations({
  initialActivities,
  initialCheckpoints,
  initialCounts,
  currentCycleId,
}: OrganizerEventOperationsProps) {
  const { getToken } = useAuth();
  const router = useRouter();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [activities, setActivities] = useState(initialActivities);
  const [checkpoints, setCheckpoints] = useState(initialCheckpoints);
  const [counts, setCounts] = useState(initialCounts);
  const [previousInitialActivities, setPreviousInitialActivities] =
    useState(initialActivities);
  const [previousInitialCheckpoints, setPreviousInitialCheckpoints] =
    useState(initialCheckpoints);
  const [previousInitialCounts, setPreviousInitialCounts] = useState(initialCounts);
  const [activityForm, setActivityForm] = useState<ActivityFormState>(
    { ...emptyActivityForm, cycleId: currentCycleId ?? "" },
  );
  const [checkpointForm, setCheckpointForm] = useState<CheckpointFormState>(
    { ...emptyCheckpointForm, cycleId: currentCycleId ?? "" },
  );
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [pendingDeletion, setPendingDeletion] = useState<PendingDeletion>(null);
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [exportCheckpointId, setExportCheckpointId] = useState("");
  const [view, setView] = useState<"event" | "setup">("event");
  const [attendeeQuery, setAttendeeQuery] = useState("");
  const [attendeeMatches, setAttendeeMatches] = useState<OrganizerApplication[]>([]);
  const [attendeeSearchBusy, setAttendeeSearchBusy] = useState(false);

  const [attendeeSearchMessage, setAttendeeSearchMessage] = useState("");

  if (
    initialActivities !== previousInitialActivities ||
    initialCheckpoints !== previousInitialCheckpoints ||
    initialCounts !== previousInitialCounts
  ) {
    setPreviousInitialActivities(initialActivities);
    setPreviousInitialCheckpoints(initialCheckpoints);
    setPreviousInitialCounts(initialCounts);
    setActivities(initialActivities);
    setCheckpoints(initialCheckpoints);
    setCounts(initialCounts);
  }

  const busy = busyAction !== null || attendeeSearchBusy;

  async function searchAttendees() {
    if (!attendeeQuery.trim() || busy) return;
    setAttendeeSearchBusy(true);
    setAttendeeMatches([]);
    setAttendeeSearchMessage("");
    try {
      const result = await client.listOrganizerApplications({ status: "accepted", q: attendeeQuery.trim() });
      setAttendeeMatches(result.items);
      setAttendeeSearchMessage(result.items.length ? "Choose an attendee. Narrow your search if they are not listed." : "No accepted attendees found. Try their email address.");
    } catch (error) {
      setAttendeeSearchMessage(organizerErrorMessage(error, "Couldn’t search attendees. Try again."));
    } finally { setAttendeeSearchBusy(false); }
  }

  const clearFeedback = () => {
    setNotice("");
    setActionError("");
  };

  const resetActivityForm = () => {
    setActivityForm({ ...emptyActivityForm, cycleId: currentCycleId ?? "" });
  };

  const resetCheckpointForm = () => {
    setCheckpointForm({ ...emptyCheckpointForm, cycleId: currentCycleId ?? "" });
  };

  const updateActivityForm = (activityId: string) => {
    const activity = activities.find((item) => item.id === activityId);
    setActivityForm(activity ? activityFormFrom(activity) : { ...emptyActivityForm, cycleId: currentCycleId ?? "" });
    clearFeedback();
  };

  const updateCheckpointForm = (checkpointId: string) => {
    const checkpoint = checkpoints.find((item) => item.id === checkpointId);
    setCheckpointForm(checkpoint ? checkpointFormFrom(checkpoint) : { ...emptyCheckpointForm, cycleId: currentCycleId ?? "" });
    clearFeedback();
  };

  const submitActivity = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    clearFeedback();

    const cycleId = activityForm.cycleId.trim();
    const slug = activityForm.slug.trim();
    const name = activityForm.name.trim();
    if (!activityForm.id && !cycleId) {
      setActionError("Provide the cycle ID before creating an activity.");
      return;
    }
    if (!slug || !name) {
      setActionError("Provide both an activity slug and name.");
      return;
    }

    setBusyAction("activity");
    try {
      const startsAt = optionalTimestamp(activityForm.startsAt);
      const endsAt = optionalTimestamp(activityForm.endsAt);
      if (startsAt && endsAt && startsAt > endsAt) {
        setActionError("An activity cannot end before it starts.");
        return;
      }

      const savedActivity = activityForm.id
        ? await client.updateOrganizerActivity(activityForm.id, {
            slug,
            name,
            startsAt: startsAt ?? null,
            endsAt: endsAt ?? null,
          } satisfies UpdateOrganizerActivityRequest)
        : await client.createOrganizerActivity({
            cycleId,
            slug,
            name,
            ...(startsAt ? { startsAt } : {}),
            ...(endsAt ? { endsAt } : {}),
          } satisfies CreateOrganizerActivityRequest);

      setActivities((current) => replacementById(current, savedActivity));
      setActivityForm(activityFormFrom(savedActivity));
      setNotice(
        activityForm.id
          ? "Activity metadata updated."
          : "Activity metadata created.",
      );
      router.refresh();
    } catch (error) {
      setActionError(organizerErrorMessage(error, "Unable to save the activity."));
    } finally {
      setBusyAction(null);
    }
  };

  const submitCheckpoint = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    clearFeedback();

    const cycleId = checkpointForm.cycleId.trim();
    const slug = checkpointForm.slug.trim() || checkpointForm.name.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    const name = checkpointForm.name.trim();
    const defaultMaxRedemptions = Number(checkpointForm.defaultMaxRedemptions);
    if (!checkpointForm.id && !cycleId) {
      setActionError("Provide the cycle ID before creating a checkpoint.");
      return;
    }
    if (!slug || !name) {
      setActionError("Provide both a checkpoint slug and name.");
      return;
    }
    if (!Number.isInteger(defaultMaxRedemptions) || defaultMaxRedemptions < 0) {
      setActionError("The default redemption limit must be a whole number of zero or more.");
      return;
    }

    setBusyAction("checkpoint");
    try {
      const opensAt = optionalTimestamp(checkpointForm.opensAt);
      const closesAt = optionalTimestamp(checkpointForm.closesAt);
      if (opensAt && closesAt && opensAt > closesAt) {
        setActionError("A checkpoint cannot close before it opens.");
        return;
      }

      const savedCheckpoint = checkpointForm.id
        ? await client.updateOrganizerCheckpoint(checkpointForm.id, {
            activityId: checkpointForm.activityId || null,
            slug,
            name,
            opensAt: opensAt ?? null,
            closesAt: closesAt ?? null,
            defaultAllowed: checkpointForm.defaultAllowed,
            defaultMaxRedemptions,
            active: checkpointForm.active,
          } satisfies UpdateOrganizerCheckpointRequest)
        : await client.createOrganizerCheckpoint({
            cycleId,
            activityId: checkpointForm.activityId || null,
            slug,
            name,
            ...(opensAt ? { opensAt } : {}),
            ...(closesAt ? { closesAt } : {}),
            defaultAllowed: checkpointForm.defaultAllowed,
            defaultMaxRedemptions,
            active: checkpointForm.active,
          } satisfies CreateOrganizerCheckpointRequest);

      setCheckpoints((current) => replacementById(current, savedCheckpoint));
      setCheckpointForm(checkpointFormFrom(savedCheckpoint));
      setNotice(
        checkpointForm.id
          ? "Checkpoint updated."
          : "Checkpoint created and ready for organizer-managed operations.",
      );
      router.refresh();
    } catch (error) {
      setActionError(organizerErrorMessage(error, "Unable to save the checkpoint."));
    } finally {
      setBusyAction(null);
    }
  };

  const confirmDeletion = async () => {
    if (!pendingDeletion) {
      return;
    }

    clearFeedback();
    setBusyAction("deletion");
    try {
      if (pendingDeletion.kind === "activity") {
        await client.deleteOrganizerActivity(pendingDeletion.id);
        setActivities((current) =>
          current.filter((activity) => activity.id !== pendingDeletion.id),
        );
        if (activityForm.id === pendingDeletion.id) {
          resetActivityForm();
        }
        setNotice("Activity deleted.");
      } else if (pendingDeletion.kind === "checkpoint") {
        await client.deleteOrganizerCheckpoint(pendingDeletion.id);
        setCheckpoints((current) =>
          current.filter((checkpoint) => checkpoint.id !== pendingDeletion.id),
        );
        setCounts((current) =>
          current.filter((count) => count.checkpointId !== pendingDeletion.id),
        );
        if (checkpointForm.id === pendingDeletion.id) {
          resetCheckpointForm();
        }
        setNotice("Checkpoint deleted.");
      }
      setPendingDeletion(null);
      router.refresh();
    } catch (error) {
      setActionError(organizerErrorMessage(error, "Unable to complete this deletion."));
    } finally {
      setBusyAction(null);
    }
  };

  const downloadCsv = async (kind: "attendance" | "reconciliation") => {
    clearFeedback();
    setBusyAction(kind === "attendance" ? "attendance-export" : "reconciliation-export");
    try {
      const filters = exportCheckpointId ? { checkpointId: exportCheckpointId } : {};
      const download =
        kind === "attendance"
          ? await client.downloadOrganizerAttendanceCsv(filters)
          : await client.downloadOrganizerReconciliationCsv(filters);
      const objectUrl = URL.createObjectURL(download.blob);
      const anchor = document.createElement("a");
      anchor.href = objectUrl;
      anchor.download =
        download.filename ??
        (kind === "attendance" ? "attendance.csv" : "reconciliation.csv");
      anchor.hidden = true;
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(objectUrl);
      setNotice(
        kind === "attendance"
          ? "Attendance CSV download started."
          : "Reconciliation CSV download started.",
      );
    } catch (error) {
      setActionError(organizerErrorMessage(error, "Unable to download the CSV export."));
    } finally {
      setBusyAction(null);
    }
  };

  return (
    <div className="organizer-operations">
      <div className="event-day-navigation" aria-label="Operations views">
        <button type="button" aria-pressed={view === "event"} onClick={() => setView("event")}>Event day</button>
        <button type="button" aria-pressed={view === "setup"} onClick={() => setView("setup")}>Event setup</button>
      </div>
      {view === "event" ? <EventDayOverview checkpoints={checkpoints} counts={counts} onSetup={() => setView("setup")} onRefresh={() => router.refresh()} /> : null}

      {pendingDeletion ? (
        <section className="operations-confirmation" aria-live="polite">
          <h2>Confirm deletion</h2>
          <p>
            {pendingDeletion.kind === "activity"
              ? `Delete activity “${pendingDeletion.name}”? Checkpoints using it must be reassigned first.`
              : `Delete checkpoint “${pendingDeletion.name}”? Existing redemption records remain immutable.`}
          </p>
          <div className="staff-actions">
            <button
              className="button primary"
              disabled={busy}
              onClick={() => void confirmDeletion()}
              type="button"
            >
              {busyAction === "deletion" ? "Deleting…" : "Confirm deletion"}
            </button>
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => setPendingDeletion(null)}
              type="button"
            >
              Cancel
            </button>
          </div>
        </section>
      ) : null}

      <div hidden={view !== "setup"} className="event-setup">
      <p className="staff-summary">Start with one entrance. Add meal or swag scan points only if you need them. Changes here affect future scans; recorded check-ins stay intact.</p>
      <details className="operations-advanced">
      <summary>Optional activity schedule</summary>
      <section className="operations-section" aria-labelledby="activity-metadata-heading">
        <div className="operations-section-heading">
          <div>
            <h2 id="activity-metadata-heading">Activity schedule</h2>
            <p className="staff-muted">
              Activities are optional schedule metadata. They do not create scanner access
              or redemption rules on their own.
            </p>
          </div>
          <div className="operations-select">
            <label htmlFor="activity-select">Edit activity</label>
            <select
              disabled={busy}
              id="activity-select"
              onChange={(event) => updateActivityForm(event.target.value)}
              value={activityForm.id ?? ""}
            >
              <option value="">Create an activity</option>
              {activities.map((activity) => (
                <option key={activity.id} value={activity.id}>
                  {activity.name} ({activity.slug})
                </option>
              ))}
            </select>
          </div>
        </div>

        <form className="operations-form" onSubmit={submitActivity}>
          <div className="operations-field">
            <label htmlFor="activity-cycle-id">Cycle ID</label>
            <input
              disabled={busy || activityForm.id !== null}
              id="activity-cycle-id"
              onChange={(event) =>
                setActivityForm((current) => ({ ...current, cycleId: event.target.value }))
              }
              required
              value={activityForm.cycleId}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="activity-slug">Slug</label>
            <input
              disabled={busy}
              id="activity-slug"
              onChange={(event) =>
                setActivityForm((current) => ({ ...current, slug: event.target.value }))
              }
              required
              value={activityForm.slug}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="activity-name">Name</label>
            <input
              disabled={busy}
              id="activity-name"
              onChange={(event) =>
                setActivityForm((current) => ({ ...current, name: event.target.value }))
              }
              required
              value={activityForm.name}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="activity-starts-at">Starts at (optional)</label>
            <input
              disabled={busy}
              id="activity-starts-at"
              onChange={(event) =>
                setActivityForm((current) => ({ ...current, startsAt: event.target.value }))
              }
              type="datetime-local"
              value={activityForm.startsAt}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="activity-ends-at">Ends at (optional)</label>
            <input
              disabled={busy}
              id="activity-ends-at"
              onChange={(event) =>
                setActivityForm((current) => ({ ...current, endsAt: event.target.value }))
              }
              type="datetime-local"
              value={activityForm.endsAt}
            />
          </div>
          <div className="operations-form-actions">
            <button className="button primary" disabled={busy} type="submit">
              {busyAction === "activity"
                ? "Saving…"
                : activityForm.id
                  ? "Save activity"
                  : "Create activity"}
            </button>
            {activityForm.id ? (
              <button
                className="button secondary"
                disabled={busy}
                onClick={() =>
                  setPendingDeletion({
                    kind: "activity",
                    id: activityForm.id as string,
                    name: activityForm.name,
                  })
                }
                type="button"
              >
                Delete activity
              </button>
            ) : null}
            <button
              className="button secondary"
              disabled={busy}
              onClick={resetActivityForm}
              type="button"
            >
              New activity
            </button>
          </div>
        </form>
      </section>
      </details>

      <section className="operations-section" aria-labelledby="checkpoint-heading">
        <div className="operations-section-heading">
          <div>
            <h2 id="checkpoint-heading">Entrance & scan points</h2>
            <p className="staff-muted">
              Choose when scanning opens and how many times each attendee can use this point. Only active points appear in the scanner.
            </p>
          </div>
          <div className="operations-select">
            <label htmlFor="checkpoint-select">Edit scan point</label>
            <select
              disabled={busy}
              id="checkpoint-select"
              onChange={(event) => updateCheckpointForm(event.target.value)}
              value={checkpointForm.id ?? ""}
            >
              <option value="">Create a scan point</option>
              {checkpoints.map((checkpoint) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  {checkpoint.name} ({checkpoint.slug})
                </option>
              ))}
            </select>
          </div>
        </div>

        <form className="operations-form" onSubmit={submitCheckpoint}>
          <details className="operations-advanced" open={!checkpointForm.cycleId}>
          <summary>Advanced identifiers & activity link</summary>
          <div className="operations-field">
            <label htmlFor="checkpoint-cycle-id">Cycle ID</label>
            <input
              disabled={busy || checkpointForm.id !== null}
              id="checkpoint-cycle-id"
              onChange={(event) =>
                setCheckpointForm((current) => ({ ...current, cycleId: event.target.value }))
              }
              required
              value={checkpointForm.cycleId}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="checkpoint-activity">Activity (optional)</label>
            <select
              disabled={busy}
              id="checkpoint-activity"
              onChange={(event) =>
                setCheckpointForm((current) => ({ ...current, activityId: event.target.value }))
              }
              value={checkpointForm.activityId}
            >
              <option value="">No linked activity</option>
              {activities.map((activity) => (
                <option key={activity.id} value={activity.id}>
                  {activity.name} ({activity.slug})
                </option>
              ))}
            </select>
          </div>
          <div className="operations-field">
            <label htmlFor="checkpoint-slug">Slug</label>
            <input
              disabled={busy}
              id="checkpoint-slug"
              onChange={(event) =>
                setCheckpointForm((current) => ({ ...current, slug: event.target.value }))
              }
              placeholder="Generated from the name for new scan points"
              value={checkpointForm.slug}
            />
          </div>
          </details>
          <div className="operations-field">
            <label htmlFor="checkpoint-name">Name</label>
            <input
              disabled={busy}
              id="checkpoint-name"
              onChange={(event) =>
                setCheckpointForm((current) => ({ ...current, name: event.target.value }))
              }
              required
              value={checkpointForm.name}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="checkpoint-opens-at">Opens at (optional)</label>
            <input
              disabled={busy}
              id="checkpoint-opens-at"
              onChange={(event) =>
                setCheckpointForm((current) => ({ ...current, opensAt: event.target.value }))
              }
              type="datetime-local"
              value={checkpointForm.opensAt}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="checkpoint-closes-at">Closes at (optional)</label>
            <input
              disabled={busy}
              id="checkpoint-closes-at"
              onChange={(event) =>
                setCheckpointForm((current) => ({ ...current, closesAt: event.target.value }))
              }
              type="datetime-local"
              value={checkpointForm.closesAt}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="checkpoint-default-limit">Allowed uses per attendee</label>
            <input
              disabled={busy}
              id="checkpoint-default-limit"
              min="0"
              onChange={(event) =>
                setCheckpointForm((current) => ({
                  ...current,
                  defaultMaxRedemptions: event.target.value,
                }))
              }
              required
              step="1"
              type="number"
              value={checkpointForm.defaultMaxRedemptions}
            />
            <p className="staff-muted">Use 1 for first arrival. Returning attendees will need a wristband or a separate re-entry policy.</p>
          </div>
          <div className="operations-checkboxes">
            <label>
              <input
                checked={checkpointForm.defaultAllowed}
                disabled={busy}
                onChange={(event) =>
                  setCheckpointForm((current) => ({
                    ...current,
                    defaultAllowed: event.target.checked,
                  }))
                }
                type="checkbox"
              />
              Allow by default
            </label>
            <label>
              <input
                checked={checkpointForm.active}
                disabled={busy}
                onChange={(event) =>
                  setCheckpointForm((current) => ({ ...current, active: event.target.checked }))
                }
                type="checkbox"
              />
              Active for scanners
            </label>
          </div>
          <div className="operations-form-actions">
            <button className="button primary" disabled={busy} type="submit">
              {busyAction === "checkpoint"
                ? "Saving…"
                : checkpointForm.id
                  ? "Save scan point"
                  : "Create scan point"}
            </button>
            {checkpointForm.id ? (
              <button
                className="button secondary"
                disabled={busy}
                onClick={() =>
                  setPendingDeletion({
                    kind: "checkpoint",
                    id: checkpointForm.id as string,
                    name: checkpointForm.name,
                  })
                }
                type="button"
              >
                Delete scan point
              </button>
            ) : null}
            <button
              className="button secondary"
              disabled={busy}
              onClick={resetCheckpointForm}
              type="button"
            >
              New scan point
            </button>
          </div>
        </form>
      </section>

      <details className="operations-advanced">
        <summary>Find an attendee’s access settings</summary>
        <p>Search by name or email, then manage exceptions inside their attendee record.</p>
        <div className="operations-field">
          <label htmlFor="access-attendee-search">Name or email</label>
          <input id="access-attendee-search" value={attendeeQuery} disabled={busy} onChange={(event) => { setAttendeeQuery(event.target.value); setAttendeeMatches([]); setAttendeeSearchMessage(""); }} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void searchAttendees(); } }} />
          <button type="button" className="button secondary" disabled={busy || !attendeeQuery.trim()} onClick={() => void searchAttendees()}>{attendeeSearchBusy ? "Searching…" : "Find attendee"}</button>
          {attendeeSearchMessage ? <p role="status">{attendeeSearchMessage}</p> : null}
          {attendeeMatches.map((application) => <Link className="attendee-search-result" key={application.id} href={`/organizer/applications/${application.id}#access-settings`}>{application.applicant.displayName || "Attendee"} · {application.applicant.email}</Link>)}
        </div>
      </details>
      </div>

      <details hidden={view !== "event"} className="operations-advanced">
      <summary>Scan point totals</summary>
      <section className="operations-section" aria-labelledby="redemption-monitoring-heading">
        <div className="operations-section-heading">
          <div>
            <h2 id="redemption-monitoring-heading">Check-in activity</h2>
            <p className="staff-muted">
              Successful scans at each point, including repeat uses where allowed. These are not unique attendance totals.
            </p>
          </div>
          <button
            className="button secondary"
            disabled={busy}
            onClick={() => router.refresh()}
            type="button"
          >
            Refresh counts
          </button>
        </div>
        {counts.length === 0 ? (
          <p className="staff-muted">No checkpoint redemptions have been recorded.</p>
        ) : (
          <div className="operations-table-wrap">
            <table className="operations-table">
              <thead>
                <tr>
                  <th scope="col">Checkpoint</th>
                  <th scope="col">Successful scans</th>
                  <th scope="col">Last scan</th>
                </tr>
              </thead>
              <tbody>
                {counts.map((count) => (
                  <tr key={count.checkpointId}>
                    <th scope="row">{count.checkpointName}</th>
                    <td>{count.totalRedemptions}</td>
                    <td>{displayTimestamp(count.lastRedeemedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      </details>
      <section hidden={view !== "event"} className="operations-section" aria-labelledby="exports-heading">
        <h2 id="exports-heading">Export attendance</h2>
        <p className="staff-muted">
          Download the check-in record for your team. Application answers and QR credentials are never included.
        </p>
        <div className="operations-export-controls">
          <div className="operations-field">
            <label htmlFor="export-checkpoint">Scan point (optional)</label>
            <select
              disabled={busy}
              id="export-checkpoint"
              onChange={(event) => setExportCheckpointId(event.target.value)}
              value={exportCheckpointId}
            >
              <option value="">All checkpoints</option>
              {checkpoints.map((checkpoint) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  {checkpoint.name} ({checkpoint.slug})
                </option>
              ))}
            </select>
          </div>
          <div className="operations-form-actions">
            <button
              className="button primary"
              disabled={busy}
              onClick={() => void downloadCsv("attendance")}
              type="button"
            >
              {busyAction === "attendance-export"
                ? "Preparing attendance…"
                : "Download attendance CSV"}
            </button>
            <details className="operations-advanced"><summary>Advanced export</summary>
              <p>Download scan IDs and staff IDs for reconciliation. Personal application data is excluded.</p>
              <button className="button secondary" disabled={busy} onClick={() => void downloadCsv("reconciliation")} type="button">{busyAction === "reconciliation-export" ? "Preparing reconciliation…" : "Download reconciliation CSV"}</button>
            </details>
          </div>
        </div>
      </section>

      {notice ? (
        <p className="application-notice" role="status">
          {notice}
        </p>
      ) : null}
      {actionError ? (
        <p className="error-message" role="alert">
          {actionError}
        </p>
      ) : null}
    </div>
  );
}
