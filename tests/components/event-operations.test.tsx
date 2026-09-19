import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OrganizerEventOperations } from "@/components/organizer-event-operations";
import type { OrganizerRedemptionCount } from "@/lib/api";

const api = vi.hoisted(() => ({ listOrganizerRedemptions: vi.fn(), listOrganizerApplications: vi.fn(), updateOrganizerAttendeeEntitlement: vi.fn() }));
const getToken = vi.hoisted(() => vi.fn());
const router = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken }) }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));
const checkpoint = { id: "entrance", cycleId: "cycle", name: "Main entrance", slug: "entry", defaultAllowed: true, defaultMaxRedemptions: 1, active: true };
const lunch = { ...checkpoint, id: "lunch", name: "Saturday lunch", slug: "lunch" };
const count = { checkpointId: "entrance", checkpointName: "Main entrance", totalRedemptions: 20, uniqueAttendees: 12, confirmedRsvps: 30 };
function show(counts: OrganizerRedemptionCount[] = [count]) { return render(<OrganizerEventOperations initialCheckpoints={[checkpoint]} initialCounts={counts} />); }

describe("Simple check-in dashboard", () => {
  afterEach(cleanup);
  beforeEach(() => { vi.resetAllMocks(); api.listOrganizerRedemptions.mockResolvedValue({ items: [] }); });

  it("shows attendance, scanner, volunteers and search without setup or exports", async () => {
    show(); await screen.findByText(/No recent check-ins for Main entrance/);
    expect(screen.getByRole("link", { name: "Open scanner" }).getAttribute("href")).toBe("/scanner");
    expect(screen.getByRole("link", { name: "Manage volunteers" }).getAttribute("href")).toBe("/organizer/reviewers");
    expect(screen.getByRole("heading", { name: "Find an attendee" })).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy(); expect(screen.getByText("30")).toBeTruthy();
    expect(screen.queryByRole("combobox")).toBeNull();
    for (const label of [/Event day/i, /Event setup/i, /scan point/i, /export/i, /activity metadata/i, /redemption monitoring/i]) {
      expect(screen.queryByText(label)).toBeNull();
    }
  });

  it("does not substitute scan counts when the older API lacks unique totals", async () => {
    show([{ checkpointId: "entrance", checkpointName: "Main entrance", totalRedemptions: 20 }]);
    await screen.findByText(/Attendance totals are currently unavailable/);
    expect(screen.getAllByText("—").length).toBe(2);
    expect(screen.queryByText("20")).toBeNull();
  });

  it("reports recent activity failures without hiding the scanner link and can refresh", async () => {
    api.listOrganizerRedemptions.mockRejectedValueOnce(new Error("offline"));
    show(); await screen.findByRole("alert");
    expect(screen.getByRole("link", { name: "Open scanner" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    await screen.findByText(/No recent check-ins/);
    expect(router.refresh).toHaveBeenCalledOnce();
    expect(api.listOrganizerRedemptions).toHaveBeenCalledTimes(2);
  });

  it("finds an accepted attendee directly by email and opens their record", async () => {
    api.listOrganizerApplications.mockResolvedValue({ items: [{ id: "application", applicant: { displayName: "Alex Morgan", email: "alex@example.test" } }], nextCursor: null });
    show();
    fireEvent.change(screen.getByLabelText("Name or email"), { target: { value: "  alex@example.test  " } });
    fireEvent.submit(screen.getByLabelText("Name or email").closest("form")!);
    expect((await screen.findByRole("link", { name: /Alex Morgan alex/ })).getAttribute("href")).toBe("/organizer/applications/application");
    expect(api.listOrganizerApplications).toHaveBeenCalledWith({ status: "accepted", q: "alex@example.test" });
    expect(api.updateOrganizerAttendeeEntitlement).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Name or email"), { target: { value: "someone else" } });
    expect(screen.queryByRole("link", { name: /Alex Morgan/ })).toBeNull();
  });

  it("shows no-match and search errors without changing any access rules", async () => {
    api.listOrganizerApplications.mockResolvedValueOnce({ items: [], nextCursor: null }).mockRejectedValueOnce(new Error("offline"));
    show();
    expect((screen.getByRole("button", { name: "Search" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Name or email"), { target: { value: "unknown" } });
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    await screen.findByText(/No accepted attendees found/);
    fireEvent.click(screen.getByRole("button", { name: "Search" }));
    await screen.findByRole("alert");
    expect(api.updateOrganizerAttendeeEntitlement).not.toHaveBeenCalled();
  });

  it("keeps meal and entrance counts separate and limits recent rows to the selection", async () => {
    api.listOrganizerRedemptions.mockResolvedValue({ items: [
      { id: "one", checkpoint: { id: "entrance" }, attendee: { displayName: "Entrance attendee" }, redeemedAt: "2026-09-26T12:00:00Z" },
      { id: "two", checkpoint: { id: "lunch" }, attendee: { displayName: "Lunch attendee" }, redeemedAt: "2026-09-26T16:00:00Z" },
    ] });
    render(<OrganizerEventOperations initialCheckpoints={[checkpoint, lunch]} initialCounts={[count, { ...count, checkpointId: "lunch", uniqueAttendees: 7 }]} />);
    expect(screen.queryByText("12")).toBeNull();
    expect(screen.queryByText("Entrance attendee")).toBeNull();
    fireEvent.change(screen.getByLabelText("Show check-ins for"), { target: { value: "lunch" } });
    await screen.findByText("Lunch attendee");
    expect(screen.getByText("7")).toBeTruthy();
    expect(screen.queryByText("12")).toBeNull();
    expect(screen.queryByText("Entrance attendee")).toBeNull();
    fireEvent.change(screen.getByLabelText("Show check-ins for"), { target: { value: "entrance" } });
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.queryByText("Lunch attendee")).toBeNull();
  });

  it("does not invent scanning choices when none are configured", async () => {
    render(<OrganizerEventOperations initialCheckpoints={[]} initialCounts={[]} />);
    await screen.findByText(/Check-in hasn’t been configured/);
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.getAllByText("—").length).toBe(2);
  });

  it("uses refreshed server totals without retaining an outdated snapshot", async () => {
    const view = show();
    view.rerender(<OrganizerEventOperations initialCheckpoints={[checkpoint]} initialCounts={[{ ...count, uniqueAttendees: 13 }]} />);
    await waitFor(() => expect(screen.getByText("13")).toBeTruthy());
    expect(screen.queryByText("12")).toBeNull();
  });
});
