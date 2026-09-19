"use client";

import Link from "next/link";
import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ApiError, createApiClient, type OrganizerAttendanceSummary, type OrganizerCheckpoint, type OrganizerRedemption, type OrganizerRedemptionCount } from "@/lib/api";

export function CheckInOverview({ checkpoints: initialCheckpoints, counts, onRefresh, children }: {
  checkpoints: OrganizerCheckpoint[];
  counts: OrganizerRedemptionCount[];
  onRefresh: () => void;
  children: ReactNode;
}) {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [summary, setSummary] = useState<OrganizerAttendanceSummary | null>(null);
  const [summaryError, setSummaryError] = useState("");
  const [added, setAdded] = useState<OrganizerCheckpoint | null>(null);
  const [enabling, setEnabling] = useState(false);
  const [setupError, setSetupError] = useState("");
  const setupBusy = useRef(false);
  const checkpoints = summary ? [...initialCheckpoints, ...(added && !initialCheckpoints.some((point) => point.id === added.id) ? [added] : [])].filter((point) => point.cycleId === summary.cycleId) : [];
  const [selection, setSelection] = useState("");
  // Never guess which configured activity represents entrance attendance.
  const selected = checkpoints.find((point) => point.id === selection) ?? (checkpoints.length === 1 ? checkpoints[0] : null);
  const count = counts.find((item) => item.checkpointId === selected?.id);
  const [recent, setRecent] = useState<OrganizerRedemption[] | null>(null);
  const [error, setError] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void client.getOrganizerAttendanceSummary().then((data) => {
      if (!cancelled) { setSummary(data); setSummaryError(""); }
    }).catch((error: unknown) => {
      if (!cancelled) {
        setSummary(null);
        setSummaryError(error instanceof ApiError && error.status === 404
          ? "No active event was found. Check the event configuration before enabling check-in."
          : "We couldn’t load attendance totals. Please refresh before enabling check-in.");
      }
    });
    return () => { cancelled = true; };
  }, [client, reload, counts]);

  async function enableEntrance() {
    if (!summary || setupBusy.current) return;
    setupBusy.current = true;
    setEnabling(true);
    setSetupError("");
    try {
      const point = await client.enableOrganizerEntrance(summary.cycleId);
      setAdded(point);
      setSelection(point.id);
      onRefresh();
    } catch (error: unknown) {
      setSetupError(error instanceof ApiError && error.status === 409
        ? "The event setup changed. Refresh to load the latest check-in options."
        : "We couldn’t confirm setup. You can safely try again; this won’t create a second entrance.");
    } finally { setupBusy.current = false; setEnabling(false); }
  }

  useEffect(() => {
    let cancelled = false;
    void client.listOrganizerRedemptions().then((data) => {
      if (!cancelled) { setRecent(data.items); setError(false); }
    }).catch(() => { if (!cancelled) setError(true); });
    return () => { cancelled = true; };
  }, [client, reload, counts]);

  const rows = selected ? (recent ?? []).filter((row) => row.checkpoint.id === selected.id).slice(0, 5) : [];
  return (
    <div className="check-in-overview">
      <div className="check-in-actions">
        <Link href="/scanner" className="button primary">Open scanner <span aria-hidden="true">↗</span></Link>
        <Link href="/organizer/reviewers" className="staff-link">Manage volunteers</Link>
      </div>

      {checkpoints.length > 1 ? <div className="check-in-selection">
        <label htmlFor="check-in-activity">Show check-ins for</label>
        <select id="check-in-activity" value={selected?.id ?? ""} onChange={(event) => setSelection(event.target.value)}>
          <option value="">Choose entrance or a meal</option>
          {checkpoints.map((point) => <option key={point.id} value={point.id}>{point.name}{point.active ? "" : " (closed)"}</option>)}
        </select>
      </div> : null}

      <dl className="event-day-stats">
        <div><dt>Confirmed attendees</dt><dd>{summary?.confirmedRsvps ?? "—"}</dd></div>
        <div><dt>People checked in{selected ? ` · ${selected.name}` : ""}</dt><dd>{summary && !checkpoints.length ? 0 : count?.uniqueAttendees ?? "—"}</dd></div>
      </dl>
      {summaryError ? <p role="alert">{summaryError}</p>
        : !summary ? <p role="status">Loading attendance…</p>
        : !checkpoints.length ? <section className="check-in-setup" aria-labelledby="enable-check-in-heading">
          <h2 id="enable-check-in-heading">Ready for arrivals?</h2>
          <p className="staff-muted">Enable entrance check-in for {summary.cycleName}. Each attendee can check in once with their released pass.</p>
          <button type="button" className="button primary" disabled={enabling} onClick={() => void enableEntrance()}>{enabling ? "Enabling check-in…" : "Enable entrance check-in"}</button>
          <p className="staff-muted check-in-count-note">Enables scanning now. This does not release passes or change RSVPs.</p>
          {setupError ? <p role="alert">{setupError}</p> : null}
        </section>
        : !selected ? <p className="staff-muted">Choose what to view. Entrance and meal check-ins are counted separately.</p>
          : !selected.active ? <p className="staff-muted" role="status">{selected.name} is closed. Its existing scanning rules have been kept.</p>
          : count?.uniqueAttendees === undefined ? <p className="staff-muted" role="status">Attendance totals are currently unavailable. Refresh to update check-in counts.</p>
            : <p className="staff-muted check-in-count-note">Confirmed for the event. Checked in once per person at {selected.name}.</p>}

      {children}

      <section className="check-in-recent" aria-labelledby="recent-check-ins-heading">
        <div className="operations-section-heading">
          <h2 id="recent-check-ins-heading">Recent check-ins</h2>
          <button type="button" className="button secondary" onClick={() => { setRecent(null); setError(false); setReload((value) => value + 1); onRefresh(); }}>Refresh</button>
        </div>
        {error ? <p role="alert">We couldn’t load recent check-ins. Please refresh.</p>
          : !selected ? <p className="staff-muted">{checkpoints.length ? "Choose what to view above." : "Recent check-ins will appear here."}</p>
            : recent === null ? <p role="status">Loading check-ins…</p>
              : rows.length === 0 ? <p className="staff-muted">No recent check-ins for {selected.name}.</p>
                : <ul className="event-day-recent">{rows.map((row) => (
                  <li key={row.id}><strong>{row.attendee.displayName}</strong><time dateTime={row.redeemedAt}>{new Date(row.redeemedAt).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}</time></li>
                ))}</ul>}
        {rows.length > 0 && !error ? <p className="staff-muted check-in-count-note">Recent activity, not a complete attendance list.</p> : null}
      </section>
    </div>
  );
}
