import { auth } from "@clerk/nextjs/server";
import Link from "next/link";
import { redirect } from "next/navigation";
import {
  ApiError,
  createApiClient,
  type OrganizerApplicationFilters,
  type OrganizerApplicationListResponse,
} from "@/lib/api";
import {
  StaffEmptyState,
  StaffErrorState,
  StaffPageFrame,
} from "@/components/staff-workflow";

type OrganizerApplicationsPageProps = {
  searchParams: Promise<{
    q?: string;
    status?: string;
  }>;
};

export default async function OrganizerApplicationsPage({
  searchParams,
}: OrganizerApplicationsPageProps) {
  const { userId, getToken } = await auth();
  if (!userId) {
    redirect("/");
  }

  const query = await searchParams;
  const filters: OrganizerApplicationFilters = {
    ...(query.q ? { q: query.q } : {}),
    ...(query.status === "submitted" ||
    query.status === "accepted" ||
    query.status === "waitlisted" ||
    query.status === "rejected"
      ? { status: query.status }
      : {}),
  };
  const client = createApiClient({ getToken });
  let applications: OrganizerApplicationListResponse | null = null;
  let loadError: unknown = undefined;

  try {
    applications = await client.listOrganizerApplications(filters);
  } catch (error) {
    loadError = error;
  }

  if (applications === null) {
    const message =
      loadError instanceof ApiError && loadError.status === 403
        ? "Admin access is required to view applications."
        : loadError instanceof Error
          ? loadError.message
          : "Applications could not be loaded.";

    return (
      <StaffPageFrame
        eyebrow="Admin workspace"
        role="admin"
        title="Applications"
      >
        <StaffErrorState title="Applications unavailable">{message}</StaffErrorState>
      </StaffPageFrame>
    );
  }

  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Applications"
    >
      <p className="staff-summary">
        Search applications after they have been submitted. Draft answers remain
        private to applicants.
      </p>

      <div className="staff-metrics" aria-label="Application queue summary">
        <div><span>Visible records</span><strong>{applications.items.length.toString().padStart(2, "0")}</strong></div>
        <div><span>Awaiting decision</span><strong>{applications.items.filter((item) => item.status === "submitted").length.toString().padStart(2, "0")}</strong></div>
        <div><span>Current scope</span><strong className="metric-word">{filters.status ?? "All"}</strong></div>
      </div>

      <form className="staff-toolbar" method="get">
        <div className="staff-filter">
          <label htmlFor="application-search">Search applications</label>
          <input
            defaultValue={filters.q ?? ""}
            id="application-search"
            name="q"
            placeholder="Name or email"
            type="search"
          />
        </div>
        <div className="staff-filter">
          <label htmlFor="application-status">Status</label>
          <select
            defaultValue={filters.status ?? ""}
            id="application-status"
            name="status"
          >
            <option value="">All available statuses</option>
            <option value="submitted">Submitted</option>
            <option value="accepted">Accepted</option>
            <option value="waitlisted">Waitlisted</option>
            <option value="rejected">Rejected</option>
          </select>
        </div>
        <button className="button primary" type="submit">
          Apply filters
        </button>
      </form>

      {applications.items.length === 0 ? (
        <StaffEmptyState title="No applications found">
          Try a different search or status filter.
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
                  {application.submittedAt
                    ? `Submitted ${new Date(application.submittedAt).toLocaleDateString()}`
                    : "Awaiting submission"}
                </p>
              </div>
              <span className={`status-pill ${application.status}`}>{application.status}</span>
              <Link
                className="staff-link"
                href={`/organizer/applications/${application.id}`}
              >
                Open <span aria-hidden="true">↗</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </StaffPageFrame>
  );
}
