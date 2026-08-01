import { auth } from "@clerk/nextjs/server";
import Link from "next/link";
import { redirect } from "next/navigation";
import { OrganizerDecisionActions } from "@/components/organizer-decision-actions";
import { OrganizerPassActions } from "@/components/organizer-pass-actions";
import { AdminResumeViewer } from "@/components/admin-resume-viewer";
import {
  ApplicationAnswersList,
  StaffErrorState,
  StaffPageFrame,
} from "@/components/staff-workflow";
import {
  ApiError,
  createApiClient,
  type OrganizerApplication,
} from "@/lib/api";

type OrganizerApplicationDetailPageProps = {
  params: Promise<{
    applicationId: string;
  }>;
};

export default async function OrganizerApplicationDetailPage({
  params,
}: OrganizerApplicationDetailPageProps) {
  const { userId, getToken } = await auth();
  if (!userId) {
    redirect("/");
  }

  const { applicationId } = await params;
  const client = createApiClient({ getToken });
  let application: OrganizerApplication | null = null;
  let loadError: unknown = undefined;

  try {
    application = await client.getOrganizerApplication(applicationId);
  } catch (error) {
    loadError = error;
  }

  if (application === null) {
    const message =
      loadError instanceof ApiError && loadError.status === 403
        ? "Admin access is required to view this application."
        : loadError instanceof ApiError && loadError.status === 404
          ? "This application does not exist or is no longer available."
          : loadError instanceof Error
            ? loadError.message
            : "The application could not be loaded.";

    return (
      <StaffPageFrame
        eyebrow="Admin workspace"
        role="admin"
        title="Application detail"
      >
        <StaffErrorState title="Application unavailable">{message}</StaffErrorState>
      </StaffPageFrame>
    );
  }

  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Application detail"
    >
      <p className="staff-summary">
        <Link className="staff-link" href="/organizer/applications">
          Back to applications
        </Link>
      </p>

      <div className="application-detail">
        <section aria-labelledby="applicant-heading">
          <h2 id="applicant-heading">Applicant</h2>
          <p className="staff-summary">
            {application.applicant.displayName || "No display name"} ·{" "}
            {application.applicant.email}
          </p>
          <p className="staff-muted">
            Status: {application.status}
            {application.submittedAt
              ? ` · Submitted ${application.submittedAt}`
              : ""}
          </p>
        </section>

        <section aria-labelledby="answers-heading">
          <h2 id="answers-heading">Submitted answers</h2>
          <ApplicationAnswersList answers={application.answers} />
        </section>

        <section aria-labelledby="resume-heading">
          <h2 id="resume-heading">Resume</h2>
          <AdminResumeViewer applicationId={application.id} />
        </section>

        <section aria-labelledby="decision-heading">
          <h2 id="decision-heading">Decision</h2>
          {application.submittedAt ? (
            <OrganizerDecisionActions
              applicationId={application.id}
              initialDecision={application.currentDecision ?? null}
            />
          ) : (
            <p className="staff-muted">
              A decision can be recorded only after the application is submitted.
            </p>
          )}
        </section>

        {application.attendeePass ? (
          <section aria-labelledby="pass-heading">
            <h2 id="pass-heading">Attendee pass</h2>
            <OrganizerPassActions initialAttendeePass={application.attendeePass} />
          </section>
        ) : null}
      </div>
    </StaffPageFrame>
  );
}
