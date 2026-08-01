import { StaffPageFrame } from "@/components/staff-workflow";

export default function OrganizerOperationsLoading() {
  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Loading event operations"
    >
      <p className="staff-summary" aria-live="polite">
        Loading authorized checkpoint and redemption operations…
      </p>
    </StaffPageFrame>
  );
}
