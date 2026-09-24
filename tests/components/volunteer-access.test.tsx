import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { VolunteerSignup, VolunteerApprovalQueue } from "@/components/volunteer-access";
import { type VolunteerRequest } from "@/lib/api";

const mocks = vi.hoisted(() => ({ getToken: vi.fn(), getVolunteerAccess: vi.fn(), requestVolunteerAccess: vi.fn(), listVolunteerRequests: vi.fn(), reviewVolunteerRequest: vi.fn() }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken: mocks.getToken }) }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => mocks }));
const volunteer: VolunteerRequest = { userId: "test-account", realName: "Alex Morgan", email: "alex@example.test", status: "pending", scannerAccess: false, createdAt: "2026-09-24T12:00:00Z", reviewedAt: null };

describe("volunteer requests", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.getVolunteerAccess.mockResolvedValue({ request: null, scannerAccess: false });
    mocks.requestVolunteerAccess.mockResolvedValue({ request: volunteer, scannerAccess: false });
    mocks.listVolunteerRequests.mockResolvedValue({ items: [volunteer] });
    mocks.reviewVolunteerRequest.mockResolvedValue(undefined);
  });
  it("requests by name without an application, email entry or automatic access", async () => {
    render(<VolunteerSignup />);
    fireEvent.change(await screen.findByLabelText("Your name on the volunteer schedule"), { target: { value: "Alex Morgan" } });
    expect(screen.queryByLabelText(/email/i)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Request access" }));
    await screen.findByRole("heading", { name: "Request sent" });
    expect(mocks.requestVolunteerAccess).toHaveBeenCalledWith("Alex Morgan");
    expect(screen.queryByRole("link", { name: "Open scanner" })).toBeNull();
    expect(mocks.reviewVolunteerRequest).not.toHaveBeenCalled();
  });
  it("refreshes approval and opens the scanner only when actual access is active", async () => {
    mocks.getVolunteerAccess.mockResolvedValueOnce({ request: volunteer, scannerAccess: false });
    render(<VolunteerSignup />);
    await screen.findByRole("heading", { name: "Request sent" });
    mocks.getVolunteerAccess.mockResolvedValueOnce({ request: { ...volunteer, status: "approved" }, scannerAccess: true });
    fireEvent.click(screen.getByRole("button", { name: "Check approval" }));
    expect((await screen.findByRole("link", { name: "Open scanner" })).getAttribute("href")).toBe("/scanner");
  });
  it.each(["approved", "rejected", "revoked"])("does not infer scanner access from request status %s", async (status) => {
    mocks.getVolunteerAccess.mockResolvedValue({ request: { ...volunteer, status }, scannerAccess: false });
    render(<VolunteerSignup />);
    await screen.findByRole("heading", { name: "Contact your volunteer lead" });
    expect(screen.queryByRole("link", { name: "Open scanner" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Request access" })).toBeNull();
  });
  it("offers retry rather than a request form when status is unavailable", async () => {
    mocks.getVolunteerAccess.mockRejectedValue(new Error("Unavailable"));
    render(<VolunteerSignup />);
    await screen.findByRole("alert");
    expect(screen.queryByRole("button", { name: "Request access" })).toBeNull();
    expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
  });
  it("requires account verification before admin approval and uses the selected account", async () => {
    mocks.listVolunteerRequests.mockResolvedValue({ items: [volunteer, { ...volunteer, userId: "second-account", email: "other@example.test" }] });
    render(<VolunteerApprovalQueue />);
    const cards = await screen.findAllByRole("article", { name: "Request from Alex Morgan" });
    const first = within(cards[0]); const second = within(cards[1]);
    expect((first.getByRole("button", { name: "Approve scanner access" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(first.getByRole("checkbox"));
    expect((second.getByRole("button", { name: "Approve scanner access" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(first.getByRole("button", { name: "Approve scanner access" }));
    await screen.findByText("Scanner access approved for Alex Morgan.");
    expect(mocks.reviewVolunteerRequest).toHaveBeenCalledWith("test-account", "approved", "pending");
    fireEvent.click(first.getByRole("button", { name: "Revoke scanner access" }));
    await screen.findByText("Request revoked for Alex Morgan.");
    expect(mocks.reviewVolunteerRequest).toHaveBeenLastCalledWith("test-account", "revoked", "approved");
  });
  it("declines without granting a role", async () => {
    render(<VolunteerApprovalQueue />);
    fireEvent.click(await screen.findByRole("button", { name: "Decline request" }));
    await screen.findByText("Request rejected for Alex Morgan.");
    expect(mocks.reviewVolunteerRequest).toHaveBeenCalledWith("test-account", "rejected", "pending");
  });
  it("clears stale approval controls on failure and requires refresh", async () => {
    mocks.reviewVolunteerRequest.mockRejectedValue(new Error("This request has changed."));
    render(<VolunteerApprovalQueue />);
    fireEvent.click(await screen.findByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Approve scanner access" }));
    await screen.findByRole("alert");
    expect(screen.queryByRole("button", { name: "Approve scanner access" })).toBeNull();
    expect(screen.queryByText(/Scanner access approved/)).toBeNull();
  });
  it("prevents duplicate approval clicks while saving", async () => {
    mocks.reviewVolunteerRequest.mockReturnValue(new Promise(() => {}));
    render(<VolunteerApprovalQueue />);
    fireEvent.click(await screen.findByRole("checkbox"));
    const button = screen.getByRole("button", { name: "Approve scanner access" });
    fireEvent.click(button); fireEvent.click(button);
    await waitFor(() => expect(mocks.reviewVolunteerRequest).toHaveBeenCalledTimes(1));
  });
});
