import { StaffPageFrame } from "@/components/staff-workflow";

export default function OrganizerOperationsLoading() {
  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Check-in"
      compactHeader
    >
      <p className="staff-summary" aria-live="polite">
        Loading attendance…
      </p>
    </StaffPageFrame>
  );
}
