"use client";

import { useAuth } from "@clerk/nextjs";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { createApiClient, type VolunteerAccess, type VolunteerRequest } from "@/lib/api";

const errorMessage = (error: unknown) => error instanceof Error ? error.message : "Something went wrong. Please try again.";
const invitationUrl = "https://apply.hackatlantic.ca/volunteer";

function VolunteerInvitation() {
  const [copyStatus, setCopyStatus] = useState("");
  async function copyLink() {
    try {
      await navigator.clipboard.writeText(invitationUrl);
      setCopyStatus("Link copied. Ready to share in Discord.");
    } catch {
      setCopyStatus("Couldn’t copy automatically. Select and copy the link above.");
    }
  }
  return <section className="volunteer-invitation" aria-labelledby="volunteer-invitation-title">
    <div>
      <h2 id="volunteer-invitation-title">Invite your volunteers</h2>
      <p>Share this link in Discord. Volunteers sign in and request access using their real name.</p>
    </div>
    <div className="volunteer-link-row">
      <a href={invitationUrl} className="volunteer-share">apply.hackatlantic.ca/volunteer</a>
      <button type="button" className="button primary" onClick={() => void copyLink()}>Copy invitation link</button>
    </div>
    <p className="volunteer-link-note">Only your approval grants scanner access.</p>
    {copyStatus && <p role="status" className="volunteer-feedback">{copyStatus}</p>}
  </section>;
}

export function VolunteerSignup() {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [access, setAccess] = useState<VolunteerAccess | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const locked = useRef(false);
  const load = useCallback(async () => {
    if (locked.current) return;
    locked.current = true; setBusy(true); setError("");
    try { setAccess(await client.getVolunteerAccess()); } catch (e) { setError(errorMessage(e)); }
    finally { locked.current = false; setBusy(false); }
  }, [client]);
  useEffect(() => {
    let active = true;
    locked.current = true;
    void client.getVolunteerAccess()
      .then((result) => { if (active) setAccess(result); })
      .catch((e) => { if (active) setError(errorMessage(e)); })
      .finally(() => { if (active) { locked.current = false; setBusy(false); } });
    return () => { active = false; locked.current = false; };
  }, [client]);
  async function submit(event: FormEvent) {
    event.preventDefault(); if (locked.current) return;
    locked.current = true; setBusy(true); setError("");
    try { setAccess(await client.requestVolunteerAccess(name.trim())); } catch (e) { setError(errorMessage(e)); }
    finally { locked.current = false; setBusy(false); }
  }
  const request = access?.request;
  return <div className="volunteer-step">
    {error && <p className="error-message" role="alert">{error}</p>}
    {!access ? <><p role="status">{busy ? "Checking your account…" : "We couldn’t load your access status."}</p><button className="button secondary" disabled={busy} onClick={() => void load()}>Try again</button></> :
      access.scannerAccess ? <><h2>You’re ready to scan</h2><p>Choose check-in or a meal in the scanner, then scan and confirm each attendee.</p><Link className="button primary" href="/scanner">Open scanner</Link></> :
      request ? <>
        <h2>{request.status === "pending" ? "Request sent" : "Contact your volunteer lead"}</h2>
        <p role="status">{request.status === "pending" ? `Thanks, ${request.realName}. An admin needs to approve your scanner access.` : "Scanner access is not active for this account. Ask your lead in Discord if you need help."}</p>
        <p>No application is needed. Check back here after your lead approves you.</p>
        <button className="button secondary" disabled={busy} onClick={() => void load()}>{busy ? "Checking…" : "Check approval"}</button>
      </> : <form onSubmit={submit}>
        <h2>Request scanner access</h2>
        <label htmlFor="volunteer-real-name">Your name on the volunteer schedule</label>
        <input id="volunteer-real-name" autoComplete="name" required minLength={2} maxLength={100} value={name} disabled={busy} onChange={(e) => setName(e.target.value)} aria-describedby="volunteer-request-help" />
        <p id="volunteer-request-help">Your name helps the admins match you to the schedule. Access is only granted after approval.</p>
        <button className="button primary" disabled={busy || name.trim().length < 2} type="submit">{busy ? "Sending…" : "Request access"}</button>
      </form>}
  </div>;
}

export function VolunteerApprovalQueue() {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [items, setItems] = useState<VolunteerRequest[]>([]);
  const [offset, setOffset] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [verified, setVerified] = useState<Record<string, boolean>>({});
  const locked = useRef(false);
  const load = useCallback(async (page: number) => {
    if (locked.current) return;
    locked.current = true; setBusy(true); setError(""); setVerified({}); setLoaded(false);
    try { const result = await client.listVolunteerRequests(page); setItems(result.items); setOffset(page); setLoaded(true); }
    catch (e) { setError(errorMessage(e)); }
    finally { locked.current = false; setBusy(false); }
  }, [client]);
  useEffect(() => {
    let active = true;
    locked.current = true;
    void client.listVolunteerRequests(0)
      .then((result) => { if (active) { setItems(result.items); setLoaded(true); } })
      .catch((e) => { if (active) setError(errorMessage(e)); })
      .finally(() => { if (active) { locked.current = false; setBusy(false); } });
    return () => { active = false; locked.current = false; };
  }, [client]);
  async function review(item: VolunteerRequest, status: "approved" | "rejected" | "revoked") {
    if (locked.current || (status === "approved" && !verified[item.userId])) return;
    locked.current = true; setBusy(true); setError(""); setMessage("");
    try {
      await client.reviewVolunteerRequest(item.userId, status, item.status);
      setItems((rows) => rows.map((r) => r.userId === item.userId ? { ...r, status, scannerAccess: status === "approved" ? true : status === "revoked" ? false : r.scannerAccess } : r));
      setVerified({}); setMessage(status === "approved" ? `Scanner access approved for ${item.realName}.` : `Request ${status} for ${item.realName}.`);
    } catch (e) { setError(errorMessage(e)); setLoaded(false); setVerified({}); }
    finally { locked.current = false; setBusy(false); }
  }
  return <div className="volunteer-queue">
    <VolunteerInvitation />
    <section className="volunteer-requests" aria-labelledby="volunteer-queue-title">
    <div className="volunteer-queue-header">
      <div><h2 id="volunteer-queue-title">Volunteer requests</h2><p>Approve people you recognize from the schedule.</p></div>
      <button className="button secondary" disabled={busy} onClick={() => void load(offset)}>{busy ? "Loading…" : "Refresh requests"}</button>
    </div>
    <details className="volunteer-review-help"><summary>Before you approve</summary><p>Names are self-reported. Verify unfamiliar or duplicate names in Discord; use the account email to distinguish requests. Scanners can check passes, but cannot access applications or review notes.</p></details>
    {error && <p className="error-message" role="alert">{error}</p>}
    {message && <p className="volunteer-feedback" role="status">{message}</p>}
    {busy && !loaded && <p role="status" className="volunteer-empty">Loading volunteer requests…</p>}
    {loaded && items.length === 0 && <div className="volunteer-empty"><h3>{offset === 0 ? "No requests yet" : "No more requests"}</h3><p>{offset === 0 ? "Once volunteers use your invitation link, their requests will appear here." : "Go back to see earlier requests."}</p></div>}
    {loaded && items.map((item) => <article key={item.userId} className="volunteer-request" aria-label={`Request from ${item.realName}`}>
      <div className="volunteer-request-header"><div><h3>{item.realName}</h3><p>{item.email}</p></div><span className="volunteer-status" data-active={item.scannerAccess}>{item.scannerAccess ? "Scanner active" : item.status === "pending" ? "Awaiting approval" : "No scanner access"}</span></div>
      <p className="volunteer-request-status">Request: {item.status}</p>
      {item.status === "pending" && <label className="volunteer-confirm"><input type="checkbox" checked={!!verified[item.userId]} disabled={busy} onChange={(e) => setVerified({ ...verified, [item.userId]: e.target.checked })} />I recognize this volunteer and have verified this account belongs to them.</label>}
      <div className="volunteer-actions">
        {item.status === "pending" && <button className="button primary" disabled={busy || !verified[item.userId]} onClick={() => void review(item,"approved")}>Approve scanner access</button>}
        {item.status === "pending" && <button className="button secondary" disabled={busy} onClick={() => void review(item,"rejected")}>Decline request</button>}
        {item.status === "approved" && <button className="button secondary" disabled={busy} onClick={() => void review(item,"revoked")}>Revoke scanner access</button>}
      </div>
      {(item.status === "rejected" || item.status === "revoked") && <p>To reconsider this account, verify the person again and use Manage access by email below.</p>}
    </article>)}
    {(offset > 0 || (loaded && items.length === 50)) && <nav className="volunteer-actions" aria-label="Request pages">
      {offset > 0 && <button className="button secondary" disabled={busy} onClick={() => void load(Math.max(0,offset-50))}>Previous requests</button>}
      {loaded && items.length === 50 && <button className="button secondary" disabled={busy} onClick={() => void load(offset+50)}>Next requests</button>}
    </nav>}
    </section>
  </div>;
}
