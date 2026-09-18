import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OrganizerEventOperations } from "@/components/organizer-event-operations";

const api = vi.hoisted(() => ({ listOrganizerRedemptions: vi.fn(), listOrganizerApplications: vi.fn(), getOrganizerApplication: vi.fn(), updateOrganizerAttendeeEntitlement: vi.fn() }));
const getToken = vi.hoisted(() => vi.fn());
const router = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken }) }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));
const checkpoint = { id: "entrance", cycleId: "cycle", name: "Main entrance", slug: "entry", defaultAllowed: true, defaultMaxRedemptions: 1, active: true };
const count = { checkpointId: "entrance", checkpointName: "Main entrance", totalRedemptions: 20, uniqueAttendees: 12, confirmedRsvps: 30 };
function show(counts = [count]) { return render(<OrganizerEventOperations initialActivities={[]} initialCheckpoints={[checkpoint]} initialCounts={counts} currentCycleId="cycle" />); }

describe("Event operations", () => {
  afterEach(cleanup);
  beforeEach(() => { vi.resetAllMocks(); api.listOrganizerRedemptions.mockResolvedValue({ items: [] }); });

  it("defaults to event day and hides setup and exceptions", async () => {
    show(); await screen.findByText(/No check-ins in the latest activity/);
    expect(screen.getByRole("link", { name: "Open scanner" }).getAttribute("href")).toBe("/scanner");
    expect(screen.getByRole("link", { name: "Manage volunteers" }).getAttribute("href")).toBe("/organizer/reviewers");
    expect(screen.queryByRole("heading", { name: "Entrance & scan points" })).toBeNull();
    expect(screen.getByText("12")).toBeTruthy(); expect(screen.getByText("30")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Event setup" }));
    expect(screen.getByRole("heading", { name: "Entrance & scan points" })).toBeTruthy();
    expect(screen.getByText("Find an attendee’s access settings").closest("details")?.open).toBe(false);
    expect(screen.queryByRole("heading", { name: "Check-in activity" })).toBeNull();
  });

  it("does not substitute scan counts when the older API lacks unique totals", async () => {
    render(<OrganizerEventOperations initialActivities={[]} initialCheckpoints={[checkpoint]} initialCounts={[{ checkpointId: "entrance", checkpointName: "Main entrance", totalRedemptions: 20 }]} />);
    await screen.findByText(/Attendance totals are not available/);
    expect(screen.getAllByText("—").length).toBe(2);
  });

  it("reports recent activity failures without hiding the scanner link", async () => {
    api.listOrganizerRedemptions.mockRejectedValue(new Error("offline"));
    show(); await screen.findByRole("alert");
    expect(screen.getByRole("link", { name: "Open scanner" })).toBeTruthy();
  });

  it("finds an accepted attendee by email and links to access settings on their record", async () => {
    api.listOrganizerApplications.mockResolvedValue({ items: [{ id: "application", applicant: { displayName: "Alex Morgan", email: "alex@example.test" } }], nextCursor: null });
    show();
    fireEvent.click(screen.getByRole("button", { name: "Event setup" }));
    fireEvent.click(screen.getByText("Find an attendee’s access settings"));
    fireEvent.change(screen.getByLabelText("Name or email"), { target: { value: "alex@example.test" } });
    fireEvent.click(screen.getByRole("button", { name: "Find attendee" }));
    expect((await screen.findByRole("link", { name: /Alex Morgan · alex/ })).getAttribute("href")).toBe("/organizer/applications/application#access-settings");
    expect(api.listOrganizerApplications).toHaveBeenCalledWith({ status: "accepted", q: "alex@example.test" });
    expect(api.updateOrganizerAttendeeEntitlement).not.toHaveBeenCalled();
  });
});
