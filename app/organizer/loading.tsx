import { StaffPageFrame } from "@/components/staff-workflow";

export default function OrganizerLoading() {
  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Loading admin workspace"
    >
      <p className="staff-summary" aria-live="polite">
        Loading authorized application data…
      </p>
    </StaffPageFrame>
  );
}
