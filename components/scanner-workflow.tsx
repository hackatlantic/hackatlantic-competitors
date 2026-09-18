"use client";

import { useAuth } from "@clerk/nextjs";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  ApiError,
  createApiClient,
  type ScannerCheckpoint,
  type ScannerLookupResponse,
  type ScannerRedemptionOutcome,
  type ScannerRedemptionRequest,
  type ScannerRedemptionResponse,
} from "@/lib/api";

type CheckpointLoadState = "loading" | "ready" | "empty" | "error";
type PendingAction = "lookup" | "redeem" | null;

type LookupState = {
  qrToken: string;
  response: ScannerLookupResponse;
};

type RetryableRedemption = ScannerRedemptionRequest;

type ScannerResult =
  | {
      kind: "lookup";
      outcome: "invalid_pass" | "revoked_pass";
    }
  | {
      kind: "redemption";
      response: ScannerRedemptionResponse;
    };

type ScannerError = {
  message: string;
  retryable: boolean;
};

type CameraControls = {
  stop: () => void;
};

const outcomeCopy: Record<
  ScannerRedemptionOutcome,
  { title: string; message: string }
> = {
  redeemed: {
    title: "Checked in",
    message: "Check-in recorded. You can scan the next pass.",
  },
  already_exhausted: {
    title: "Already checked in",
    message: "This pass has used its allowance at this scan point. Ask an organizer if help is needed.",
  },
  not_entitled: {
    title: "Not available for this pass",
    message: "This attendee does not have access at this scan point.",
  },
  outside_window: {
    title: "Scan point closed",
    message: "Check-in is not open here right now.",
  },
  invalid_pass: {
    title: "Pass not valid",
    message: "This QR code does not identify a valid pass.",
  },
  revoked_pass: {
    title: "Pass revoked",
    message: "This pass is no longer active.",
  },
};

function getScannerOutcome(error: unknown): "invalid_pass" | "revoked_pass" | null {
  if (!(error instanceof ApiError)) {
    return null;
  }

  switch (error.body.code) {
    case "invalid_pass":
    case "pass_not_found":
      return "invalid_pass";
    case "revoked_pass":
      return "revoked_pass";
    default:
      return null;
  }
}

function getScannerError(error: unknown): ScannerError {
  if (error instanceof ApiError) {
    if (error.status === 401) {
      return {
        message: "Your session has ended. Sign in again before scanning.",
        retryable: false,
      };
    }

    if (error.status === 403) {
      return {
        message: "Scanner access is required to use this screen.",
        retryable: false,
      };
    }

    if (error.status === 409 && error.body.code === "idempotency_conflict") {
      return {
        message: "This scan cannot be retried. Start a new scan instead.",
        retryable: false,
      };
    }

    if (error.status === 429) {
      return {
        message: "Scanning is temporarily limited. Wait a moment, then retry.",
        retryable: true,
      };
    }

    if (error.status >= 500) {
      return {
        message: "The scanner service is temporarily unavailable. Try again.",
        retryable: true,
      };
    }
  }

  return {
    message: "The scan could not be completed. Check the connection and try again.",
    retryable: true,
  };
}

function isScannerAccessError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 403;
}

function createIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }

  throw new Error("This browser cannot securely create a scan identifier.");
}



export function ScannerWorkflow() {
  const { userId, isLoaded } = useAuth();
  if (!isLoaded) return <main className="scanner-page"><section className="scanner-panel"><p role="status">Preparing scanner…</p></section></main>;
  if (!userId) return <main className="scanner-page"><section className="scanner-panel"><h1>Sign in to scan</h1><Link href="/">Return to sign in</Link></section></main>;
  return <ScannerSession key={userId} />;
}

function ScannerSession() {
  const reducedMotion = useReducedMotion();
  const { getToken, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [checkpoints, setCheckpoints] = useState<ScannerCheckpoint[]>([]);
  const [checkpointLoadState, setCheckpointLoadState] =
    useState<CheckpointLoadState>("loading");
  const [accessDenied, setAccessDenied] = useState(false);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [checkpointId, setCheckpointId] = useState("");
  const [qrToken, setQrToken] = useState("");
  const [lookup, setLookup] = useState<LookupState | null>(null);
  const [result, setResult] = useState<ScannerResult | null>(null);
  const [error, setError] = useState<ScannerError | null>(null);
  const [pendingAction, setPendingAction] = useState<PendingAction>(null);
  const [retryableRedemption, setRetryableRedemption] =
    useState<RetryableRedemption | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const [cameraActive, setCameraActive] = useState(false);
  const [cameraMessage, setCameraMessage] = useState<string | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const cameraControlsRef = useRef<CameraControls | null>(null);
  const cameraSequence = useRef(0);
  const requestSequence = useRef(0);
  const requestBusy = useRef(false);
  const confirmRef = useRef<HTMLButtonElement | null>(null);

  const stopCamera = () => {
    cameraSequence.current++;
    cameraControlsRef.current?.stop();
    cameraControlsRef.current = null;
    setCameraActive(false);
  };

  useEffect(
    () => () => {
      cameraSequence.current++;
      requestSequence.current++;
      cameraControlsRef.current?.stop();
    },
    [],
  );

  useEffect(() => {
    if (!isLoaded) {
      return;
    }

    let cancelled = false;

    const loadCheckpoints = async () => {
      setCheckpointLoadState("loading");
      setError(null);

      try {
        const nextCheckpoints: ScannerCheckpoint[] = [];
        let nextCursor: string | undefined;

        do {
          const page = await client.listScannerCheckpoints(nextCursor);
          nextCheckpoints.push(...page.items);
          nextCursor = page.nextCursor ?? undefined;
        } while (nextCursor);

        if (!cancelled) {
          setCheckpoints(nextCheckpoints);
          setCheckpointId((current) => nextCheckpoints.some((point) => point.id === current)
            ? current : nextCheckpoints.length === 1 ? nextCheckpoints[0].id : "");
          setCheckpointLoadState(nextCheckpoints.length === 0 ? "empty" : "ready");
        }
      } catch (nextError) {
        if (!cancelled) {
          if (nextError instanceof ApiError && nextError.status === 401) {
            setSessionExpired(true);
          } else if (isScannerAccessError(nextError)) {
            setAccessDenied(true);
          } else {
            setCheckpointLoadState("error");
          }
        }
      }
    };

    void loadCheckpoints();

    return () => {
      cancelled = true;
    };
  }, [client, isLoaded, reloadVersion]);

  const activeCheckpoint =
    checkpoints.find((checkpoint) => checkpoint.id === checkpointId) ?? null;
  const busy = pendingAction !== null;

  const clearTransientState = () => {
    setResult(null);
    setError(null);
  };

  const clearCredentialState = () => {
    setQrToken("");
    setLookup(null);
    setRetryableRedemption(null);
  };

  const handleCredentialChange = (value: string) => {
    setQrToken(value);
    setLookup(null);
    setRetryableRedemption(null);
    clearTransientState();
  };

  const handleStartCamera = async () => {
    if (!videoRef.current || cameraActive || requestBusy.current) {
      return;
    }
    setCameraMessage(null);
    setCameraActive(true);
    const sequence = ++cameraSequence.current;
    let captured = false;
    try {
      const { BrowserQRCodeReader } = await import("@zxing/browser");
      if (sequence !== cameraSequence.current || !videoRef.current) return;
      const reader = new BrowserQRCodeReader();
      const controls = await reader.decodeFromConstraints(
        { video: { facingMode: { ideal: "environment" } } },
        videoRef.current,
        (scanResult) => {
          if (!scanResult || captured || sequence !== cameraSequence.current) {
            return;
          }
          captured = true;
          const token = scanResult.getText();
          handleCredentialChange(token);
          setCameraMessage(null);
          stopCamera();
          void lookupPass(token);
        },
      );
      if (sequence !== cameraSequence.current) controls.stop();
      else cameraControlsRef.current = controls;
    } catch {
      if (sequence !== cameraSequence.current) return;
      stopCamera();
      setCameraMessage(
        "The camera could not be opened. Allow camera access or enter the QR code manually.",
      );
    }
  };

  const handleCheckpointChange = (value: string) => {
    setCheckpointId(value);
    setRetryableRedemption(null);
    clearTransientState();
  };

  const handleLookup = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    await lookupPass(qrToken);
  };

  const lookupPass = async (token: string) => {
    if (requestBusy.current) return;
    const nextQrToken = token.trim();
    if (!nextQrToken) {
      setError({ message: "Enter or paste a QR code before looking it up.", retryable: false });
      return;
    }

    stopCamera();
    requestBusy.current = true;
    const sequence = ++requestSequence.current;
    clearTransientState();
    setLookup(null);
    setRetryableRedemption(null);
    setPendingAction("lookup");

    try {
      const response = await client.lookupScannerPass({ qrToken: nextQrToken });
      if (sequence !== requestSequence.current) return;
      if (response.pass.status === "revoked") {
        setResult({ kind: "lookup", outcome: "revoked_pass" });
        clearCredentialState();
      } else {
        setLookup({ qrToken: nextQrToken, response });
        window.requestAnimationFrame(() => confirmRef.current?.focus());
      }
    } catch (nextError) {
      if (sequence !== requestSequence.current) return;
      const outcome = getScannerOutcome(nextError);
      if (outcome) {
        setResult({ kind: "lookup", outcome });
        clearCredentialState();
      } else if (nextError instanceof ApiError && nextError.status === 401) {
        setSessionExpired(true);
      } else if (isScannerAccessError(nextError)) {
        setAccessDenied(true);
      } else {
        setError(getScannerError(nextError));
      }
    } finally {
      if (sequence === requestSequence.current) {
        requestBusy.current = false;
        setPendingAction(null);
      }
    }
  };

  const redeem = async (request: ScannerRedemptionRequest) => {
    if (requestBusy.current) return;
    requestBusy.current = true;
    const sequence = ++requestSequence.current;
    clearTransientState();
    setPendingAction("redeem");

    try {
      const response = await client.redeemScannerPass(request);
      if (sequence !== requestSequence.current) return;
      setResult({ kind: "redemption", response });
      clearCredentialState();
    } catch (nextError) {
      if (sequence !== requestSequence.current) return;
      const outcome = getScannerOutcome(nextError);
      if (outcome) {
        setResult({
          kind: "redemption",
          response: { outcome },
        });
        clearCredentialState();
      } else if (nextError instanceof ApiError && nextError.status === 401) {
        setSessionExpired(true);
      } else if (isScannerAccessError(nextError)) {
        setAccessDenied(true);
      } else {
        const nextScannerError = getScannerError(nextError);
        setError(nextScannerError);
        setRetryableRedemption(nextScannerError.retryable ? request : null);
      }
    } finally {
      if (sequence === requestSequence.current) {
        requestBusy.current = false;
        setPendingAction(null);
      }
    }
  };

  const handleRedeem = () => {
    const nextQrToken = qrToken.trim();
    if (!lookup || lookup.qrToken !== nextQrToken) {
      setError({
        message: "Look up this QR code before recording a redemption.",
        retryable: false,
      });
      return;
    }

    if (!activeCheckpoint) {
      setError({ message: "Choose a scan point before checking in.", retryable: false });
      return;
    }

    const retryMatchesCurrentPayload =
      retryableRedemption?.qrToken === nextQrToken &&
      retryableRedemption.checkpointId === activeCheckpoint.id;
    let request: ScannerRedemptionRequest;

    try {
      if (retryMatchesCurrentPayload && retryableRedemption) {
        request = retryableRedemption;
      } else {
        request = {
          qrToken: nextQrToken,
          checkpointId: activeCheckpoint.id,
          idempotencyKey: createIdempotencyKey(),
        };
      }
    } catch (nextError) {
      setError(getScannerError(nextError));
      return;
    }

    void redeem(request);
  };

  const handleRetryRedemption = () => {
    if (retryableRedemption) {
      void redeem(retryableRedemption);
    }
  };

  const handleScanAnother = () => {
    clearCredentialState();
    clearTransientState();
    void handleStartCamera();
  };

  if (!isLoaded) {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state" aria-busy="true" aria-live="polite">
          <p className="eyebrow">Scanner</p>
          <h1>Preparing scanner</h1>
          <p>Checking your secure session…</p>
        </section>
      </main>
    );
  }

  if (sessionExpired) {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state scanner-error-state" aria-live="assertive">
          <p className="eyebrow">Scanner</p>
          <h1>Session ended</h1>
          <p role="alert">Sign in again before scanning another pass.</p>
          <Link className="button secondary scanner-session-link" href="/">
            Sign in again
          </Link>
        </section>
      </main>
    );
  }

  if (accessDenied) {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state scanner-error-state" aria-live="assertive">
          <p className="eyebrow">Scanner</p>
          <h1>Scanner access required</h1>
          <p role="alert">Your account is not authorized to scan entry passes.</p>
        </section>
      </main>
    );
  }

  if (checkpointLoadState === "loading") {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state" aria-busy="true" aria-live="polite">
          <p className="eyebrow">Scanner</p>
          <h1>Loading checkpoints</h1>
          <p>Getting the active checkpoints available to your scanner account…</p>
        </section>
      </main>
    );
  }

  if (checkpointLoadState === "error") {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state scanner-error-state" aria-live="assertive">
          <p className="eyebrow">Scanner</p>
          <h1>Scanner unavailable</h1>
          <p role="alert">Active checkpoints could not be loaded. Check the connection and try again.</p>
          <button
            className="button secondary"
            onClick={() => setReloadVersion((version) => version + 1)}
            type="button"
          >
            Try again
          </button>
        </section>
      </main>
    );
  }

  if (checkpointLoadState === "empty") {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state" aria-live="polite">
          <p className="eyebrow">Scanner</p>
          <h1>No active checkpoints</h1>
          <p>There are no checkpoints available for scanning right now.</p>
        </section>
      </main>
    );
  }

  return (
    <main className="scanner-page">
      <section className="scanner-panel" aria-labelledby="scanner-heading">
        <Link className="staff-link" href="/">
          ← Back to dashboard
        </Link>
        <h1 id="scanner-heading">Check in attendees</h1>
        <p className="scanner-summary">
          Scan a ticket, check the name, then confirm check-in.
        </p>

        <form className="scanner-form" hidden={Boolean(lookup) || Boolean(result)} onSubmit={handleLookup}>
          <div className="scanner-field">
            <label htmlFor="scanner-checkpoint">Scan point</label>
            <select
              disabled={busy}
              id="scanner-checkpoint"
              onChange={(event) => handleCheckpointChange(event.target.value)}
              value={checkpointId}
            >
              <option value="">Choose where you’re scanning</option>
              {checkpoints.map((checkpoint) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  {checkpoint.name}
                </option>
              ))}
            </select>
          </div>

          <div className="scanner-field">
            <div className={`scanner-camera${cameraActive ? " scanner-camera-active" : ""}`}>
              <video
                aria-label="QR code camera preview"
                muted
                playsInline
                ref={videoRef}
              />
            </div>
            <div className="scanner-actions">
              {cameraActive ? (
                <button className="button secondary" onClick={stopCamera} type="button">
                  Stop camera
                </button>
              ) : (
                <button
                  className="button primary"
                  disabled={busy || Boolean(lookup) || Boolean(result) || !activeCheckpoint}
                  onClick={() => void handleStartCamera()}
                  type="button"
                >
                  Scan ticket
                </button>
              )}
            </div>
            {cameraMessage ? (
              <p className="scanner-help" aria-live="polite">{cameraMessage}</p>
            ) : null}
            <details className="scanner-manual">
            <summary>Camera not working? Enter a code</summary>
            <label htmlFor="scanner-qr-token">QR code</label>
            <input
              aria-describedby="scanner-qr-help"
              autoCapitalize="none"
              autoComplete="off"
              disabled={busy}
              id="scanner-qr-token"
              inputMode="text"
              onChange={(event) => handleCredentialChange(event.target.value)}
              placeholder="Paste or type the QR code"
              spellCheck={false}
              type="text"
              value={qrToken}
            />
            <p id="scanner-qr-help" className="scanner-help">
              Paste the code from a QR reader. This is not the attendee’s email.
            </p>
            <button className="button secondary" disabled={busy || !activeCheckpoint} type="submit">
              {pendingAction === "lookup" ? "Verifying…" : "Verify code"}
            </button>
            </details>
          </div>

        </form>

        <AnimatePresence initial={false} mode="popLayout">
          {busy ? (
            <motion.section
              animate={{ opacity: 1, y: 0 }}
              className="scanner-progress"
              exit={{ opacity: 0, y: -8 }}
              initial={reducedMotion ? false : { opacity: 0, y: 8 }}
              key="scanner-progress"
              aria-busy="true"
              aria-live="polite"
            >
              <p>{pendingAction === "lookup" ? "Verifying pass…" : "Recording check-in…"}</p>
            </motion.section>
          ) : null}

          {lookup ? (
            <motion.section
              animate={{ opacity: 1, scale: 1 }}
              className="scanner-result scanner-result-valid"
              exit={{ opacity: 0, scale: 0.98 }}
              initial={reducedMotion ? false : { opacity: 0, scale: 0.98 }}
              key={`lookup-${lookup.qrToken}`}
              aria-live="polite"
            >
              <h2>Ready to check in</h2>
              <p>
                <strong>{lookup.response.attendee.displayName}</strong> · {activeCheckpoint?.name || "Choose a scan point"}.
                {activeCheckpoint
                  ? " Check the name, then tap Check in. Entry has not been recorded yet."
                  : " Choose a scan point to continue."}
              </p>
              <div className="scanner-actions">
              <button
                ref={confirmRef}
                className="button primary"
                disabled={busy || !activeCheckpoint}
                onClick={handleRedeem}
                type="button"
              >
                {pendingAction === "redeem"
                  ? "Checking in…"
                  : activeCheckpoint
                    ? `Check in at ${activeCheckpoint.name}`
                    : "Choose a scan point"}
              </button>
                <button className="button secondary" type="button" disabled={busy} onClick={handleScanAnother}>Cancel and scan another</button>
              </div>
            </motion.section>
          ) : null}
        </AnimatePresence>

        {result?.kind === "lookup" ? (
          <ScanOutcome outcome={result.outcome} />
        ) : null}

        {result?.kind === "redemption" ? (
          <ScanOutcome outcome={result.response.outcome} attendee={result.response.attendee} />
        ) : null}

        {result ? (
          <div className="scanner-actions">
            <button className="button secondary" onClick={handleScanAnother} type="button">
              Scan next attendee
            </button>
          </div>
        ) : null}

        {error ? (
          <section className="scanner-error" aria-live="assertive">
            <p role="alert">{error.message}</p>
            <div className="scanner-actions">
              {retryableRedemption ? (
                <button
                  className="button secondary"
                  disabled={busy}
                  onClick={handleRetryRedemption}
                  type="button"
                >
                  Retry check-in
                </button>
              ) : null}
            </div>
          </section>
        ) : null}
      </section>
    </main>
  );
}

type ScanOutcomeProps = {
  outcome: ScannerRedemptionOutcome;
  attendee?: ScannerRedemptionResponse["attendee"];
};

function ScanOutcome({ outcome, attendee }: ScanOutcomeProps) {
  const reducedMotion = useReducedMotion();
  const copy = outcomeCopy[outcome];

  return (
    <motion.section
      animate={{ opacity: 1, scale: 1, y: 0 }}
      className={`scanner-result scanner-result-${outcome}`}
      initial={reducedMotion ? false : { opacity: 0, scale: 0.98, y: 10 }}
      key={outcome}
      transition={{ type: "spring", stiffness: 360, damping: 30 }}
      aria-live={outcome === "redeemed" ? "polite" : "assertive"}
    >
      <h2>{copy.title}</h2>
      <p>
        {attendee?.displayName ? <strong>{attendee.displayName}. </strong> : null}
        {copy.message}
      </p>
    </motion.section>
  );
}
