"use client";

import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useRef, useState } from "react";
import { createApiClient, type OrganizerCheckpoint, type OrganizerEntitlement } from "@/lib/api";

type Rule = { allowed: boolean; maxRedemptions: number };
export function AttendeeAccessSettings({ attendeeId, cycleId, displayName }: { attendeeId: string; cycleId: string; displayName: string }) {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [points, setPoints] = useState<OrganizerCheckpoint[]>([]);
  const [pointId, setPointId] = useState("");
  const [rule, setRule] = useState<Rule>({ allowed: true, maxRedemptions: 1 });
  const [existing, setExisting] = useState<OrganizerEntitlement | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirmation, setConfirmation] = useState<"save" | "remove" | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const requestBusy = useRef(false);
  const point = points.find((item) => item.id === pointId);
  useEffect(() => {
    let cancelled = false;
    void client.listOrganizerCheckpoints().then((data) => { if (!cancelled) setPoints(data.items.filter((item) => item.cycleId === cycleId)); })
      .catch(() => { if (!cancelled) setError("Couldn’t load scan points. Try again."); });
    return () => { cancelled = true; };
  }, [client, cycleId, reload]);

  async function loadRule(id: string) {
    if (requestBusy.current) return;
    setPointId(id); setReady(false); setConfirmation(null); setExisting(null); setError(""); setMessage("");
    const selected = points.find((item) => item.id === id);
    if (!selected) return;
    requestBusy.current = true; setBusy(true);
    try {
      const data = await client.getOrganizerAttendeeEntitlement(attendeeId, id);
      setExisting(data.override);
      setRule({ allowed: data.override?.allowed ?? selected.defaultAllowed, maxRedemptions: data.override?.maxRedemptions ?? selected.defaultMaxRedemptions });
      setReady(true);
    } catch { setError("Couldn’t load this person’s access. Retry before making changes."); }
    finally { requestBusy.current = false; setBusy(false); }
  }

  async function confirm() {
    if (!confirmation || !point || !ready || requestBusy.current) return;
    if (!Number.isInteger(rule.maxRedemptions) || rule.maxRedemptions < 0) { setError("Allowed uses must be a whole number of zero or more."); return; }
    requestBusy.current = true; setBusy(true); setError(""); setMessage("");
    try {
      if (confirmation === "remove") {
        await client.deleteOrganizerAttendeeEntitlement(attendeeId, pointId);
        setExisting(null); setRule({ allowed: point.defaultAllowed, maxRedemptions: point.defaultMaxRedemptions });
        setMessage("Exception removed. The scan point’s usual rules apply.");
      } else {
        const saved = await client.updateOrganizerAttendeeEntitlement(attendeeId, pointId, rule);
        setExisting(saved); setMessage("Access exception saved.");
      }
      setConfirmation(null);
    } catch { setError("Couldn’t confirm the change was saved. Check this person’s current access before trying again."); setReady(false); setConfirmation(null); }
    finally { requestBusy.current = false; setBusy(false); }
  }

  return <section id="access-settings" className="attendee-access-settings" aria-labelledby="access-settings-heading">
    <h2 id="access-settings-heading">Event access</h2>
    <p className="staff-muted">Most attendees use the scan point’s usual rules. Exceptions only affect this person; they never grant staff access or erase previous check-ins.</p>
    <details className="operations-advanced">
      <summary>Manage an access exception</summary>
      <div className="operations-form">
        <div className="operations-field"><label htmlFor="person-scan-point">Scan point</label><select id="person-scan-point" value={pointId} disabled={busy} onChange={(event) => void loadRule(event.target.value)}><option value="">Choose a scan point</option>{points.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></div>
        {busy ? <p role="status">Loading or saving access…</p> : null}
        {ready ? <>
          <p>{existing ? "This person has a custom rule." : "Using the scan point’s usual rules."}</p>
          <div className="operations-checkboxes"><label><input type="checkbox" checked={rule.allowed} disabled={busy} onChange={(event) => { setConfirmation(null); setRule({ ...rule, allowed: event.target.checked }); }} />Allow access</label></div>
          <div className="operations-field"><label htmlFor="person-scan-limit">Total allowed uses</label><input id="person-scan-limit" type="number" min="0" step="1" value={Number.isNaN(rule.maxRedemptions) ? "" : rule.maxRedemptions} disabled={busy} onChange={(event) => { setConfirmation(null); setRule({ ...rule, maxRedemptions: event.target.valueAsNumber }); }} /><p>This is the total allowance, not extra uses on top of earlier scans.</p></div>
          <div className="staff-actions"><button className="button primary" type="button" disabled={busy} onClick={() => { setError(""); if (!Number.isInteger(rule.maxRedemptions) || rule.maxRedemptions < 0) setError("Allowed uses must be a whole number of zero or more."); else setConfirmation("save"); }}>Save exception</button>{existing ? <button type="button" className="button secondary" disabled={busy} onClick={() => setConfirmation("remove")}>Use usual rules</button> : null}</div>
        </> : null}
        {confirmation ? <div className="operations-confirmation"><h3>Confirm access change</h3><p>{confirmation === "remove" ? `Restore the usual rules for ${displayName} at ${point?.name}?` : `${rule.allowed ? `Allow ${rule.maxRedemptions} total uses` : "Deny access"} for ${displayName} at ${point?.name}?`}</p><div className="staff-actions"><button type="button" className="button primary" disabled={busy} onClick={() => void confirm()}>Confirm change</button><button type="button" className="button secondary" disabled={busy} onClick={() => setConfirmation(null)}>Cancel</button></div></div> : null}
        {message ? <p role="status">{message}</p> : null}
        {error ? <div><p role="alert">{error}</p><button type="button" className="button secondary" disabled={busy} onClick={() => { setError(""); if (pointId) void loadRule(pointId); else setReload((value) => value + 1); }}>Try again</button></div> : null}
      </div>
    </details>
  </section>;
}
