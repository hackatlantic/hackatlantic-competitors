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
        Admin access comes only from the backend privileged-email allowlist. Grant or
        revoke scanner access here for event volunteers who do not need ATS access.
      </p>
      <ScannerRoleForm />
    </StaffPageFrame>
  );
}
