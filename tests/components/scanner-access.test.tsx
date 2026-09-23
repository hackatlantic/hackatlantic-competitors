import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ScannerRoleForm } from "@/components/organizer-reviewer-actions";
import { ApiError, type ScannerAccessUser } from "@/lib/api";

const mocks = vi.hoisted(() => ({ getToken: vi.fn(), lookupScannerUser: vi.fn(), grantScannerRole: vi.fn(), revokeScannerRole: vi.fn() }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken: mocks.getToken }) }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => mocks }));
vi.mock("framer-motion", async (original) => ({ ...(await original<typeof import("framer-motion")>()), useReducedMotion: () => true }));
const volunteer: ScannerAccessUser = { id: "hidden-local-uuid", email: "volunteer@example.test", displayName: "Test Volunteer", scannerAccess: false, canManage: true };

describe("Scanner access by email", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.lookupScannerUser.mockResolvedValue(volunteer);
    mocks.grantScannerRole.mockResolvedValue(undefined);
    mocks.revokeScannerRole.mockResolvedValue(undefined);
  });

  async function findVolunteer() {
    fireEvent.change(screen.getByLabelText("Volunteer email"), { target: { value: volunteer.email } });
    fireEvent.click(screen.getByRole("button", { name: "Find volunteer" }));
    await screen.findByRole("region", { name: "Volunteer account" });
  }

  it("looks up by email, confirms the account, then grants and revokes without UUID entry", async () => {
    render(<ScannerRoleForm />);
    expect(screen.queryByLabelText("Local user ID")).toBeNull();
    await findVolunteer();
    expect(mocks.lookupScannerUser).toHaveBeenCalledWith(volunteer.email);
    expect(screen.getByText("Test Volunteer")).toBeTruthy();
    expect(screen.queryByText(volunteer.id)).toBeNull();
    expect(mocks.grantScannerRole).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Grant scanner role" }));
    await screen.findByText("Scanner role granted.");
    expect(mocks.grantScannerRole).toHaveBeenCalledWith(volunteer.id);
    expect(screen.getByText("Scanner access is active.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Revoke scanner role" }));
    await screen.findByText("Scanner role revoked.");
    expect(mocks.revokeScannerRole).toHaveBeenCalledWith(volunteer.id);
    expect(screen.getByText("No scanner access yet.")).toBeTruthy();
  });

  it("invalidates the previous selection when the email changes", async () => {
    render(<ScannerRoleForm />);
    await findVolunteer();
    fireEvent.change(screen.getByLabelText("Volunteer email"), { target: { value: "other@example.test" } });
    expect(screen.queryByRole("region", { name: "Volunteer account" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Grant scanner role" })).toBeNull();
    expect(mocks.grantScannerRole).not.toHaveBeenCalled();
  });

  it.each([
    new ApiError(404, { message: "Ask the volunteer to sign up and open HackAtlantic once." }),
    new ApiError(409, { message: "More than one account matches this email." }),
    new ApiError(503, { message: "Unable to verify this account right now." }),
  ])("explains lookup failures without exposing a grant action", async (error) => {
    mocks.lookupScannerUser.mockRejectedValueOnce(error);
    render(<ScannerRoleForm />);
    fireEvent.change(screen.getByLabelText("Volunteer email"), { target: { value: volunteer.email } });
    fireEvent.click(screen.getByRole("button", { name: "Find volunteer" }));
    expect((await screen.findByRole("alert")).textContent).toBe(error.message);
    expect(screen.queryByRole("button", { name: "Grant scanner role" })).toBeNull();
  });

  it("shows inherited admin access without offering a misleading revoke action", async () => {
    mocks.lookupScannerUser.mockResolvedValueOnce({ ...volunteer, scannerAccess: true, canManage: false });
    render(<ScannerRoleForm />);
    await findVolunteer();
    expect(screen.getByText(/Admin accounts already have scanner access/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Revoke scanner role" })).toBeNull();
  });

  it("prevents repeated clicks during a mutation and preserves state on failure", async () => {
    let rejectGrant!: (error: Error) => void;
    mocks.grantScannerRole.mockImplementationOnce(() => new Promise((_, reject) => { rejectGrant = reject; }));
    render(<ScannerRoleForm />);
    await findVolunteer();
    fireEvent.click(screen.getByRole("button", { name: "Grant scanner role" }));
    const pending = screen.getByRole("button", { name: "Granting…" });
    expect(pending.getAttribute("aria-busy")).toBe("true");
    expect((screen.getByLabelText("Volunteer email") as HTMLInputElement).disabled).toBe(true);
    fireEvent.click(pending);
    expect(mocks.grantScannerRole).toHaveBeenCalledTimes(1);
    await act(async () => rejectGrant(new Error("Access could not be changed.")));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("Access could not be changed."));
    expect(screen.getByText("No scanner access yet.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Grant scanner role" })).toBeTruthy();
  });
});
