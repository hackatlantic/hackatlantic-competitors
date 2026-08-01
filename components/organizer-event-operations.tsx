"use client";

import { useAuth } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { type FormEvent, useMemo, useState } from "react";
import {
  ApiError,
  createApiClient,
  type CreateOrganizerActivityRequest,
  type CreateOrganizerCheckpointRequest,
  type OrganizerActivity,
  type OrganizerCheckpoint,
  type OrganizerEntitlement,
  type OrganizerRedemptionCount,
  type UpdateOrganizerActivityRequest,
  type UpdateOrganizerCheckpointRequest,
} from "@/lib/api";

type OrganizerEventOperationsProps = {
  initialActivities: OrganizerActivity[];
  initialCheckpoints: OrganizerCheckpoint[];
  initialCounts: OrganizerRedemptionCount[];
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

type EntitlementFormState = {
  attendeeId: string;
  checkpointId: string;
  allowed: boolean;
  maxRedemptions: string;
};

type PendingDeletion =
  | { kind: "activity"; id: string; name: string }
  | { kind: "checkpoint"; id: string; name: string }
  | { kind: "entitlement"; attendeeId: string; checkpointId: string }
  | null;

type PendingEntitlementSave = {
  attendeeId: string;
  checkpointId: string;
  allowed: boolean;
  maxRedemptions: number;
} | null;

type BusyAction =
  | "activity"
  | "checkpoint"
  | "entitlement"
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

const emptyEntitlementForm: EntitlementFormState = {
  attendeeId: "",
  checkpointId: "",
  allowed: true,
  maxRedemptions: "1",
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
    emptyActivityForm,
  );
  const [checkpointForm, setCheckpointForm] = useState<CheckpointFormState>(
    emptyCheckpointForm,
  );
  const [entitlementForm, setEntitlementForm] = useState<EntitlementFormState>(
    emptyEntitlementForm,
  );
  const [loadedEntitlement, setLoadedEntitlement] =
    useState<OrganizerEntitlement | null>(null);
  const [pendingDeletion, setPendingDeletion] = useState<PendingDeletion>(null);
  const [pendingEntitlementSave, setPendingEntitlementSave] =
    useState<PendingEntitlementSave>(null);
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [exportCheckpointId, setExportCheckpointId] = useState("");

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

  const busy = busyAction !== null;

  const clearFeedback = () => {
    setNotice("");
    setActionError("");
  };

  const resetActivityForm = () => {
    setActivityForm(emptyActivityForm);
  };

  const resetCheckpointForm = () => {
    setCheckpointForm(emptyCheckpointForm);
  };

  const updateActivityForm = (activityId: string) => {
    const activity = activities.find((item) => item.id === activityId);
    setActivityForm(activity ? activityFormFrom(activity) : emptyActivityForm);
    clearFeedback();
  };

  const updateCheckpointForm = (checkpointId: string) => {
    const checkpoint = checkpoints.find((item) => item.id === checkpointId);
    setCheckpointForm(checkpoint ? checkpointFormFrom(checkpoint) : emptyCheckpointForm);
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
    const slug = checkpointForm.slug.trim();
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

  const loadEntitlement = async () => {
    clearFeedback();
    const attendeeId = entitlementForm.attendeeId.trim();
    const checkpointId = entitlementForm.checkpointId;
    if (!attendeeId || !checkpointId) {
      setActionError("Enter an attendee ID and choose a checkpoint before loading an override.");
      return;
    }

    setBusyAction("entitlement");
    try {
      const response = await client.getOrganizerAttendeeEntitlement(
        attendeeId,
        checkpointId,
      );
      setLoadedEntitlement(response.override);
      setEntitlementForm((current) => ({
        ...current,
        allowed: response.override?.allowed ?? true,
        maxRedemptions: String(response.override?.maxRedemptions ?? 1),
      }));
      setNotice(
        response.override
          ? "Current attendee override loaded."
          : "No explicit override exists. The checkpoint default currently applies.",
      );
    } catch (error) {
      setLoadedEntitlement(null);
      setActionError(
        organizerErrorMessage(error, "Unable to load the attendee entitlement."),
      );
    } finally {
      setBusyAction(null);
    }
  };

  const requestEntitlementSave = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    clearFeedback();
    const attendeeId = entitlementForm.attendeeId.trim();
    const checkpointId = entitlementForm.checkpointId;
    const maxRedemptions = Number(entitlementForm.maxRedemptions);
    if (!attendeeId || !checkpointId) {
      setActionError("Enter an attendee ID and choose a checkpoint before saving an override.");
      return;
    }
    if (!Number.isInteger(maxRedemptions) || maxRedemptions < 0) {
      setActionError("The override redemption limit must be a whole number of zero or more.");
      return;
    }

    setPendingEntitlementSave({
      attendeeId,
      checkpointId,
      allowed: entitlementForm.allowed,
      maxRedemptions,
    });
  };

  const confirmEntitlementSave = async () => {
    if (!pendingEntitlementSave) {
      return;
    }

    clearFeedback();
    setBusyAction("entitlement");
    try {
      const entitlement = await client.updateOrganizerAttendeeEntitlement(
        pendingEntitlementSave.attendeeId,
        pendingEntitlementSave.checkpointId,
        {
          allowed: pendingEntitlementSave.allowed,
          maxRedemptions: pendingEntitlementSave.maxRedemptions,
        },
      );
      setLoadedEntitlement(entitlement);
      setPendingEntitlementSave(null);
      setNotice("Attendee entitlement override saved.");
      router.refresh();
    } catch (error) {
      setActionError(
        organizerErrorMessage(error, "Unable to save the attendee entitlement."),
      );
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
      } else {
        await client.deleteOrganizerAttendeeEntitlement(
          pendingDeletion.attendeeId,
          pendingDeletion.checkpointId,
        );
        setLoadedEntitlement(null);
        setNotice("Attendee entitlement override removed. The checkpoint default now applies.");
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
      <p className="staff-summary">
        Configure event operations from organizer-authorized data. Checkpoint defaults
        and attendee overrides are enforced by the server during redemption; they do not
        grant scanner access.
      </p>

      {pendingDeletion ? (
        <section className="operations-confirmation" aria-live="polite">
          <h2>Confirm deletion</h2>
          <p>
            {pendingDeletion.kind === "activity"
              ? `Delete activity “${pendingDeletion.name}”? Checkpoints using it must be reassigned first.`
              : pendingDeletion.kind === "checkpoint"
                ? `Delete checkpoint “${pendingDeletion.name}”? Existing redemption records remain immutable.`
                : "Remove this attendee override? The checkpoint default will apply to future redemptions."}
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

      <section className="operations-section" aria-labelledby="activity-metadata-heading">
        <div className="operations-section-heading">
          <div>
            <h2 id="activity-metadata-heading">Activity metadata</h2>
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

      <section className="operations-section" aria-labelledby="checkpoint-heading">
        <div className="operations-section-heading">
          <div>
            <h2 id="checkpoint-heading">Checkpoints</h2>
            <p className="staff-muted">
              Set an active checkpoint&apos;s redemption window and authoritative default
              entitlement. Scanner operators choose from active checkpoints only.
            </p>
          </div>
          <div className="operations-select">
            <label htmlFor="checkpoint-select">Edit checkpoint</label>
            <select
              disabled={busy}
              id="checkpoint-select"
              onChange={(event) => updateCheckpointForm(event.target.value)}
              value={checkpointForm.id ?? ""}
            >
              <option value="">Create a checkpoint</option>
              {checkpoints.map((checkpoint) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  {checkpoint.name} ({checkpoint.slug})
                </option>
              ))}
            </select>
          </div>
        </div>

        <form className="operations-form" onSubmit={submitCheckpoint}>
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
              required
              value={checkpointForm.slug}
            />
          </div>
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
            <label htmlFor="checkpoint-default-limit">Default redemption limit</label>
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
                  ? "Save checkpoint"
                  : "Create checkpoint"}
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
                Delete checkpoint
              </button>
            ) : null}
            <button
              className="button secondary"
              disabled={busy}
              onClick={resetCheckpointForm}
              type="button"
            >
              New checkpoint
            </button>
          </div>
        </form>
      </section>

      <section className="operations-section" aria-labelledby="entitlement-heading">
        <h2 id="entitlement-heading">Attendee entitlement override</h2>
        <p className="staff-muted">
          An override is event access data, not a scanner or application role. It takes
          precedence over the checkpoint default only for this attendee.
        </p>
        <form className="operations-form" onSubmit={requestEntitlementSave}>
          <div className="operations-field">
            <label htmlFor="entitlement-attendee-id">Attendee ID</label>
            <input
              disabled={busy}
              id="entitlement-attendee-id"
              onChange={(event) => {
                setLoadedEntitlement(null);
                setPendingEntitlementSave(null);
                setEntitlementForm((current) => ({ ...current, attendeeId: event.target.value }));
              }}
              required
              value={entitlementForm.attendeeId}
            />
          </div>
          <div className="operations-field">
            <label htmlFor="entitlement-checkpoint">Checkpoint</label>
            <select
              disabled={busy}
              id="entitlement-checkpoint"
              onChange={(event) => {
                setLoadedEntitlement(null);
                setPendingEntitlementSave(null);
                setEntitlementForm((current) => ({ ...current, checkpointId: event.target.value }));
              }}
              required
              value={entitlementForm.checkpointId}
            >
              <option value="">Choose a checkpoint</option>
              {checkpoints.map((checkpoint) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  {checkpoint.name} ({checkpoint.slug})
                </option>
              ))}
            </select>
          </div>
          <div className="operations-field">
            <label htmlFor="entitlement-limit">Override redemption limit</label>
            <input
              disabled={busy}
              id="entitlement-limit"
              min="0"
              onChange={(event) => {
                setPendingEntitlementSave(null);
                setEntitlementForm((current) => ({
                  ...current,
                  maxRedemptions: event.target.value,
                }));
              }}
              required
              step="1"
              type="number"
              value={entitlementForm.maxRedemptions}
            />
          </div>
          <div className="operations-checkboxes">
            <label>
              <input
                checked={entitlementForm.allowed}
                disabled={busy}
                onChange={(event) => {
                  setPendingEntitlementSave(null);
                  setEntitlementForm((current) => ({
                    ...current,
                    allowed: event.target.checked,
                  }));
                }}
                type="checkbox"
              />
              Allow this attendee
            </label>
          </div>
          <div className="operations-form-actions">
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void loadEntitlement()}
              type="button"
            >
              {busyAction === "entitlement" ? "Loading…" : "Load current override"}
            </button>
            <button className="button primary" disabled={busy} type="submit">
              {busyAction === "entitlement" ? "Saving…" : "Save override"}
            </button>
            {loadedEntitlement ? (
              <button
                className="button secondary"
                disabled={busy}
                onClick={() =>
                  setPendingDeletion({
                    kind: "entitlement",
                    attendeeId: entitlementForm.attendeeId.trim(),
                    checkpointId: entitlementForm.checkpointId,
                  })
                }
                type="button"
              >
                Remove override
              </button>
            ) : null}
          </div>
        </form>
        {pendingEntitlementSave ? (
          <section className="operations-confirmation" aria-live="polite">
            <h3>Confirm attendee override</h3>
            <p>
              {pendingEntitlementSave.allowed
                ? `Allow this attendee up to ${pendingEntitlementSave.maxRedemptions} redemption${pendingEntitlementSave.maxRedemptions === 1 ? "" : "s"} at this checkpoint?`
                : "Deny this attendee at this checkpoint? The redemption limit will not be used while access is denied."}
            </p>
            <div className="staff-actions">
              <button
                className="button primary"
                disabled={busy}
                onClick={() => void confirmEntitlementSave()}
                type="button"
              >
                {busyAction === "entitlement" ? "Saving…" : "Confirm override"}
              </button>
              <button
                className="button secondary"
                disabled={busy}
                onClick={() => setPendingEntitlementSave(null)}
                type="button"
              >
                Cancel
              </button>
            </div>
          </section>
        ) : null}
      </section>

      <section className="operations-section" aria-labelledby="redemption-monitoring-heading">
        <div className="operations-section-heading">
          <div>
            <h2 id="redemption-monitoring-heading">Redemption monitoring</h2>
            <p className="staff-muted">
              Counts are operational aggregates. Individual scan credentials, application
              answers, and review records are not displayed here.
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
                  <th scope="col">Redemptions</th>
                  <th scope="col">Last redemption</th>
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

      <section className="operations-section" aria-labelledby="exports-heading">
        <h2 id="exports-heading">CSV exports</h2>
        <p className="staff-muted">
          Download the minimum necessary attendance or reconciliation data. Exports omit
          credential hashes, application answers, reviews, and decisions.
        </p>
        <div className="operations-export-controls">
          <div className="operations-field">
            <label htmlFor="export-checkpoint">Checkpoint filter (optional)</label>
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
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void downloadCsv("reconciliation")}
              type="button"
            >
              {busyAction === "reconciliation-export"
                ? "Preparing reconciliation…"
                : "Download reconciliation CSV"}
            </button>
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
