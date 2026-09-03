"use client";

import { useAuth } from "@clerk/nextjs";
import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { PassQrCode } from "@/components/pass-qr-code";
import {
  createApiClient,
  type AttendeePass,
  type OrganizerAttendeePass,
  type PassIssuance,
} from "@/lib/api";

type ActionState = "idle" | "issuing" | "revoking" | "reissuing";
type DestructiveAction = "revoke" | "reissue" | null;

type OrganizerPassActionsProps = {
  initialAttendeePass: OrganizerAttendeePass;
  rsvpConfirmed: boolean;
};

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

export function OrganizerPassActions({
  initialAttendeePass,
  rsvpConfirmed,
}: OrganizerPassActionsProps) {
  const { getToken } = useAuth();
  const router = useRouter();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [previousInitialPass, setPreviousInitialPass] = useState(initialAttendeePass.pass);
  const [pass, setPass] = useState<AttendeePass | null>(initialAttendeePass.pass);
  const [issuedQrToken, setIssuedQrToken] = useState<string | null>(null);
  const [actionState, setActionState] = useState<ActionState>("idle");
  const [pendingAction, setPendingAction] = useState<DestructiveAction>(null);
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");

  if (initialAttendeePass.pass !== previousInitialPass) {
    setPreviousInitialPass(initialAttendeePass.pass);
    setPass(initialAttendeePass.pass);
  }

  const recordIssuance = (issuance: PassIssuance, noticeMessage: string) => {
    setPass({
      id: issuance.id,
      attendeeId: issuance.attendeeId,
      displayName: issuance.displayName,
      status: issuance.status,
      issuedAt: issuance.issuedAt,
      ...(issuance.revokedAt ? { revokedAt: issuance.revokedAt } : {}),
    });
    setIssuedQrToken(issuance.qrToken);
    setNotice(noticeMessage);
  };

  const issuePass = async () => {
    if (!rsvpConfirmed || actionState !== "idle") return;
    setActionState("issuing");
    setActionError("");
    setNotice("");
    setPendingAction(null);
    setIssuedQrToken(null);

    try {
      const issuance = await client.issueAttendeePass(initialAttendeePass.attendeeId);
      recordIssuance(
        issuance,
        "Pass issued. The pass-link message has been queued for the attendee.",
      );
      router.refresh();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Unable to issue the pass.");
    } finally {
      setActionState("idle");
    }
  };

  const revokePass = async () => {
    if (!pass || pass.status !== "active") {
      return;
    }

    setActionState("revoking");
    setActionError("");
    setNotice("");

    try {
      const revokedPass = await client.revokeAttendeePass(pass.id);
      setPass(revokedPass);
      setIssuedQrToken(null);
      setPendingAction(null);
      setNotice("Pass revoked. Its QR credential can no longer be used.");
      router.refresh();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Unable to revoke the pass.");
    } finally {
      setActionState("idle");
    }
  };

  const reissuePass = async () => {
    if (!rsvpConfirmed || !pass || pass.status !== "active") {
      return;
    }

    setActionState("reissuing");
    setActionError("");
    setNotice("");
    setIssuedQrToken(null);

    try {
      const issuance = await client.reissueAttendeePass(pass.id);
      recordIssuance(
        issuance,
        "Pass reissued. The previous QR credential has been invalidated and a new pass-link message has been queued.",
      );
      setPendingAction(null);
      router.refresh();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Unable to reissue the pass.");
    } finally {
      setActionState("idle");
    }
  };

  const submitPendingAction = () => {
    if (pendingAction === "revoke") {
      void revokePass();
    } else if (pendingAction === "reissue") {
      void reissuePass();
    }
  };

  const busy = actionState !== "idle";
  const canIssue = !pass || pass.status === "revoked";

  return (
    <div className="pass-workflow">
      <div className="pass-current">
        <h3>{pass?.status === "active" ? "Active pass" : "Pass status"}</h3>
        {pass ? (
          <p className="staff-muted">
            {pass.status === "active" ? "Active" : "Revoked"} · Issued{" "}
            <time dateTime={pass.issuedAt}>{displayTimestamp(pass.issuedAt)}</time>
            {pass.revokedAt ? (
              <>
                {" "}· Revoked{" "}
                <time dateTime={pass.revokedAt}>{displayTimestamp(pass.revokedAt)}</time>
              </>
            ) : null}
          </p>
        ) : (
          <p className="staff-muted">No pass has been issued for this attendee.</p>
        )}
      </div>

      <p className="staff-muted">
        {rsvpConfirmed
          ? "RSVP confirmed. Release the pass a few days before the event when you are ready; confirming an RSVP does not issue a pass automatically."
          : "Pass release is unavailable until the attendee confirms their RSVP."}
      </p>

      {pendingAction ? (
        <div className="pass-action-confirmation" aria-live="polite">
          <p>
            {pendingAction === "revoke"
              ? "Revoke this pass? Its existing QR credential will no longer work."
              : "Reissue this pass? Its existing QR credential will be invalidated immediately."}
          </p>
          <div className="staff-actions">
            <button
              className="button primary"
              disabled={busy || (pendingAction === "reissue" && !rsvpConfirmed)}
              onClick={submitPendingAction}
              type="button"
            >
              {pendingAction === "revoke"
                ? actionState === "revoking"
                  ? "Revoking…"
                  : "Confirm revoke"
                : actionState === "reissuing"
                  ? "Reissuing…"
                  : "Confirm reissue"}
            </button>
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => setPendingAction(null)}
              type="button"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="staff-actions">
          {canIssue ? (
            <button
              className="button primary"
              disabled={busy || !rsvpConfirmed}
              onClick={() => void issuePass()}
              type="button"
            >
              {actionState === "issuing" ? "Issuing…" : "Issue pass"}
            </button>
          ) : null}
          {pass?.status === "active" ? (
            <>
              <button
                className="button secondary"
                disabled={busy || !rsvpConfirmed}
                onClick={() => setPendingAction("reissue")}
                type="button"
              >
                Reissue pass
              </button>
              <button
                className="button secondary"
                disabled={busy}
                onClick={() => setPendingAction("revoke")}
                type="button"
              >
                Revoke pass
              </button>
            </>
          ) : null}
        </div>
      )}

      {issuedQrToken ? (
        <div className="pass-issuance-confirmation" aria-live="polite">
          <h3>New QR pass</h3>
          <p>
            This QR code is shown only for this issuance confirmation. Do not capture or
            share it outside the attendee&apos;s approved delivery channel.
          </p>
          <PassQrCode value={issuedQrToken} />
          <button
            className="button secondary"
            onClick={() => setIssuedQrToken(null)}
            type="button"
          >
            Close confirmation
          </button>
        </div>
      ) : null}

      {notice ? (
        <p className="application-notice" role="status">
          {notice}
        </p>
      ) : null}
      {actionError ? (
        <p className="error-message" role="alert">
          {actionError}
        </p>
      ) : null}
    </div>
  );
}
