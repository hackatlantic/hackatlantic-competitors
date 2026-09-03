import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApplicantRSVP } from "@/components/applicant-rsvp";
import { ApiError } from "@/lib/api";

const api = vi.hoisted(() => ({ getApplicationRSVP: vi.fn(), respondToRSVP: vi.fn() }));
const getToken = vi.hoisted(() => vi.fn());
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken }) }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));

const pending = { applicationId: "application", decisionId: "acceptance", status: "pending", lockVersion: 0 };

describe("ApplicantRSVP", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks();
    api.getApplicationRSVP.mockResolvedValue(pending);
    api.respondToRSVP.mockImplementation(async (_id, input) => ({ ...pending, ...input, lockVersion: 1 }));
  });

  it("loads an existing response and confirms using its decision and version", async () => {
    render(<ApplicantRSVP applicationId="application" />);
    fireEvent.click(await screen.findByRole("button", { name: "Confirm attendance" }));
    await screen.findByText("You’re confirmed! We look forward to seeing you.");
    expect(api.respondToRSVP).toHaveBeenCalledWith("application", { decisionId: "acceptance", lockVersion: 0, status: "confirmed" });
    expect((screen.getByRole("button", { name: "Attendance confirmed" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("requires confirmation before declining and allows cancellation", async () => {
    render(<ApplicantRSVP applicationId="application" />);
    fireEvent.click(await screen.findByRole("button", { name: "I can’t attend" }));
    expect(api.respondToRSVP).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Keep my current response" }));
    expect(api.respondToRSVP).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "I can’t attend" }));
    fireEvent.click(screen.getByRole("button", { name: "Yes, I can’t attend" }));
    await screen.findByText("Your RSVP is saved: you won’t be attending.");
    expect(api.respondToRSVP).toHaveBeenCalledWith("application", expect.objectContaining({ status: "declined" }));
  });

  it("can change a previously declined response", async () => {
    api.getApplicationRSVP.mockResolvedValue({ ...pending, status: "declined", lockVersion: 3 });
    render(<ApplicantRSVP applicationId="application" />);
    fireEvent.click(await screen.findByRole("button", { name: "Confirm attendance" }));
    await waitFor(() => expect(api.respondToRSVP).toHaveBeenCalledWith("application", expect.objectContaining({ lockVersion: 3, status: "confirmed" })));
  });

  it("does not optimistically show success and prevents repeat requests while saving", async () => {
    let complete!: (value: unknown) => void;
    api.respondToRSVP.mockReturnValue(new Promise((resolve) => { complete = resolve; }));
    render(<ApplicantRSVP applicationId="application" />);
    const button = await screen.findByRole("button", { name: "Confirm attendance" });
    fireEvent.click(button); fireEvent.click(button);
    expect(api.respondToRSVP).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("You’re confirmed! We look forward to seeing you.")).toBeNull();
    await act(async () => complete({ ...pending, status: "confirmed", lockVersion: 1 }));
    expect(screen.getByText("You’re confirmed! We look forward to seeing you.")).not.toBeNull();
  });

  it("shows a failed save and permits a safe retry", async () => {
    api.respondToRSVP.mockRejectedValueOnce(new Error("offline"));
    render(<ApplicantRSVP applicationId="application" />);
    fireEvent.click(await screen.findByRole("button", { name: "Confirm attendance" }));
    expect((await screen.findByRole("alert")).textContent).toContain("couldn’t confirm");
    fireEvent.click(screen.getByRole("button", { name: "Confirm attendance" }));
    await screen.findByText("You’re confirmed! We look forward to seeing you.");
  });

  it("requires reloading after a version conflict instead of overwriting it", async () => {
    api.respondToRSVP.mockRejectedValueOnce(new ApiError(409, { code: "rsvp_conflict" }));
    render(<ApplicantRSVP applicationId="application" />);
    fireEvent.click(await screen.findByRole("button", { name: "Confirm attendance" }));
    fireEvent.click(await screen.findByRole("button", { name: "Reload RSVP" }));
    await screen.findByRole("button", { name: "Confirm attendance" });
    expect(api.getApplicationRSVP).toHaveBeenCalledTimes(2);
    expect(api.respondToRSVP).toHaveBeenCalledTimes(1);
  });

  it("does not offer response controls when no released acceptance is available", async () => {
    api.getApplicationRSVP.mockRejectedValue(new ApiError(404, { code: "rsvp_not_available" }));
    render(<ApplicantRSVP applicationId="application" />);
    expect((await screen.findByRole("alert")).textContent).toContain("no longer available");
    expect(screen.queryByRole("button", { name: "Confirm attendance" })).toBeNull();
  });
});
