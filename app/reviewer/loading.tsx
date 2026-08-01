import { StaffPageFrame } from "@/components/staff-workflow";

export default function ReviewerLoading() {
  return (
    <StaffPageFrame
      eyebrow="Admin workspace"
      role="admin"
      title="Loading review workspace"
    >
      <p className="staff-summary" aria-live="polite">
        Loading authorized review data…
      </p>
    </StaffPageFrame>
  );
}
