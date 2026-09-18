"use client";

import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useState } from "react";
import Image from "next/image";
import { ApiError, createApiClient, type AuthenticatedAttendeePass } from "@/lib/api";
import { PassQrCode } from "@/components/pass-qr-code";

type PassState =
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "error" }
  | { kind: "ready"; pass: AuthenticatedAttendeePass; userId: string };

// Event logistics supplied by the organizers. The QR credential remains API-owned.
export function EventTicket({ pass }: { pass: AuthenticatedAttendeePass }) {
  return (
    <article className="event-ticket" aria-labelledby="ticket-title">
      <div className="ticket-top">
        <div className="ticket-brand-row">
          <span className="ticket-wordmark">HACKATLANTIC</span>
          <span className="ticket-edition">2026</span>
        </div>
        <div className="ticket-title-row">
          <div>
            <p className="ticket-eyebrow">September 26–27 · Fredericton</p>
            <h1 id="ticket-title">You’re in.</h1>
            <p className="ticket-subtitle">Your Hack Atlantic event pass</p>
          </div>
          <Image className="ticket-logo" src="/hackatlantic-logo.jpg" alt="" width={88} height={88} />
        </div>
        <div className="ticket-attendee">
          <span className="ticket-label">Attendee</span>
          <strong>{pass.displayName || "Hack Atlantic attendee"}</strong>
        </div>
        <dl className="ticket-details">
          <div><dt>Check-in date</dt><dd><time dateTime="2026-09-26">Sat, Sep 26</time></dd></div>
          <div><dt>Check-in time</dt><dd><time dateTime="2026-09-26T09:30:00-03:00">9:30 AM <span>ADT</span></time></dd></div>
          <div className="ticket-location"><dt>Location</dt><dd>UNB · Head Hall Atrium</dd></div>
        </dl>
      </div>
      <div className="ticket-perforation" aria-hidden="true"><span /></div>
      <div className="ticket-stub">
        <div className="ticket-scan-heading"><h2>Ready for check-in</h2><span>ADMIT ONE</span></div>
        <div className="ticket-code"><PassQrCode value={pass.qrToken} /></div>
        <p className="ticket-scan-copy">Show this code to a volunteer at the entrance.</p>
        <p className="ticket-private">This pass is yours. Keep your QR code private.</p>
      </div>
    </article>
  );
}

export function EventPass() {
  const { getToken, userId, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [state, setState] = useState<PassState>({ kind: "loading" });
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (!isLoaded || !userId) return;
    let cancelled = false;
    async function load() {
      try {
        const pass = await client.getAttendeePass();
        if (!cancelled) setState(pass.status === "active" ? { kind: "ready", pass, userId: userId! } : { kind: "unavailable" });
      } catch (error) {
        if (!cancelled) setState({ kind: error instanceof ApiError && error.status === 404 ? "unavailable" : "error" });
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [client, isLoaded, userId, reload]);

  // Hide loaded credentials immediately on sign-out or account switching.
  if (isLoaded && !userId) {
    return <section className="ticket-message"><h1>Sign in to view your pass</h1><p>Return to the dashboard to sign in.</p></section>;
  }
  if (isLoaded && state.kind === "ready" && state.userId === userId) return <EventTicket pass={state.pass} />;
  const loading = !isLoaded || state.kind === "loading" || state.kind === "ready";
  return (
    <section className="ticket-message" aria-live="polite" aria-busy={loading}>
      <p className="ticket-eyebrow">Hack Atlantic · Event pass</p>
      <h1>{loading ? "Getting your pass…" : state.kind === "unavailable" ? "Your pass is on its way." : "Let’s try that again."}</h1>
      <p>{loading ? "Checking your entry pass." : state.kind === "unavailable" ? "Passes are released by the organizers shortly before the event, after you confirm your RSVP. No pass has been issued to your account yet." : "We couldn’t load your pass. Check your connection and try again."}</p>
      {!loading && <button className="button secondary" type="button" onClick={() => { setState({ kind: "loading" }); setReload((value) => value + 1); }}>{state.kind === "error" ? "Try again" : "Check again"}</button>}
    </section>
  );
}
