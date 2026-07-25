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

  return (
    <main className="admin-page">
      <header className="admin-header">
        <div>
          <p className="eyebrow">Admin</p>
          <h1>Applications</h1>
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
                <td>{applicant.full_name ?? "Not submitted"}</td>
                <td>{applicant.email ?? "Not submitted"}</td>
                <td>{applicant.school ?? "Not submitted"}</td>
                <td>
                  <span
                    className={
                      applicant.accepted ? "status-pill accepted-pill" : "status-pill"
                    }
                  >
                    {applicant.accepted ? "Accepted" : "Pending"}
                  </span>
                </td>
                <td>
                  {applicant.qr_code_id ? <code>{applicant.qr_code_id}</code> : "-"}
                </td>
                <td>
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
                    "-"
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
