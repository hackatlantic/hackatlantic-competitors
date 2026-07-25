import Link from "next/link";
import { SignInButton, UserButton } from "@clerk/nextjs";
import { requireAdmin } from "@/lib/admin-auth";
import { listApplicants } from "@/lib/applicants";

export const dynamic = "force-dynamic";

export default async function AdminPage() {
  const admin = await requireAdmin();

  if (!admin.authorized) {
    return (
      <main className="admin-page">
        <section className="panel admin-empty">
          <p className="eyebrow">Admin</p>
          <h1>Restricted</h1>
          <p>
            {admin.reason === "signed-out"
              ? "Sign in with an authorized admin account."
              : "Your account is not authorized to view applications."}
          </p>
          {admin.reason === "signed-out" ? (
            <SignInButton>
              <button className="button primary" type="button">
                Sign in
              </button>
            </SignInButton>
          ) : null}
        </section>
      </main>
    );
  }

  const applicants = await listApplicants();
  const submittedCount = applicants.filter((applicant) => applicant.applied_at)
    .length;
  const acceptedCount = applicants.filter((applicant) => applicant.accepted)
    .length;

  return (
    <main className="admin-page">
      <header className="admin-header">
        <div>
          <p className="eyebrow">Admin</p>
          <h1>Applications</h1>
          <p>
            {submittedCount} submitted, {acceptedCount} accepted
          </p>
        </div>
        <UserButton />
      </header>

      <section className="admin-table-wrap">
        <table className="admin-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>School</th>
              <th>Status</th>
              <th>QR id</th>
              <th>Resume</th>
            </tr>
          </thead>
          <tbody>
            {applicants.map((applicant) => (
              <tr key={applicant.user_id}>
                <td>
                  <div className="primary-cell">
                    <span>{applicant.full_name ?? "Not submitted"}</span>
                    <small>{formatDate(applicant.applied_at)}</small>
                  </div>
                </td>
                <td className="truncate-cell">{applicant.email ?? "Not submitted"}</td>
                <td className="truncate-cell">{applicant.school ?? "Not submitted"}</td>
                <td className="center-cell">
                  <span
                    className={
                      applicant.accepted ? "status-pill accepted-pill" : "status-pill"
                    }
                  >
                    {applicant.accepted ? "Accepted" : "Pending"}
                  </span>
                </td>
                <td className="mono-cell">
                  {applicant.qr_code_id ? <code>{applicant.qr_code_id}</code> : "—"}
                </td>
                <td className="action-cell">
                  {applicant.resume_path ? (
                    <Link
                      className="text-link"
                      href={`/api/admin/resumes/${encodeResumePath(
                        applicant.resume_path
                      )}`}
                      target="_blank"
                    >
                      Open resume
                    </Link>
                  ) : (
                    "—"
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  );
}

function encodeResumePath(path: string) {
  return path.split("/").map(encodeURIComponent).join("/");
}

function formatDate(date: string | null) {
  if (!date) {
    return "Not submitted";
  }

  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short"
  }).format(new Date(date));
}
