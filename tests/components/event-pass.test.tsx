import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EventPass } from "@/components/event-pass";
import { ApplicantPass } from "@/components/applicant-pass";
import { ApiError } from "@/lib/api";

const auth = vi.hoisted(() => ({ getToken: vi.fn(), isLoaded: true, userId: "user-one" as string | null }));
const api = vi.hoisted(() => ({ getAttendeePass: vi.fn(), ensureStaffPass: vi.fn() }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => auth }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));
const pass = { id: "pass-one", attendeeId: "attendee", displayName: "Test Attendee", status: "active", issuedAt: "2026-09-17T12:00:00Z", qrToken: "ha_qr_v1_test-ticket-not-a-real-credential" };

describe("Event pass", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.clearAllMocks();
    auth.userId = "user-one";
    auth.isLoaded = true;
    api.getAttendeePass.mockResolvedValue(pass);
    api.ensureStaffPass.mockRejectedValue(new ApiError(404, { code: "pass_not_found" }));
  });

  it("only links from the dashboard after confirming availability, without embedding a QR", async () => {
    render(<ApplicantPass />);
    expect(screen.queryByRole("link", { name: /View event pass/ })).toBeNull();
    expect((await screen.findByRole("link", { name: /View event pass/ })).getAttribute("href")).toBe("/event-pass");
    expect(screen.queryByRole("img", { name: /QR code/ })).toBeNull();
  });

  it("keeps the dashboard in waiting state until a pass is released", async () => {
    api.getAttendeePass.mockRejectedValueOnce(new ApiError(404, { code: "pass_not_found" }));
    render(<ApplicantPass />);
    await screen.findByText(/Your pass will appear here/);
    expect(screen.queryByRole("link", { name: /View event pass/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Check for pass" }));
    await screen.findByRole("link", { name: /View event pass/ });
  });

  it("does not describe a pass lookup failure as an unreleased pass", async () => {
    api.getAttendeePass.mockRejectedValueOnce(new Error("offline"));
    render(<ApplicantPass />);
    await screen.findByText(/couldn’t check your pass/);
    expect(screen.queryByRole("link", { name: /View event pass/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByRole("link", { name: /View event pass/ });
  });

  it("renders the issued ticket, attendee and check-in logistics", async () => {
    render(<EventPass />);
    expect(screen.getByText("Getting your pass…")).toBeTruthy();
    await screen.findByRole("heading", { name: "You’re in." });
    expect(screen.getByText("Test Attendee")).toBeTruthy();
    expect(screen.getByText("UNB · Head Hall Atrium")).toBeTruthy();
    expect(screen.getByText("Sat, Sep 26")).toBeTruthy();
    expect(screen.getByRole("img", { name: /QR code/ })).toBeTruthy();
    expect(screen.queryByText(pass.qrToken)).toBeNull();
  });

  it("automatically loads a named volunteer pass without an application", async () => {
    api.ensureStaffPass.mockResolvedValue({ ...pass, kind: "volunteer", expiresAt: "2026-09-27T18:00:00Z" });
    render(<EventPass />);
    await screen.findByText("Volunteer");
    expect(screen.getByText("Staff entry & overnight re-entry")).toBeTruthy();
    expect(screen.getByText("Sun, Sep 27 · 3 PM ADT")).toBeTruthy();
    expect(api.getAttendeePass).not.toHaveBeenCalled();
  });

  it("does not fall back to an attendee ticket when staff verification fails", async () => {
    api.ensureStaffPass.mockRejectedValue(new ApiError(503, { code: "unavailable" }));
    render(<EventPass />);
    await screen.findByText("Let’s try that again.");
    expect(api.getAttendeePass).not.toHaveBeenCalled();
  });

  it("shows no ticket for an unissued pass and supports checking again", async () => {
    api.getAttendeePass.mockRejectedValueOnce(new ApiError(404, { code: "pass_not_found" }));
    render(<EventPass />);
    await screen.findByText("Your pass is on its way.");
    expect(screen.queryByRole("img", { name: /QR code/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Check again" }));
    await screen.findByRole("heading", { name: "You’re in." });
  });

  it("does not mistake a server failure for an unissued pass", async () => {
    api.getAttendeePass.mockRejectedValueOnce(new ApiError(503, { code: "unavailable" }));
    render(<EventPass />);
    await screen.findByText("Let’s try that again.");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByRole("img", { name: /QR code/ });
  });

  it("never renders a revoked pass returned unexpectedly", async () => {
    api.getAttendeePass.mockResolvedValue({ ...pass, status: "revoked" });
    render(<EventPass />);
    await screen.findByText("Your pass is on its way.");
    expect(screen.queryByRole("img", { name: /QR code/ })).toBeNull();
  });

  it("hides a loaded QR immediately on sign-out", async () => {
    const view = render(<EventPass />);
    await screen.findByRole("img", { name: /QR code/ });
    auth.userId = null;
    view.rerender(<EventPass />);
    expect(screen.queryByRole("img", { name: /QR code/ })).toBeNull();
    expect(screen.getByText("Sign in to view your pass")).toBeTruthy();
  });

  it("hides the previous account's ticket while another account loads", async () => {
    const view = render(<EventPass />);
    await screen.findByRole("img", { name: /QR code/ });
    api.getAttendeePass.mockReturnValue(new Promise(() => {}));
    auth.userId = "user-two";
    view.rerender(<EventPass />);
    expect(screen.queryByRole("img", { name: /QR code/ })).toBeNull();
  });
});
