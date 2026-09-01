import Link from "next/link";
import type { ReactNode } from "react";
import type { ApplicationAnswers } from "@/lib/api";
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
  eyebrow,
  title,
  children,
}: StaffPageFrameProps) {
  return (
    <main className="staff-page">
      <aside className="staff-sidebar">
        <BrandMark inverse />
        <div className="staff-sidebar-context">
          <span className="coordinate-label">CONTROL DESK</span>
          <strong>{eyebrow}</strong>
        </div>
        <nav className="staff-nav" aria-label={`${role} navigation`}>
          <Link href="/organizer/applications"><span>01</span>Applications</Link>
          <Link href="/reviewer/applications"><span>02</span>Review queue</Link>
          <Link href="/organizer/reviewers"><span>03</span>Access</Link>
          <Link href="/organizer/operations"><span>04</span>Operations</Link>
          <Link href="/scanner"><span>05</span>Scanner</Link>
        </nav>
        <Link className="staff-home-link" href="/">← Applicant view</Link>
      </aside>

      <section className="staff-content">
        <header className="staff-content-header">
          <div>
            <p className="eyebrow">{eyebrow}</p>
            <h1 id="staff-page-heading">{title}</h1>
          </div>
          <span className="system-status"><i /> System live</span>
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

const applicationAnswerLabels: Record<string, string> = {
  fullName: "Name",
  email: "Email",
  school: "School",
  dietaryRestrictions: "Dietary restrictions",
  hackAtlanticExcitement: "What are you most excited about at Hack Atlantic?",
  priorHackathonExperience: "Prior hackathon experience",
  desiredTeammateNames: "Desired teammate names",
  hardwareProject: "Looking to make a hardware project?",
  hardwareEquipment: "Requested hardware equipment",
};

type ApplicationAnswersListProps = {
  answers: ApplicationAnswers;
};

export function ApplicationAnswersList({ answers }: ApplicationAnswersListProps) {
  const entries = Object.entries(answers);

  if (entries.length === 0) {
    return <p className="staff-muted">No answers were submitted.</p>;
  }

  return (
    <dl className="application-answers">
      {entries.map(([key, value]) => (
        <div key={key}>
          <dt>{applicationAnswerLabels[key] ?? key}</dt>
          <dd>{displayAnswer(value)}</dd>
        </div>
      ))}
    </dl>
  );
}
