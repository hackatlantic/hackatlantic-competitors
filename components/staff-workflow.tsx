import Link from "next/link";
import type { ReactNode } from "react";
import type { ApplicationAnswers } from "@/lib/api";
import {
  applicationQuestionLabel,
  orderedApplicationAnswers,
} from "@/lib/application-form";
import { BrandMark } from "@/components/brand-mark";

type StaffRole = "admin";

type StaffPageFrameProps = {
  role: StaffRole;
  eyebrow: string;
  title: string;
  children: ReactNode;
};

export function StaffPageFrame({
  role,
  title,
  children,
}: StaffPageFrameProps) {
  return (
    <main className="staff-page">
      <aside className="staff-sidebar">
        <BrandMark inverse />
        <nav className="staff-nav" aria-label={`${role} navigation`}>
          <Link href="/organizer/applications">Applications</Link>
          <Link href="/reviewer/applications">Review queue</Link>
          <Link href="/organizer/reviewers">Access</Link>
          <Link href="/organizer/operations">Operations</Link>
          <Link href="/scanner">Scanner</Link>
        </nav>
        <Link className="staff-home-link" href="/">Applicant view</Link>
      </aside>

      <section className="staff-content">
        <header className="staff-content-header">
          <div>
            <h1 id="staff-page-heading">{title}</h1>
          </div>
        </header>
        <div className="staff-panel" aria-labelledby="staff-page-heading">{children}</div>
      </section>
    </main>
  );
}

type StaffStateProps = {
  title: string;
  children: ReactNode;
};

export function StaffEmptyState({ title, children }: StaffStateProps) {
  return (
    <section className="staff-state" aria-live="polite">
      <h2>{title}</h2>
      <p>{children}</p>
    </section>
  );
}

export function StaffErrorState({ title, children }: StaffStateProps) {
  return (
    <section className="staff-state error-state" aria-live="assertive">
      <h2>{title}</h2>
      <p className="error-message" role="alert">
        {children}
      </p>
    </section>
  );
}

function displayAnswer(value: ApplicationAnswers[string]): string {
  if (typeof value === "boolean") {
    return value ? "Yes" : "No";
  }

  return String(value);
}

type ApplicationAnswersListProps = {
  answers: ApplicationAnswers;
};

export function ApplicationAnswersList({ answers }: ApplicationAnswersListProps) {
  const entries = orderedApplicationAnswers(answers);

  if (entries.length === 0) {
    return <p className="staff-muted">No answers were submitted.</p>;
  }

  return (
    <dl className="application-answers">
      {entries.map(([key, value]) => (
        <div key={key}>
          <dt>{applicationQuestionLabel(key)}</dt>
          <dd>{displayAnswer(value)}</dd>
        </div>
      ))}
    </dl>
  );
}
