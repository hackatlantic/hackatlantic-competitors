import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ScannerWorkflow } from "@/components/scanner-workflow";
import { ApiError } from "@/lib/api";

const auth = vi.hoisted(() => ({ getToken: vi.fn(), isLoaded: true, userId: "scanner" as string | null }));
const api = vi.hoisted(() => ({ listScannerCheckpoints: vi.fn(), lookupScannerPass: vi.fn(), redeemScannerPass: vi.fn() }));
const camera = vi.hoisted(() => ({ start: vi.fn(), stop: vi.fn(), capture: null as null | ((result: { getText: () => string }) => void) }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => auth }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));
vi.mock("@zxing/browser", () => ({ BrowserQRCodeReader: class { decodeFromConstraints = camera.start; } }));
const point = { id: "entrance", name: "Main entrance" };
const token = "synthetic-test-pass";

async function scan() {
  fireEvent.click(await screen.findByRole("button", { name: "Scan ticket" }));
  await waitFor(() => expect(camera.capture).not.toBeNull());
  await act(async () => camera.capture!({ getText: () => token }));
}

describe("Volunteer check-in", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks(); auth.userId = "scanner"; camera.capture = null;
    api.listScannerCheckpoints.mockResolvedValue({ items: [point], nextCursor: null });
    api.lookupScannerPass.mockResolvedValue({ attendee: { displayName: "Alex Morgan" }, pass: { status: "active" } });
    api.redeemScannerPass.mockResolvedValue({ outcome: "redeemed", attendee: { displayName: "Alex Morgan" } });
    camera.start.mockImplementation(async (_constraints, _video, callback) => { camera.capture = callback; return { stop: camera.stop }; });
  });

  it("auto-selects the only point and verifies once per camera capture without recording entry", async () => {
    render(<ScannerWorkflow />);
    await scan();
    await act(async () => camera.capture!({ getText: () => token }));
    expect(api.lookupScannerPass).toHaveBeenCalledTimes(1);
    expect(api.lookupScannerPass).toHaveBeenCalledWith({ qrToken: token });
    expect(api.redeemScannerPass).not.toHaveBeenCalled();
    expect(screen.getByText("Ready to check in")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Checked in" })).toBeNull();
    expect((screen.getByLabelText("Scan point") as HTMLSelectElement).value).toBe("entrance");
    fireEvent.click(screen.getByRole("button", { name: "Check in at Main entrance" }));
    await screen.findByRole("heading", { name: "Checked in" });
    expect(api.redeemScannerPass).toHaveBeenCalledWith(expect.objectContaining({ qrToken: token, checkpointId: "entrance" }));
    fireEvent.click(screen.getByRole("button", { name: "Scan next attendee" }));
    await waitFor(() => expect(camera.start).toHaveBeenCalledTimes(2));
  });

  it("requires choosing a point when more than one exists", async () => {
    api.listScannerCheckpoints.mockResolvedValue({ items: [point, { id: "lunch", name: "Lunch" }], nextCursor: null });
    render(<ScannerWorkflow />);
    const button = await screen.findByRole("button", { name: "Scan ticket" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Scan point"), { target: { value: "lunch" } });
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });

  it("allows manual verification and prevents repeat clicks while recording", async () => {
    let complete!: (value: unknown) => void;
    api.redeemScannerPass.mockReturnValue(new Promise((resolve) => { complete = resolve; }));
    render(<ScannerWorkflow />);
    await screen.findByRole("button", { name: "Scan ticket" });
    fireEvent.click(screen.getByText("Camera not working? Enter a code"));
    fireEvent.change(screen.getByLabelText("QR code"), { target: { value: token } });
    fireEvent.click(screen.getByRole("button", { name: "Verify code" }));
    const checkIn = await screen.findByRole("button", { name: "Check in at Main entrance" });
    fireEvent.click(checkIn); fireEvent.click(checkIn);
    expect(api.redeemScannerPass).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("heading", { name: "Checked in" })).toBeNull();
    await act(async () => complete({ outcome: "redeemed" }));
    await screen.findByRole("heading", { name: "Checked in" });
  });

  it("retries an uncertain check-in with the SAME idempotency key", async () => {
    api.redeemScannerPass.mockRejectedValueOnce(new Error("offline"));
    render(<ScannerWorkflow />); await scan();
    fireEvent.click(screen.getByRole("button", { name: "Check in at Main entrance" }));
    fireEvent.click(await screen.findByRole("button", { name: "Retry check-in" }));
    await screen.findByRole("heading", { name: "Checked in" });
    expect(api.redeemScannerPass.mock.calls[1][0]).toEqual(api.redeemScannerPass.mock.calls[0][0]);
  });

  it.each(["revoked_pass", "invalid_pass"])("does not offer check-in for %s", async (code) => {
    api.lookupScannerPass.mockRejectedValue(new ApiError(404, { code }));
    render(<ScannerWorkflow />); await scan();
    expect(screen.queryByRole("button", { name: "Check in at Main entrance" })).toBeNull();
    expect(api.redeemScannerPass).not.toHaveBeenCalled();
  });

  it("shows an already-used pass as a rejection, not a new check-in", async () => {
    api.redeemScannerPass.mockResolvedValue({ outcome: "already_exhausted" });
    render(<ScannerWorkflow />); await scan();
    fireEvent.click(screen.getByRole("button", { name: "Check in at Main entrance" }));
    await screen.findByRole("heading", { name: "Already checked in" });
    expect(screen.queryByRole("heading", { name: "Checked in" })).toBeNull();
  });

  it("stops the camera and removes verification on sign-out", async () => {
    const view = render(<ScannerWorkflow />); await scan();
    auth.userId = null; view.rerender(<ScannerWorkflow />);
    expect(screen.queryByText("Alex Morgan")).toBeNull();
    expect(screen.queryByRole("button", { name: "Check in at Main entrance" })).toBeNull();
    expect(camera.stop).toHaveBeenCalled();
  });

  it("preserves authorization failures", async () => {
    api.listScannerCheckpoints.mockRejectedValue(new ApiError(403, { code: "forbidden" }));
    render(<ScannerWorkflow />);
    await screen.findByRole("heading", { name: "Scanner access required" });
    expect(screen.queryByRole("button", { name: "Scan ticket" })).toBeNull();
  });
});
