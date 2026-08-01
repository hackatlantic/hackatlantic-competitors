"use client";

import { useAuth } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { type FormEvent, useMemo, useState } from "react";
import {
  createApiClient,
  type DecisionOutcome,
  type OrganizerDecision,
} from "@/lib/api";

type ActionState = "idle" | "recording" | "releasing";

type OrganizerDecisionActionsProps = {
  applicationId: string;
  initialDecision: OrganizerDecision | null;
};

const outcomeOptions: Array<{ label: string; value: DecisionOutcome }> = [
  { value: "accepted", label: "Accepted" },
  { value: "waitlisted", label: "Waitlisted" },
  { value: "rejected", label: "Rejected" },
];

export function OrganizerDecisionActions({
  applicationId,
  initialDecision,
}: OrganizerDecisionActionsProps) {
  const { getToken } = useAuth();
  const router = useRouter();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [decision, setDecision] = useState<OrganizerDecision | null>(initialDecision);
  const [outcome, setOutcome] = useState<DecisionOutcome>("accepted");
  const [internalReason, setInternalReason] = useState("");
  const [actionState, setActionState] = useState<ActionState>("idle");
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [confirmingRelease, setConfirmingRelease] = useState(false);


  const recordDecision = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setActionState("recording");
    setNotice("");
    setActionError("");

    try {
      const recordedDecision = await client.recordOrganizerDecision(applicationId, {
        outcome,
        ...(internalReason.trim() ? { internalReason: internalReason.trim() } : {}),
      });
      setDecision(recordedDecision);
      setConfirmingRelease(false);
      setInternalReason("");
      setNotice("Decision recorded. It remains internal until it is released.");
      router.refresh();
    } catch (error) {
      setActionError(
        error instanceof Error ? error.message : "Unable to record the decision.",
      );
    } finally {
      setActionState("idle");
    }
  };

  const releaseDecision = async () => {
    if (!decision || decision.releasedAt) {
      return;
    }

    setActionState("releasing");
    setNotice("");
    setActionError("");

    try {
      const releasedDecision = await client.releaseOrganizerDecision(decision.id);
      setDecision(releasedDecision);
      setNotice("Decision released to the applicant.");
      router.refresh();
    } catch (error) {
      setActionError(
        error instanceof Error ? error.message : "Unable to release the decision.",
      );
    } finally {
      setActionState("idle");
    }
  };

  return (
    <div className="decision-workflow">
      {decision ? (
        <div className="decision-current">
          <h3>
            Current decision:{" "}
            {outcomeOptions.find((option) => option.value === decision.outcome)?.label ??
              decision.outcome}
          </h3>
          <p className="staff-muted">
            {decision.releasedAt
              ? "Released to the applicant. Record a new decision to supersede it."
              : "Internal only. Release it when the applicant may view the outcome."}
          </p>
          {!decision.releasedAt && !confirmingRelease ? (
            <button
              className="button secondary"
              disabled={actionState !== "idle"}
              onClick={() => setConfirmingRelease(true)}
              type="button"
            >
              Release decision
            </button>
          ) : null}
          {!decision.releasedAt && confirmingRelease ? (
            <div className="decision-release-confirmation" role="alert">
              <p>
                This immediately shows the <strong>{decision.outcome}</strong> decision to
                the applicant and queues their email. It cannot be unreleased.
              </p>
              <div className="staff-actions">
                <button
                  className="button primary"
                  disabled={actionState !== "idle"}
                  onClick={() => void releaseDecision()}
                  type="button"
                >
                  {actionState === "releasing" ? "Releasing…" : "Confirm release"}
                </button>
                <button
                  className="button secondary"
                  disabled={actionState !== "idle"}
                  onClick={() => setConfirmingRelease(false)}
                  type="button"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : null}
        </div>
      ) : (
        <p className="staff-muted">No decision has been recorded for this application.</p>
      )}

      <form className="decision-form" onSubmit={recordDecision}>
        <div>
          <label htmlFor="decision-outcome">Outcome</label>
          <select
            disabled={actionState !== "idle"}
            id="decision-outcome"
            onChange={(event) => setOutcome(event.target.value as DecisionOutcome)}
            value={outcome}
          >
            {outcomeOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="decision-internal-reason">Internal reason (optional)</label>
          <textarea
            disabled={actionState !== "idle"}
            id="decision-internal-reason"
            onChange={(event) => setInternalReason(event.target.value)}
            rows={4}
            value={internalReason}
          />
          <p className="staff-muted">
            Internal reasons are not included in applicant responses or decision emails.
          </p>
        </div>
        <button className="button primary" disabled={actionState !== "idle"} type="submit">
          {actionState === "recording"
            ? "Recording…"
            : decision
              ? "Record superseding decision"
              : "Record decision"}
        </button>
      </form>

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
