import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApplicantDashboard } from "@/components/applicant-dashboard";
import { ApiError, type ApplicantApplication } from "@/lib/api";

const authGetToken = vi.hoisted(() => vi.fn());
const motionPreference = vi.hoisted(() => ({ reduced: false }));
const scrollIntoView = vi.fn();

vi.mock("framer-motion", async (importOriginal) => ({
  ...(await importOriginal<typeof import("framer-motion")>()),
  useReducedMotion: () => motionPreference.reduced,
}));
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
    motionPreference.reduced = false;
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView,
    });
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
        { key: "hardwareEquipment", label: "What equipment are you looking to use?", type: "string", required: true, showWhen: { key: "hardwareProject", equals: true } },
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
    await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText(/Name/)));
  });

  it("reveals hardware details and immediately removes hidden controls and stale errors", async () => {
    render(<ApplicantDashboard />);
    await screen.findByRole("heading", { name: "Your application" });
    expect(screen.queryByLabelText(/What equipment/)).toBeNull();

    fireEvent.click(screen.getByLabelText("Yes"));
    const equipment = screen.getByLabelText(/What equipment/);
    expect(equipment.closest(".conditional-question")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Submit application" }));
    expect(equipment.getAttribute("aria-invalid")).toBe("true");
    fireEvent.change(equipment, { target: { value: "Microcontroller" } });
    expect(equipment.getAttribute("aria-invalid")).toBe("false");

    fireEvent.click(screen.getByLabelText("No"));
    // No exit animation may leave an irrelevant field focusable or submitted.
    expect(screen.queryByLabelText(/What equipment/)).toBeNull();
    expect(equipment.isConnected).toBe(false);
    fireEvent.click(screen.getByLabelText("Yes"));
    expect((screen.getByLabelText(/What equipment/) as HTMLTextAreaElement).value).toBe("");
    expect(screen.getByLabelText(/What equipment/).getAttribute("aria-invalid")).toBe("false");
  });

  it("shows pending feedback, blocks duplicate saves, and confirms the saved state", async () => {
    const draft = (await api.getMyApplications()).items[0] as ApplicantApplication;
    let finishSave!: (value: ApplicantApplication) => void;
    api.saveApplicationDraft.mockImplementationOnce(() => new Promise<ApplicantApplication>((resolve) => {
      finishSave = resolve;
    }));
    render(<ApplicantDashboard />);
    await screen.findByRole("heading", { name: "Your application" });
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: "Test Applicant" } });
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    const saving = screen.getByRole("button", { name: "Saving…" });
    expect(saving.getAttribute("aria-busy")).toBe("true");
    expect((saving as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Submit application" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(saving);
    expect(api.saveApplicationDraft).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("Saving your changes…")).toBeTruthy();

    await act(async () => finishSave({ ...draft, lockVersion: 2, answers: { fullName: "Test Applicant", email: "applicant@example.com" } }));
    expect(await screen.findByText("All changes saved")).toBeTruthy();
    expect(screen.getByText("Draft saved.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Save draft" }).getAttribute("aria-busy")).toBeNull();
  });

  it("keeps validation and conditional fields usable without motion", async () => {
    motionPreference.reduced = true;
    render(<ApplicantDashboard />);
    await screen.findByRole("heading", { name: "Your application" });
    fireEvent.click(screen.getByLabelText("Yes"));
    const group = screen.getByLabelText(/What equipment/).closest(".conditional-question") as HTMLElement;
    expect(group.style.opacity).toBe("1");
    expect(group.style.transform).toBe("none");
    fireEvent.click(screen.getByRole("button", { name: "Submit application" }));
    await waitFor(() => expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "instant", block: "center" }));
    expect(document.activeElement).toBe(screen.getByLabelText(/Name/));
  });

  it("renders an attached resume as an aligned application summary card", async () => {
    api.getMyApplications.mockResolvedValueOnce({
      items: [{
        id: "application-1",
        cycleId: "cycle-1",
        formId: "form-1",
        formVersion: 2,
        status: "submitted",
        lockVersion: 1,
        answers: {},
        submittedAt: "2026-09-01T14:09:00Z",
        createdAt: "2026-09-01T12:00:00Z",
        updatedAt: "2026-09-01T14:09:00Z",
      }],
      nextCursor: null,
    });
    api.getApplicationResume.mockResolvedValueOnce({
      originalFilename: "AdebowaleAdebayo2026-08-24.pdf",
      byteSize: 1024,
      uploadedAt: "2026-08-24T12:00:00Z",
    });

    render(<ApplicantDashboard />);

    const heading = await screen.findByRole("heading", { name: "Resume" });
    const summary = heading.closest(".application-resume-summary");
    expect(summary).toBeTruthy();
    expect(summary?.textContent).toContain("AdebowaleAdebayo2026-08-24.pdf");
    expect(summary?.textContent).toContain("Attached");
    expect(heading.closest(".application-overview")).toBeTruthy();
  });
});
