"use client";

import Link from "next/link";
import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useState } from "react";
import { createApiClient, type OrganizerCheckpoint, type OrganizerRedemption, type OrganizerRedemptionCount } from "@/lib/api";

export function EventDayOverview({ checkpoints, counts, onSetup, onRefresh }: {
  checkpoints: OrganizerCheckpoint[];
  counts: OrganizerRedemptionCount[];
  onSetup: () => void;
  onRefresh: () => void;
}) {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const active = checkpoints.filter((point) => point.active);
  const [selection, setSelection] = useState("");
  const selected = checkpoints.find((point) => point.id === selection) ?? (active.length === 1 ? active[0] : null);
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
  const rows = (recent ?? []).filter((row) => !selected || row.checkpoint.id === selected.id).slice(0, 10);
  return <section className="event-day-overview" aria-labelledby="event-day-heading">
    <div className="operations-section-heading">
      <div><h2 id="event-day-heading">Ready for arrivals</h2><p className="staff-muted">Open the scanner to welcome attendees. Manage event settings separately.</p></div>
      <div className="staff-actions"><Link href="/scanner" className="button primary">Open scanner</Link><Link href="/organizer/reviewers" className="button secondary">Manage volunteers</Link></div>
    </div>
    {!active.length ? <div className="operations-confirmation"><p>No scan points are active. Set up your entrance before check-in begins.</p><button className="button secondary" onClick={onSetup} type="button">Set up entrance</button></div> : null}
    <div className="operations-field">
      <label htmlFor="event-day-point">Attendance at</label>
      <select id="event-day-point" value={selected?.id ?? ""} onChange={(event) => setSelection(event.target.value)}>
        <option value="">Choose an entrance or scan point</option>
        {checkpoints.map((point) => <option key={point.id} value={point.id}>{point.name}{point.active ? "" : " (inactive)"}</option>)}
      </select>
    </div>
    {selected ? <>
      <dl className="event-day-stats">
        <div><dt>People checked in here</dt><dd>{count?.uniqueAttendees ?? "—"}</dd></div>
        <div><dt>Confirmed for this event</dt><dd>{count?.confirmedRsvps ?? "—"}</dd></div>
      </dl>
      <p className="staff-muted">Each person is counted once at {selected.name}. Confirmed attendance includes all current accepted RSVPs for this event, not just this scan point.</p>
      {count?.uniqueAttendees === undefined ? <p role="status">Attendance totals are not available from the API yet.</p> : null}
    </> : <p className="staff-muted">Choose the entrance to see unique check-ins. We don’t combine entrance, meal and swag scans into an attendance total.</p>}
    <div className="operations-section-heading"><h3>Recent check-ins</h3><button type="button" className="button secondary" onClick={() => { setRecent(null); setError(false); setReload((value) => value + 1); onRefresh(); }}>Refresh overview</button></div>
    {error ? <p role="alert">We couldn’t load recent check-ins. Try refreshing activity.</p> : recent === null ? <p role="status">Loading check-ins…</p> : rows.length === 0 ? <p>No check-ins in the latest activity for this selection.</p> : <ul className="event-day-recent">{rows.map((row) => <li key={row.id}><div><strong>{row.attendee.displayName}</strong><span>{row.checkpoint.name}</span></div><time dateTime={row.redeemedAt}>{new Date(row.redeemedAt).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}</time></li>)}</ul>}
    <p className="staff-muted">Shows up to 10 matches from the latest 100 successful scans. Use the attendance export for the complete record.</p>
  </section>;
}
