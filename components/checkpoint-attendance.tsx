"use client";

import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useState } from "react";
import { createApiClient, type OrganizerCheckpoint, type OrganizerRedemption } from "@/lib/api";

// The API's maximum report size. Never describe a capped report as complete.
const REPORT_LIMIT = 500;
const timeFormat = new Intl.DateTimeFormat("en-US", {
  timeZone: "America/Moncton", month: "short", day: "numeric",
  hour: "numeric", minute: "2-digit", timeZoneName: "short",
});

export function CheckpointAttendance({ checkpoint, refreshKey, onRefresh }: {
  checkpoint: OrganizerCheckpoint | null;
  refreshKey: number;
  onRefresh: () => void;
}) {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [result, setResult] = useState<{ items: OrganizerRedemption[]; refreshKey: number } | null>(null);
  const [failed, setFailed] = useState(false);
  const [search, setSearch] = useState("");
  const checkpointId = checkpoint?.id;
  useEffect(() => {
    if (!checkpointId) return;
    let cancelled = false;
    void client.listOrganizerRedemptions({ checkpointId, limit: REPORT_LIMIT }).then((data) => {
      if (!cancelled) { setResult({ items: data.items, refreshKey }); setFailed(false); }
    }).catch(() => { if (!cancelled) setFailed(true); });
    return () => { cancelled = true; };
  }, [client, checkpointId, refreshKey]);

  const loaded = result?.refreshKey === refreshKey ? result.items : null;
  const seen = new Set<string>();
  const attendees = (loaded ?? []).filter((row) => {
    if (row.checkpoint.id !== checkpointId || seen.has(row.attendee.id)) return false;
    seen.add(row.attendee.id);
    return true;
  });
  const rows = attendees.filter((row) => row.attendee.displayName.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  const capped = (loaded?.length ?? 0) >= REPORT_LIMIT;

  return <section className="check-in-recent" aria-labelledby="checkpoint-attendance-heading">
    <div className="operations-section-heading">
      <h2 id="checkpoint-attendance-heading">Who’s checked in{checkpoint ? ` · ${checkpoint.name}` : ""}</h2>
      <button type="button" className="button secondary" onClick={() => { setFailed(false); setResult(null); onRefresh(); }}>Refresh</button>
    </div>
    <p className="staff-muted check-in-count-note">Successful check-ins only. Failed scans, duplicate attempts and pass-only re-entry checks are not included.</p>
    {!checkpoint ? <p className="staff-muted">Choose a checkpoint above to see its attendance.</p>
      : failed ? <p role="alert">We couldn’t load attendance. Please refresh.</p>
        : !loaded ? <p role="status">Loading check-ins…</p>
          : <>
            <div className="check-in-selection">
              <label htmlFor="attendance-search">Search checked-in names</label>
              <input id="attendance-search" type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Attendee name" />
            </div>
            <p className="staff-muted" role="status">Showing {rows.length} of {attendees.length} {capped ? "loaded " : ""}people at {checkpoint.name}.</p>
            {capped ? <p className="staff-muted">This list covers the latest 500 successful scans at this checkpoint and may omit older check-ins. The total above is not limited by this list.</p> : null}
            {!attendees.length ? <p className="staff-muted">No check-ins recorded for {checkpoint.name} yet.</p>
              : !rows.length ? <p className="staff-muted">No matching names in this attendance list.</p>
                : <ul className="event-day-recent">{rows.map((row) => <li key={row.attendee.id}>
                    <strong>{row.attendee.displayName || "Unnamed attendee"}</strong>
                    <time dateTime={row.redeemedAt}>{timeFormat.format(new Date(row.redeemedAt))}</time>
                  </li>)}</ul>}
          </>}
  </section>;
}
