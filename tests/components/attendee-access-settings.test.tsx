import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AttendeeAccessSettings } from "@/components/attendee-access-settings";
const api = vi.hoisted(() => ({ listOrganizerCheckpoints: vi.fn(), getOrganizerAttendeeEntitlement: vi.fn(), updateOrganizerAttendeeEntitlement: vi.fn(), deleteOrganizerAttendeeEntitlement: vi.fn() }));
const getToken = vi.hoisted(() => vi.fn());
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken }) }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));
const point = { id: "entry", cycleId: "cycle", name: "Entrance", defaultAllowed: true, defaultMaxRedemptions: 1 };
async function show() {
  render(<AttendeeAccessSettings attendeeId="person" cycleId="cycle" displayName="Alex Morgan" />);
  fireEvent.click(screen.getByText("Manage an access exception"));
  await screen.findByRole("option", { name: "Entrance" });
  fireEvent.change(screen.getByLabelText("Scan point"), { target: { value: "entry" } });
  await screen.findByText(/Using the scan point/);
}
describe("Attendee access exceptions", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks();
    api.listOrganizerCheckpoints.mockResolvedValue({ items: [point, { ...point, id: "old", name: "Old event", cycleId: "old-cycle" }] });
    api.getOrganizerAttendeeEntitlement.mockResolvedValue({ override: null });
    api.updateOrganizerAttendeeEntitlement.mockResolvedValue({ attendeeId: "person", checkpointId: "entry", allowed: true, maxRedemptions: 2 });
    api.deleteOrganizerAttendeeEntitlement.mockResolvedValue(undefined);
  });
  it("uses the attendee record and same event's points; requires an explicit confirmation", async () => {
    await show();
    expect(screen.queryByRole("option", { name: "Old event" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Total allowed uses"), { target: { value: "2" } });
    fireEvent.click(screen.getByRole("button", { name: "Save exception" }));
    expect(api.updateOrganizerAttendeeEntitlement).not.toHaveBeenCalled();
    expect(screen.getByText(/Allow 2 total uses for Alex Morgan at Entrance/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Confirm change" }));
    await screen.findByText("Access exception saved.");
    expect(api.updateOrganizerAttendeeEntitlement).toHaveBeenCalledWith("person", "entry", { allowed: true, maxRedemptions: 2 });
    fireEvent.click(screen.getByRole("button", { name: "Use usual rules" }));
    expect(api.deleteOrganizerAttendeeEntitlement).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Confirm change" }));
    await waitFor(() => expect(api.deleteOrganizerAttendeeEntitlement).toHaveBeenCalledWith("person", "entry"));
  });
  it("does not permit saving before loading current access", async () => {
    api.getOrganizerAttendeeEntitlement.mockRejectedValue(new Error("offline"));
    render(<AttendeeAccessSettings attendeeId="person" cycleId="cycle" displayName="Alex" />);
    fireEvent.click(screen.getByText("Manage an access exception"));
    await screen.findByRole("option", { name: "Entrance" });
    fireEvent.change(screen.getByLabelText("Scan point"), { target: { value: "entry" } });
    await screen.findByRole("alert");
    expect(screen.queryByRole("button", { name: "Save exception" })).toBeNull();
  });
  it("preserves denied access and zero uses without sending response metadata back", async () => {
    api.getOrganizerAttendeeEntitlement.mockResolvedValue({ override: { attendeeId: "person", checkpointId: "entry", allowed: false, maxRedemptions: 0 } });
    render(<AttendeeAccessSettings attendeeId="person" cycleId="cycle" displayName="Alex Morgan" />);
    fireEvent.click(screen.getByText("Manage an access exception"));
    await screen.findByRole("option", { name: "Entrance" });
    fireEvent.change(screen.getByLabelText("Scan point"), { target: { value: "entry" } });
    await screen.findByText("This person has a custom rule.");
    expect((screen.getByLabelText("Allow access") as HTMLInputElement).checked).toBe(false);
    expect((screen.getByLabelText("Total allowed uses") as HTMLInputElement).value).toBe("0");
    fireEvent.click(screen.getByRole("button", { name: "Save exception" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm change" }));
    await screen.findByText("Access exception saved.");
    expect(api.updateOrganizerAttendeeEntitlement).toHaveBeenCalledWith("person", "entry", { allowed: false, maxRedemptions: 0 });
  });
  it("rejects empty or fractional allowances without calling the API", async () => {
    await show();
    fireEvent.change(screen.getByLabelText("Total allowed uses"), { target: { value: "1.5" } });
    fireEvent.click(screen.getByRole("button", { name: "Save exception" }));
    await screen.findByRole("alert");
    expect(api.updateOrganizerAttendeeEntitlement).not.toHaveBeenCalled();
  });
});
