import type { ApplicantReleasedDecision } from "@/lib/api";

export type DecisionLoadState = "loading" | "ready" | "empty" | "error";

type ApplicantDecisionStatusProps = {
  decision: ApplicantReleasedDecision | null;
  onRetry: () => void;
  state: DecisionLoadState;
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

export function ApplicantDecisionStatus({ decision, onRetry, state }: ApplicantDecisionStatusProps) {
  if (state === "loading") {
    return (
      <section className="application-decision" aria-busy="true" aria-live="polite">
        <h2>Decision</h2>
        <p>Checking for a released decision…</p>
      </section>
    );
  }

  if (state === "error") {
    return (
      <section className="application-decision" aria-live="polite">
        <h2>Decision unavailable</h2>
        <p className="error-message" role="alert">
          We could not load a released decision. Your application remains unchanged.
        </p>
        <button className="button secondary" onClick={onRetry} type="button">
          Try again
        </button>
      </section>
    );
  }

  if (state === "empty" || !decision) {
    return (
      <section className="application-decision" aria-live="polite">
        <h2>Decision</h2>
        <p>No decision has been released for your application yet.</p>
      </section>
    );
  }

  const outcomeLabel =
    decision.outcome === "accepted"
      ? "Accepted"
      : decision.outcome === "waitlisted"
        ? "Waitlisted"
        : "Rejected";

  return (
    <section className={`application-decision decision-${decision.outcome}`} aria-live="polite">
      <h2>Decision</h2>
      <p>
        Your application decision: <strong className="decision-outcome">{outcomeLabel}</strong>.
      </p>
      <p>
        Released <time dateTime={decision.releasedAt}>{displayTimestamp(decision.releasedAt)}</time>.
      </p>
    </section>
  );
}
