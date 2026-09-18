"use client";

import Link from "next/link";
import { useAuth } from "@clerk/nextjs";
import { useEffect, useMemo, useState } from "react";
import { ApiError, createApiClient } from "@/lib/api";

export function ApplicantPass() {
  const { getToken, userId, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [result, setResult] = useState<{ owner: string; status: "ready" | "waiting" | "error" } | null>(null);
  const [reload, setReload] = useState(0);
  useEffect(() => {
    if (!isLoaded || !userId) return;
    let cancelled = false;
    void client.getAttendeePass().then((pass) => {
      // Only retain availability here. The ticket page owns the QR credential.
      if (!cancelled) setResult({ owner: userId, status: pass.status === "active" ? "ready" : "waiting" });
    }).catch((error: unknown) => {
      if (!cancelled) setResult({ owner: userId, status: error instanceof ApiError && error.status === 404 ? "waiting" : "error" });
    });
    return () => { cancelled = true; };
  }, [client, userId, isLoaded, reload]);
  const status = isLoaded && userId && result?.owner === userId ? result.status : "loading";
  return (
    <section className="attendance-next-step" aria-label="Your event pass" aria-live="polite">
      {status === "loading" ? <p>Checking your event pass…</p> : status === "ready" ? <>
        <p>Your pass is ready. Show it to a volunteer at the entrance.</p>
        <Link className="button primary" href="/event-pass" prefetch={false}>View event pass <span aria-hidden="true">↗</span></Link>
      </> : <>
        <p>{status === "waiting" ? "Your pass will appear here when the organizers release it before the event." : "We couldn’t check your pass. Please try again."}</p>
        <button className="button secondary" type="button" onClick={() => { setResult(null); setReload((value) => value + 1); }}>{status === "waiting" ? "Check for pass" : "Try again"}</button>
      </>}
    </section>
  );
}
