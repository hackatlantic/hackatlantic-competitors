"use client";

import { useAuth } from "@clerk/nextjs";
import { type FormEvent, useMemo, useState } from "react";
import { createApiClient } from "@/lib/api";

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
  const [userId, setUserId] = useState("");
  const [actionState, setActionState] = useState<"idle" | "granting" | "revoking">("idle");
  const [message, setMessage] = useState("");
  const success = message === "Scanner role granted." || message === "Scanner role revoked.";

  const changeScannerRole = async (action: "grant" | "revoke") => {
    const targetUserId = userId.trim();
    if (!targetUserId) {
      setMessage("Enter a local user ID.");
      return;
    }
    setActionState(action === "grant" ? "granting" : "revoking");
    setMessage("");
    try {
      if (action === "grant") {
        await client.grantScannerRole(targetUserId);
        setMessage("Scanner role granted.");
      } else {
        await client.revokeScannerRole(targetUserId);
        setMessage("Scanner role revoked.");
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to change scanner access.");
    } finally {
      setActionState("idle");
    }
  };

  return (
    <div className="reviewer-role-form">
      <div>
        <label htmlFor="scanner-user-id">Local user ID</label>
        <input
          autoComplete="off"
          id="scanner-user-id"
          onChange={(event) => setUserId(event.target.value)}
          placeholder="Scanner UUID"
          required
          spellCheck={false}
          value={userId}
        />
      </div>
      <div className="staff-actions">
        <button
          className="button primary"
          disabled={actionState !== "idle"}
          onClick={() => void changeScannerRole("grant")}
          type="button"
        >
          {actionState === "granting" ? "Granting…" : "Grant scanner role"}
        </button>
        <button
          className="button secondary"
          disabled={actionState !== "idle"}
          onClick={() => void changeScannerRole("revoke")}
          type="button"
        >
          {actionState === "revoking" ? "Revoking…" : "Revoke scanner role"}
        </button>
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
