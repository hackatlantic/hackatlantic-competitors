"use client";

import { useAuth } from "@clerk/nextjs";
import { type FormEvent, useMemo, useState } from "react";
import { createApiClient, type ScannerAccessUser } from "@/lib/api";
import { ApplicationButton } from "@/components/application-motion";

type ActionState = "idle" | "saving";

export function ReviewerRoleForm() {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [userId, setUserId] = useState("");
  const [actionState, setActionState] = useState<ActionState>("idle");
  const [message, setMessage] = useState("");

  const grantReviewerRole = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setActionState("saving");
    setMessage("");

    try {
      await client.grantReviewerRole(userId.trim());
      setMessage("Reviewer role granted.");
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "Unable to grant the reviewer role.",
      );
    } finally {
      setActionState("idle");
    }
  };

  return (
    <form className="reviewer-role-form" onSubmit={grantReviewerRole}>
      <div>
        <label htmlFor="reviewer-user-id">Local user ID</label>
        <input
          autoComplete="off"
          id="reviewer-user-id"
          onChange={(event) => setUserId(event.target.value)}
          placeholder="Reviewer UUID"
          required
          spellCheck={false}
          value={userId}
        />
      </div>
      <button className="button primary" disabled={actionState === "saving"} type="submit">
        {actionState === "saving" ? "Granting…" : "Grant reviewer role"}
      </button>
      {message ? (
        <p
          className={message === "Reviewer role granted." ? "application-notice" : "error-message"}
          role={message === "Reviewer role granted." ? "status" : "alert"}
        >
          {message}
        </p>
      ) : null}
    </form>
  );
}

export function ScannerRoleForm() {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [email, setEmail] = useState("");
  const [target, setTarget] = useState<ScannerAccessUser | null>(null);
  const [actionState, setActionState] = useState<"idle" | "searching" | "granting" | "revoking">("idle");
  const [message, setMessage] = useState("");
  const success = message === "Scanner role granted." || message === "Scanner role revoked.";

  const findVolunteer = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (actionState !== "idle") return;
    setTarget(null);
    setMessage("");
    setActionState("searching");
    try {
      setTarget(await client.lookupScannerUser(email.trim()));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to find this account. Try again.");
    } finally {
      setActionState("idle");
    }
  };

  const changeScannerRole = async (action: "grant" | "revoke") => {
    if (!target?.canManage || actionState !== "idle") return;
    setActionState(action === "grant" ? "granting" : "revoking");
    setMessage("");
    try {
      if (action === "grant") {
        await client.grantScannerRole(target.id);
        setMessage("Scanner role granted.");
      } else {
        await client.revokeScannerRole(target.id);
        setMessage("Scanner role revoked.");
      }
      setTarget({ ...target, scannerAccess: action === "grant" });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to change scanner access.");
    } finally {
      setActionState("idle");
    }
  };

  return (
    <div className="scanner-access-manager">
      <form className="reviewer-role-form" onSubmit={findVolunteer}>
        <div className="operations-field">
          <label htmlFor="scanner-email">Volunteer email</label>
          <p id="scanner-email-help">Use their verified primary email. They need to sign up and open HackAtlantic once first.</p>
          <input
            aria-describedby="scanner-email-help"
            autoComplete="off"
            autoCapitalize="none"
            disabled={actionState !== "idle"}
            id="scanner-email"
            onChange={(event) => {
              setEmail(event.target.value);
              setTarget(null);
              setMessage("");
            }}
            placeholder="volunteer@example.com"
            required
            spellCheck={false}
            type="email"
            value={email}
          />
        </div>
        <ApplicationButton className="primary" disabled={actionState !== "idle"} pending={actionState === "searching"} type="submit">
          {actionState === "searching" ? "Searching…" : "Find volunteer"}
        </ApplicationButton>
      </form>
      <div aria-live="polite">
        {target ? (
          <section className="scanner-access-result" aria-label="Volunteer account">
            <h2>{target.displayName || "Volunteer account"}</h2>
            <p className="scanner-access-email">{target.email}</p>
            <p>{target.scannerAccess ? "Scanner access is active." : "No scanner access yet."}</p>
            {target.canManage ? (
              <ApplicationButton
                className={target.scannerAccess ? "secondary" : "primary"}
                disabled={actionState !== "idle"}
                pending={actionState === "granting" || actionState === "revoking"}
                onClick={() => void changeScannerRole(target.scannerAccess ? "revoke" : "grant")}
                type="button"
              >
                {actionState === "granting" ? "Granting…" : actionState === "revoking" ? "Revoking…" : target.scannerAccess ? "Revoke scanner role" : "Grant scanner role"}
              </ApplicationButton>
            ) : (
              <p>Admin accounts already have scanner access. Admin and self-service access changes are not available here.</p>
            )}
          </section>
        ) : null}
      </div>
      {message ? (
        <p className={success ? "application-notice" : "error-message"} role={success ? "status" : "alert"}>
          {message}
        </p>
      ) : null}
    </div>
  );
}

type ReviewerAssignmentFormProps = {
  applicationId: string;
};

export function ReviewerAssignmentForm({
  applicationId,
}: ReviewerAssignmentFormProps) {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [reviewerUserId, setReviewerUserId] = useState("");
  const [actionState, setActionState] = useState<ActionState>("idle");
  const [message, setMessage] = useState("");

  const assignReviewer = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setActionState("saving");
    setMessage("");

    try {
      await client.assignReviewer(applicationId, {
        reviewerUserId: reviewerUserId.trim(),
      });
      setMessage("Reviewer assignment created.");
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "Unable to create the reviewer assignment.",
      );
    } finally {
      setActionState("idle");
    }
  };

  return (
    <form className="assignment-form" onSubmit={assignReviewer}>
      <div>
        <label htmlFor="assigned-reviewer-user-id">Reviewer local user ID</label>
        <input
          autoComplete="off"
          id="assigned-reviewer-user-id"
          onChange={(event) => setReviewerUserId(event.target.value)}
          placeholder="Reviewer UUID"
          required
          spellCheck={false}
          value={reviewerUserId}
        />
      </div>
      <button className="button primary" disabled={actionState === "saving"} type="submit">
        {actionState === "saving" ? "Assigning…" : "Assign reviewer"}
      </button>
      {message ? (
        <p
          className={
            message === "Reviewer assignment created."
              ? "application-notice"
              : "error-message"
          }
          role={message === "Reviewer assignment created." ? "status" : "alert"}
        >
          {message}
        </p>
      ) : null}
    </form>
  );
}
