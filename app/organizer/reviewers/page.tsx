import { auth } from "@clerk/nextjs/server";
import { redirect } from "next/navigation";
import { ScannerRoleForm } from "@/components/organizer-reviewer-actions";
import { StaffPageFrame } from "@/components/staff-workflow";
import { VolunteerApprovalQueue } from "@/components/volunteer-access";
import "@/app/volunteer/volunteer.css";

export default async function OrganizerReviewersPage() {
  const { userId } = await auth();
  if (!userId) {
    redirect("/");
  }

  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Scanner access"
      compactHeader
    >
      <div className="volunteer-admin">
        <VolunteerApprovalQueue />
        <details className="volunteer-email-manager">
          <summary>Manage access by email</summary>
          <ScannerRoleForm />
        </details>
      </div>
    </StaffPageFrame>
  );
}
