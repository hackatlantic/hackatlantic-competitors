import { auth } from "@clerk/nextjs/server";
import { redirect } from "next/navigation";
import { ScannerRoleForm } from "@/components/organizer-reviewer-actions";
import { StaffPageFrame } from "@/components/staff-workflow";

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
    >
      <p className="staff-summary">
        Find a volunteer by email to grant or revoke scanner access. Scanners can
        check attendee passes without access to applications or review notes.
      </p>
      <ScannerRoleForm />
    </StaffPageFrame>
  );
}
