"use client";

import { useAuth } from "@clerk/nextjs";
import { PassQrCode } from "@/components/pass-qr-code";
import { useEffect, useMemo, useState } from "react";
import { ApiError, createApiClient, type AuthenticatedAttendeePass } from "@/lib/api";

type PassLoadState = "loading" | "ready" | "unavailable" | "error";

function displayTimestamp(value: string): string {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "long",
    timeStyle: "short",
  }).format(timestamp);
}

function isPassUnavailable(error: unknown): error is ApiError {
  return error instanceof ApiError && error.status === 404;
}

export function ApplicantPass() {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [pass, setPass] = useState<AuthenticatedAttendeePass | null>(null);
  const [state, setState] = useState<PassLoadState>("loading");
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => {
    let cancelled = false;

    const loadPass = async () => {
      try {
        const nextPass = await client.getAttendeePass();
        if (!cancelled) {
          setPass(nextPass);
          setState("ready");
        }
      } catch (error) {
        if (!cancelled) {
          setPass(null);
          setState(isPassUnavailable(error) ? "unavailable" : "error");
        }
      }
    };

    void loadPass();

    return () => {
      cancelled = true;
    };
  }, [client, reloadVersion]);

  const retryPass = () => {
    setState("loading");
    setReloadVersion((version) => version + 1);
  };

  if (state === "loading") {
    return (
      <section className="application-pass" aria-busy="true" aria-live="polite">
        <div className="event-card-heading">
          <span className="event-card-kicker">Access</span>
          <h2>Entry pass</h2>
        </div>
        <p>Checking pass status…</p>
      </section>
    );
  }

  if (state === "unavailable") {
    return (
      <section className="application-pass" aria-live="polite">
        <div className="event-card-heading">
          <span className="event-card-kicker">Access</span>
          <h2>Entry pass</h2>
        </div>
        <p><strong>Not issued yet.</strong> Passes are released shortly before the event.</p>
      </section>
    );
  }

  if (state === "error" || !pass) {
    return (
      <section className="application-pass" aria-live="polite">
        <div className="event-card-heading">
          <span className="event-card-kicker">Access</span>
          <h2>Entry pass</h2>
        </div>
        <p className="error-message" role="alert">
          We couldn’t load your pass. Try again.
        </p>
        <button className="button secondary" onClick={retryPass} type="button">
          Try again
        </button>
      </section>
    );
  }


  return (
    <section className="application-pass" aria-live="polite">
      <div className="event-card-heading">
        <span className="event-card-kicker">Access</span>
        <h2>Entry pass</h2>
      </div>
      <p>
        <strong>Issued.</strong>{" "}
        <time dateTime={pass.issuedAt}>{displayTimestamp(pass.issuedAt)}</time>.
      </p>
      <div className="attendee-pass-qr">
        <h3>Your QR code</h3>
        <p>Present this code at entry.</p>
        <PassQrCode value={pass.qrToken} />
      </div>
    </section>
  );
}
