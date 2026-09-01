import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApplicantDashboard } from "@/components/applicant-dashboard";
import { ApiError } from "@/lib/api";

const authGetToken = vi.hoisted(() => vi.fn());
const api = vi.hoisted(() => ({
  createApplication: vi.fn(),
  getApplicationDecision: vi.fn(),
  getApplicationResume: vi.fn(),
  getCurrentApplicationForm: vi.fn(),
  getCurrentUser: vi.fn(),
  getMyApplications: vi.fn(),
  saveApplicationDraft: vi.fn(),
  submitApplication: vi.fn(),
  uploadApplicationResume: vi.fn(),
}));

vi.mock("@clerk/nextjs", () => ({
  useAuth: () => ({ getToken: authGetToken, isLoaded: true }),
}));

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  createApiClient: () => api,
}));

vi.mock("@/components/applicant-decision-status", () => ({
  ApplicantDecisionStatus: () => null,
}));

vi.mock("@/components/applicant-pass", () => ({
  ApplicantPass: () => null,
}));

describe("ApplicantDashboard", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(window, "scrollTo", {
      configurable: true,
      value: vi.fn(),
    });
    api.getCurrentUser.mockResolvedValue({
      id: "user-1",
      email: "applicant@example.com",
      displayName: "Applicant",
      roles: ["applicant"],
    });
    api.getCurrentApplicationForm.mockResolvedValue({
      id: "form-1",
      cycleId: "cycle-1",
      version: 2,
      resumeRequired: false,
      resumeAfterQuestionKey: "school",
      questions: [
        { key: "fullName", label: "Name", type: "string", required: true, section: "Build your profile", control: "text" },
        { key: "email", label: "Email", type: "string", required: true, section: "Build your profile", control: "email" },
        { key: "school", label: "School", type: "string", required: true, section: "Build your profile", control: "text" },
        { key: "hardwareProject", label: "Are you looking to make a hardware project?", type: "boolean", required: true, section: "Hackathon Specific Questions" },
      ],
    });
    api.getMyApplications.mockResolvedValue({
      items: [{
        id: "application-1",
        cycleId: "cycle-1",
        formId: "form-1",
        formVersion: 2,
        status: "draft",
        lockVersion: 1,
        answers: {},
        createdAt: "2026-09-01T12:00:00Z",
        updatedAt: "2026-09-01T12:00:00Z",
      }],
      nextCursor: null,
    });
    api.getApplicationDecision.mockRejectedValue(
      new ApiError(404, { code: "decision_not_found" }),
    );
    api.getApplicationResume.mockRejectedValue(
      new ApiError(404, { code: "resume_not_found" }),
    );
  });

  it("removes internal form metadata and blocks incomplete submissions", async () => {
    render(<ApplicantDashboard />);

    await screen.findByRole("heading", { name: "Your application" });

    expect(screen.queryByText(/Applicant field notes/i)).toBeNull();
    expect(screen.queryByText(/Form version 2/i)).toBeNull();
    expect(screen.queryByText(/^Part [12]$/i)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Submit application" }));

    await waitFor(() => {
      expect(screen.getByLabelText(/Name/).getAttribute("aria-invalid")).toBe("true");
      expect(screen.getByLabelText(/School/).getAttribute("aria-invalid")).toBe("true");
      expect(
        screen.getByRole("group", { name: /hardware project/i }).getAttribute("aria-invalid"),
      ).toBe("true");
    });

    expect(screen.getByText("Complete the highlighted fields before submitting.")).toBeTruthy();
    expect(api.saveApplicationDraft).not.toHaveBeenCalled();
    expect(api.submitApplication).not.toHaveBeenCalled();
  });
});
