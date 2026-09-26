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
    title: "Recorded",
    message: "You can scan the next ticket.",
  },
  already_exhausted: {
    title: "Already recorded",
    message: "This pass has already used its allowance for this selection. Ask an organizer if help is needed.",
  },
  not_entitled: {
    title: "Not available for this pass",
    message: "This pass cannot be used for this selection.",
  },
  outside_window: {
    title: "Scanning closed",
    message: "Scanning is not open for this selection right now.",
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
            ? current : nextCheckpoints.length === 0 ? "verify" : nextCheckpoints.length === 1 ? nextCheckpoints[0].id : "");
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
  const verificationOnly = checkpointId === "verify" || Boolean(lookup?.response.pass.kind);
  const canScan = checkpointId === "verify" || Boolean(activeCheckpoint);
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
    stopCamera();
    clearCredentialState();
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

    if (!activeCheckpoint || verificationOnly) {
      setError({ message: "Choose what you’re scanning for first.", retryable: false });
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
          <h1>Loading scanner</h1>
          <p>Getting your scanning options…</p>
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
          <p role="alert">Scanning options could not be loaded. Check the connection and try again.</p>
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

  return (
    <main className="scanner-page">
      <section className="scanner-panel" aria-labelledby="scanner-heading">
        <Link className="staff-link" href="/">
          ← Back to dashboard
        </Link>
        <h1 id="scanner-heading">Scan tickets</h1>
        <Link className="staff-link" href="/event-pass">My event pass</Link>
        <p className="scanner-summary">
          Choose check-in or a meal to record attendance. For returning guests, choose Verify pass / re-entry; it only checks validity.
        </p>

        <form className="scanner-form" hidden={Boolean(lookup) || Boolean(result)} onSubmit={handleLookup}>
          <div className="scanner-field">
            <label htmlFor="scanner-checkpoint">What are you scanning for?</label>
            <select
              disabled={busy}
              id="scanner-checkpoint"
              onChange={(event) => handleCheckpointChange(event.target.value)}
              value={checkpointId}
            >
              <option value="">Choose entrance or a meal</option>
              <option value="verify">Verify pass / re-entry</option>
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
                  disabled={busy || Boolean(lookup) || Boolean(result) || !canScan}
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
            <button className="button secondary" disabled={busy || !canScan} type="submit">
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
              <p>{pendingAction === "lookup" ? "Verifying pass…" : "Recording scan…"}</p>
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
              <h2>{verificationOnly ? "Valid entry pass" : "Ready to confirm"}</h2>
              {verificationOnly ? <p><strong>{lookup.response.attendee.displayName}</strong> · {lookup.response.pass.kind === "organizer" ? "Organizer" : lookup.response.pass.kind === "volunteer" ? "Volunteer" : "Attendee"}. Check the name before admitting. No check-in or meal has been recorded.</p> :
              <p>
                <strong>{lookup.response.attendee.displayName}</strong> · {activeCheckpoint?.name || "Choose what to scan for"}.
                {activeCheckpoint
                  ? " Check the name, then confirm. This scan hasn’t been recorded yet."
                  : " Choose what you’re scanning for to continue."}
              </p>}
              <div className="scanner-actions">
              {!verificationOnly && <button
                ref={confirmRef}
                className="button primary"
                disabled={busy || !activeCheckpoint}
                onClick={handleRedeem}
                type="button"
              >
                {pendingAction === "redeem"
                  ? "Recording…"
                  : activeCheckpoint
                    ? `Confirm ${activeCheckpoint.name}`
                    : "Choose what to scan for"}
              </button>}
                <button className="button secondary" type="button" disabled={busy} onClick={handleScanAnother}>{verificationOnly ? "Scan next pass" : "Cancel and scan another"}</button>
              </div>
            </motion.section>
          ) : null}
        </AnimatePresence>

        {result?.kind === "lookup" ? (
          <ScanOutcome outcome={result.outcome} />
        ) : null}

        {result?.kind === "redemption" ? (
          <ScanOutcome outcome={result.response.outcome} attendee={result.response.attendee} checkpointName={result.response.checkpoint?.name ?? activeCheckpoint?.name} />
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
                  Retry scan
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
  checkpointName?: string;
};

function ScanOutcome({ outcome, attendee, checkpointName }: ScanOutcomeProps) {
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
      {checkpointName ? <p className="scanner-result-context">{checkpointName}</p> : null}
      <p>
        {attendee?.displayName ? <strong>{attendee.displayName}. </strong> : null}
        {copy.message}
      </p>
    </motion.section>
  );
}
