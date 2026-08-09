import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApplicantDecisionStatus } from "@/components/applicant-decision-status";

describe("ApplicantDecisionStatus", () => {
  it.each([
    ["accepted", "Accepted"],
    ["waitlisted", "Waitlisted"],
    ["rejected", "Rejected"],
  ] as const)("renders a released %s outcome with its semantic class", (outcome, label) => {
    const { container } = render(
      <ApplicantDecisionStatus
        decision={{ applicationId: "application-id", outcome, releasedAt: "2026-08-07T12:00:00Z" }}
        onRetry={() => undefined}
        state="ready"
      />,
    );

    expect(screen.getByText(label).className).toContain("decision-outcome");
    expect(container.querySelector("section")?.className).toContain(`decision-${outcome}`);
    expect(screen.getByText(label).closest("strong")).not.toBeNull();
  });

  it("offers a functional retry action when loading the decision fails", () => {
    const onRetry = vi.fn();
    render(<ApplicantDecisionStatus decision={null} onRetry={onRetry} state="error" />);

    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledOnce();
    expect(screen.getByRole("alert").textContent).toContain("application remains unchanged");
  });
});
