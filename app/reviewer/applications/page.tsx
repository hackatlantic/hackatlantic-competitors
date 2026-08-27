import { auth } from "@clerk/nextjs/server";
import Link from "next/link";
import { redirect } from "next/navigation";
import {
  StaffEmptyState,
  StaffErrorState,
  StaffPageFrame,
} from "@/components/staff-workflow";
import {
  ApiError,
  createApiClient,
  type ReviewerApplicationListResponse,
} from "@/lib/api";

export default async function ReviewerApplicationsPage() {
  const { userId, getToken } = await auth();
  if (!userId) {
    redirect("/");
  }

  const client = createApiClient({ getToken });
  let applications: ReviewerApplicationListResponse | null = null;
  let loadError: unknown = undefined;

  try {
    applications = await client.listReviewerApplications();
  } catch (error) {
    loadError = error;
  }

  if (applications === null) {
    const message =
      loadError instanceof ApiError && loadError.status === 403
        ? "Admin access is required to view the review queue."
        : loadError instanceof Error
          ? loadError.message
          : "The review queue could not be loaded.";

    return (
      <StaffPageFrame
        eyebrow="Admin workspace"
        role="admin"
        title="Review queue"
      >
        <StaffErrorState title="Review queue unavailable">{message}</StaffErrorState>
      </StaffPageFrame>
    );
  }

  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Review queue"
    >
      <p className="staff-summary">Review submitted applications.</p>

      <div className="staff-metrics" aria-label="Review queue summary">
        <div><span>Submitted</span><strong>{applications.items.length.toString().padStart(2, "0")}</strong></div>
        <div><span>Reviewed</span><strong>{applications.items.filter((item) => item.review?.status === "submitted").length.toString().padStart(2, "0")}</strong></div>
        <div><span>Still open</span><strong>{applications.items.filter((item) => item.review?.status !== "submitted").length.toString().padStart(2, "0")}</strong></div>
      </div>

      {applications.items.length === 0 ? (
        <StaffEmptyState title="No submitted applications">
          Submitted applications will appear here when they are available for review.
        </StaffEmptyState>
      ) : (
        <ul className="staff-list">
          {applications.items.map((application) => (
            <li key={application.id}>
              <span className="staff-row-index" aria-hidden="true">{application.id.slice(0, 4)}</span>
              <div className="staff-row-primary">
                <h2>
                  {application.applicant.displayName || application.applicant.email}
                </h2>
                <p>{application.applicant.email}</p>
                <p className="staff-row-meta">
                  {application.assignment?.assignedAt
                    ? `Assigned ${new Date(application.assignment.assignedAt).toLocaleDateString()}`
                    : "Open queue"}
                  {application.review?.status
                    ? ` · Review ${application.review.status}`
                    : " · Unreviewed"}
                </p>
              </div>
              <span className={`status-pill ${application.review?.status === "submitted" ? "accepted" : "submitted"}`}>
                {application.review?.status === "submitted" ? "Reviewed" : "To review"}
              </span>
              <Link
                className="staff-link"
                href={`/reviewer/applications/${application.id}`}
              >
                Review
              </Link>
            </li>
          ))}
        </ul>
      )}
    </StaffPageFrame>
  );
}
