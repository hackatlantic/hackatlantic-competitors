"use client";

import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useRef, useState } from "react";
import { ApplicationButton } from "@/components/application-motion";
import { ApiError, createApiClient, type AttendanceRSVP, type RSVPStatus } from "@/lib/api";

export function ApplicantRSVP({ applicationId }: { applicationId: string }) {
  const { getToken } = useAuth();
  const api = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [response, setResponse] = useState<AttendanceRSVP | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [declining, setDeclining] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);
  const busy = useRef(false);
  const requestSequence = useRef(0);

  useEffect(() => {
    const requests = requestSequence;
    const sequence = ++requests.current;
    const load = async () => {
      try {
        const current = await api.getApplicationRSVP(applicationId);
        if (sequence === requests.current) {
          setResponse(current);
          setLoading(false);
        }
      } catch (cause) {
        if (sequence !== requests.current) return;
        setResponse(null);
        setLoading(false);
        setError(cause instanceof ApiError && cause.status === 404
          ? "RSVP is no longer available for this acceptance. Refresh your application to check the latest decision."
          : "We couldn’t load your RSVP. Please try again.");
      }
    };
    void load();
    return () => { requests.current++; };
  }, [api, applicationId, reloadVersion]);

  function reload() {
    setLoading(true);
    setError("");
    setNotice("");
    setReloadVersion((version) => version + 1);
  }

  async function respond(status: Exclude<RSVPStatus, "pending">) {
    if (!response || busy.current) return;
    busy.current = true;
    setSaving(true);
    setError("");
    setNotice("");
    const sequence = ++requestSequence.current;
    try {
      const saved = await api.respondToRSVP(applicationId, {
        decisionId: response.decisionId, lockVersion: response.lockVersion, status,
      });
      if (sequence !== requestSequence.current) return;
      setResponse(saved);
      setDeclining(false);
      setNotice("RSVP updated.");
    } catch (cause) {
      if (sequence !== requestSequence.current) return;
      if (cause instanceof ApiError && (cause.status === 409 || cause.status === 404)) {
        setResponse(null);
        setDeclining(false);
        setError("Your response or acceptance changed. Reload your RSVP before responding again.");
      } else {
        setError("We couldn’t confirm your RSVP was saved. Please retry; repeating the same response won’t create a duplicate.");
      }
    } finally {
      busy.current = false;
      if (sequence === requestSequence.current) setSaving(false);
    }
  }

  return (
    <section className="application-rsvp" aria-labelledby="rsvp-heading" aria-busy={loading || saving}>
      <div className="event-card-heading">
        <span className="event-card-kicker">Event</span>
        <h2 id="rsvp-heading">Attendance</h2>
      </div>
      {loading ? <p role="status">Loading your RSVP…</p> : null}
      {!loading && response ? (
        <>
          <p className={`rsvp-status rsvp-${response.status}`}>
            <span>RSVP status</span>
            <strong>{response.status === "confirmed" ? "Confirmed" : response.status === "declined" ? "Not attending" : "Response needed"}</strong>
          </p>
          <p className="rsvp-help">
            {response.status === "confirmed"
              ? "You’re on the attendee list. Update your response here if your plans change."
              : response.status === "declined"
                ? "We’ve recorded that you can’t attend. You can confirm again if your plans change."
                : "Confirm attendance before entry passes are released."}
          </p>
          {declining ? (
            <div className="rsvp-confirmation" role="group" aria-label="Confirm you will not attend">
              <p>Change your RSVP to not attending?</p>
              <div className="rsvp-actions">
                <ApplicationButton className="button secondary" disabled={saving} onClick={() => void respond("declined")} type="button">{saving ? "Saving…" : "Confirm change"}</ApplicationButton>
                <ApplicationButton className="button secondary" disabled={saving} onClick={() => setDeclining(false)} type="button">Cancel</ApplicationButton>
              </div>
            </div>
          ) : (
            <div className="rsvp-actions">
              <ApplicationButton className="button primary" disabled={saving || response.status === "confirmed"} onClick={() => void respond("confirmed")} type="button">{saving ? "Saving…" : response.status === "confirmed" ? "Confirmed" : "Confirm attendance"}</ApplicationButton>
              <ApplicationButton className="button secondary" disabled={saving || response.status === "declined"} onClick={() => setDeclining(true)} type="button">Can’t attend</ApplicationButton>
            </div>
          )}
        </>
      ) : null}
      {notice ? <p role="status">{notice}</p> : null}
      {error ? <p className="error-message" role="alert">{error}</p> : null}
      {!loading && !response ? <ApplicationButton className="button secondary" onClick={reload} type="button">Reload RSVP</ApplicationButton> : null}
    </section>
  );
}
