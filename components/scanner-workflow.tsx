"use client";

import { useAuth } from "@clerk/nextjs";
import { AnimatePresence, motion } from "framer-motion";
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
    title: "Entry recorded",
    message: "This checkpoint redemption was recorded.",
  },
  already_exhausted: {
    title: "Redemption limit reached",
    message: "No additional redemption is available at this checkpoint.",
  },
  not_entitled: {
    title: "Not entitled",
    message: "This pass is not entitled to this checkpoint.",
  },
  outside_window: {
    title: "Checkpoint closed",
    message: "This checkpoint is not open for redemption right now.",
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

  const stopCamera = () => {
    cameraControlsRef.current?.stop();
    cameraControlsRef.current = null;
    setCameraActive(false);
  };

  useEffect(
    () => () => {
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
    if (!videoRef.current || cameraActive || busy) {
      return;
    }
    setCameraMessage(null);
    setCameraActive(true);
    try {
      const { BrowserQRCodeReader } = await import("@zxing/browser");
      const reader = new BrowserQRCodeReader();
      cameraControlsRef.current = await reader.decodeFromConstraints(
        { video: { facingMode: { ideal: "environment" } } },
        videoRef.current,
        (scanResult) => {
          if (!scanResult) {
            return;
          }
          handleCredentialChange(scanResult.getText());
          setCameraMessage("QR code captured. Look up the pass to verify it.");
          stopCamera();
        },
      );
    } catch {
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

    const nextQrToken = qrToken.trim();
    if (!nextQrToken) {
      setError({ message: "Enter or paste a QR code before looking it up.", retryable: false });
      return;
    }

    clearTransientState();
    setLookup(null);
    setRetryableRedemption(null);
    setPendingAction("lookup");

    try {
      const response = await client.lookupScannerPass({ qrToken: nextQrToken });
      if (response.pass.status === "revoked") {
        setResult({ kind: "lookup", outcome: "revoked_pass" });
        clearCredentialState();
      } else {
        setLookup({ qrToken: nextQrToken, response });
      }
    } catch (nextError) {
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
      setPendingAction(null);
    }
  };

  const redeem = async (request: ScannerRedemptionRequest) => {
    clearTransientState();
    setPendingAction("redeem");

    try {
      const response = await client.redeemScannerPass(request);
      setResult({ kind: "redemption", response });
      clearCredentialState();
    } catch (nextError) {
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
      setPendingAction(null);
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
      setError({ message: "Choose an active checkpoint before redeeming.", retryable: false });
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
  };

  if (!isLoaded) {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state" aria-busy="true" aria-live="polite">
          <h1>Preparing scanner</h1>
          <p>Checking session…</p>
        </section>
      </main>
    );
  }

  if (sessionExpired) {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state scanner-error-state" aria-live="assertive">
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
          <h1>Loading checkpoints</h1>
          <p>Loading active checkpoints…</p>
        </section>
      </main>
    );
  }

  if (checkpointLoadState === "error") {
    return (
      <main className="scanner-page">
        <section className="scanner-panel scanner-state scanner-error-state" aria-live="assertive">
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
          Applicant home
        </Link>
        <h1 id="scanner-heading">Verify entry pass</h1>

        <form className="scanner-form" onSubmit={handleLookup}>
          <div className="scanner-field">
            <label htmlFor="scanner-checkpoint">Checkpoint</label>
            <select
              disabled={busy}
              id="scanner-checkpoint"
              onChange={(event) => handleCheckpointChange(event.target.value)}
              value={checkpointId}
            >
              <option value="">Choose before redeeming</option>
              {checkpoints.map((checkpoint) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  {checkpoint.name}
                </option>
              ))}
            </select>
          </div>

          <div className="scanner-field">
            <label htmlFor="scanner-qr-token">QR code</label>
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
                  className="button secondary"
                  disabled={busy}
                  onClick={() => void handleStartCamera()}
                  type="button"
                >
                  Scan with camera
                </button>
              )}
            </div>
            {cameraMessage ? (
              <p className="scanner-help" aria-live="polite">{cameraMessage}</p>
            ) : null}
            <input
              autoCapitalize="none"
              autoComplete="off"
              disabled={busy}
              id="scanner-qr-token"
              inputMode="text"
              onChange={(event) => handleCredentialChange(event.target.value)}
              placeholder="Paste or type the QR code"
              required
              spellCheck={false}
              type="text"
              value={qrToken}
            />
          </div>

          <div className="scanner-actions">
            <button className="button primary" disabled={busy} type="submit">
              {pendingAction === "lookup" ? "Looking up…" : "Look up pass"}
            </button>
            {lookup ? (
              <button
                className="button secondary"
                disabled={busy || !activeCheckpoint}
                onClick={handleRedeem}
                type="button"
              >
                {pendingAction === "redeem"
                  ? "Recording…"
                  : activeCheckpoint
                    ? `Redeem at ${activeCheckpoint.name}`
                    : "Choose a checkpoint to redeem"}
              </button>
            ) : null}
          </div>
        </form>

        <AnimatePresence initial={false} mode="popLayout">
          {busy ? (
            <motion.section
              animate={{ opacity: 1, y: 0 }}
              className="scanner-progress"
              exit={{ opacity: 0, y: -8 }}
              initial={{ opacity: 0, y: 8 }}
              key="scanner-progress"
              aria-busy="true"
              aria-live="polite"
            >
              <p>{pendingAction === "lookup" ? "Verifying pass…" : "Recording redemption…"}</p>
            </motion.section>
          ) : null}

          {lookup ? (
            <motion.section
              animate={{ opacity: 1, scale: 1 }}
              className="scanner-result scanner-result-valid"
              exit={{ opacity: 0, scale: 0.98 }}
              initial={{ opacity: 0, scale: 0.98 }}
              key={`lookup-${lookup.qrToken}`}
              aria-live="polite"
            >
              <h2>Pass verified</h2>
              <p>
                <strong>{lookup.response.attendee.displayName}</strong> has an active pass.
                {activeCheckpoint
                  ? " Confirm the checkpoint to record entry."
                  : " Choose a checkpoint to record entry."}
              </p>
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
              Scan another pass
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
                  Retry redemption
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
  const copy = outcomeCopy[outcome];

  return (
    <motion.section
      animate={{ opacity: 1, scale: 1, y: 0 }}
      className={`scanner-result scanner-result-${outcome}`}
      initial={{ opacity: 0, scale: 0.98, y: 10 }}
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
