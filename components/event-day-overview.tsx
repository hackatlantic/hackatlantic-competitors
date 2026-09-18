"use client";

import Link from "next/link";
import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { createApiClient, type OrganizerCheckpoint, type OrganizerRedemption, type OrganizerRedemptionCount } from "@/lib/api";

export function CheckInOverview({ checkpoints, counts, onRefresh, children }: {
  checkpoints: OrganizerCheckpoint[];
  counts: OrganizerRedemptionCount[];
  onRefresh: () => void;
  children: ReactNode;
}) {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [selection, setSelection] = useState("");
  // Never guess which configured activity represents entrance attendance.
  const selected = checkpoints.find((point) => point.id === selection) ?? (checkpoints.length === 1 ? checkpoints[0] : null);
  const count = counts.find((item) => item.checkpointId === selected?.id);
  const [recent, setRecent] = useState<OrganizerRedemption[] | null>(null);
  const [error, setError] = useState(false);
  const [reload, setReload] = useState(0);

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
        <div><dt>Confirmed attendees</dt><dd>{count?.confirmedRsvps ?? "—"}</dd></div>
        <div><dt>People checked in{selected ? ` · ${selected.name}` : ""}</dt><dd>{count?.uniqueAttendees ?? "—"}</dd></div>
      </dl>
      {!checkpoints.length ? <p className="staff-muted" role="status">Check-in hasn’t been configured yet. Ask the event lead to enable scanning before arrivals.</p>
        : !selected ? <p className="staff-muted">Choose what to view. Entrance and meal check-ins are counted separately.</p>
          : count?.uniqueAttendees === undefined || count?.confirmedRsvps === undefined ? <p className="staff-muted" role="status">Attendance totals are currently unavailable.</p>
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
