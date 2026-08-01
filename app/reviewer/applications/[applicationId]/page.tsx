import { auth } from "@clerk/nextjs/server";
import Link from "next/link";
import { redirect } from "next/navigation";
import { ReviewerReviewForm } from "@/components/reviewer-review-form";
import { AdminResumeViewer } from "@/components/admin-resume-viewer";
import {
  ApplicationAnswersList,
  StaffErrorState,
  StaffPageFrame,
} from "@/components/staff-workflow";
import {
  ApiError,
  createApiClient,
  type ReviewerApplication,
} from "@/lib/api";

type ReviewerApplicationDetailPageProps = {
  params: Promise<{
    applicationId: string;
  }>;
};

export default async function ReviewerApplicationDetailPage({
  params,
}: ReviewerApplicationDetailPageProps) {
  const { userId, getToken } = await auth();
  if (!userId) {
    redirect("/");
  }

  const { applicationId } = await params;
  const client = createApiClient({ getToken });
  let application: ReviewerApplication | null = null;
  let loadError: unknown = undefined;

  try {
    application = await client.getReviewerApplication(applicationId);
  } catch (error) {
    loadError = error;
  }

  if (application === null) {
    const message =
      loadError instanceof ApiError && loadError.status === 403
        ? "Admin access is required to view submitted applications."
        : loadError instanceof ApiError && loadError.status === 404
          ? "This submitted application is not available for review."
          : loadError instanceof Error
            ? loadError.message
            : "The application could not be loaded.";

    return (
      <StaffPageFrame
        eyebrow="Admin workspace"
        role="admin"
        title="Application review"
      >
        <StaffErrorState title="Application unavailable">{message}</StaffErrorState>
      </StaffPageFrame>
    );
  }

  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Application review"
    >
      <p className="staff-summary">
        <Link className="staff-link" href="/reviewer/applications">
          Back to review queue
        </Link>
      </p>

      <div className="application-detail">
        <section aria-labelledby="review-applicant-heading">
          <h2 id="review-applicant-heading">Applicant</h2>
          <p className="staff-summary">
            {application.applicant.displayName || "No display name"} ·{" "}
            {application.applicant.email}
          </p>
          <p className="staff-muted">
            Submitted {application.submittedAt}
            {application.assignment?.assignedAt
              ? ` · Assigned ${application.assignment.assignedAt}`
              : " · Not assigned to you"}
          </p>
        </section>

        <section aria-labelledby="review-answers-heading">
          <h2 id="review-answers-heading">Submitted answers</h2>
          <ApplicationAnswersList answers={application.answers} />
        </section>

        <section aria-labelledby="review-resume-heading">
          <h2 id="review-resume-heading">Resume</h2>
          <AdminResumeViewer applicationId={application.id} />
        </section>

        <section aria-labelledby="review-form-heading">
          <h2 id="review-form-heading">Your internal review</h2>
          <ReviewerReviewForm
            applicationId={application.id}
            initialReview={application.review}
          />
        </section>
      </div>
    </StaffPageFrame>
  );
}
