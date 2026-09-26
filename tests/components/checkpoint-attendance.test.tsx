import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CheckpointAttendance } from "@/components/checkpoint-attendance";
import { OrganizerEventOperations } from "@/components/organizer-event-operations";
import { type OrganizerCheckpoint } from "@/lib/api";

const api = vi.hoisted(() => ({ listOrganizerRedemptions: vi.fn(), getOrganizerAttendanceSummary: vi.fn() }));
const getToken = vi.hoisted(() => vi.fn());
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken }) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => api }));
const entrance: OrganizerCheckpoint = { id: "entrance", cycleId: "cycle", slug: "main-entrance", name: "Main entrance", active: true, defaultAllowed: true, defaultMaxRedemptions: 1 };
const lunch = { ...entrance, id: "lunch", slug: "saturday-lunch", name: "Saturday lunch" };
function row(id: string, point = "entrance", person = id) {
  return { id, checkpoint: { id: point }, attendee: { id: person, displayName: `Person ${person}` }, redeemedAt: "2026-09-26T13:00:00Z" };
}
describe("Checkpoint attendance", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks(); api.listOrganizerRedemptions.mockResolvedValue({ items: [] });
    api.getOrganizerAttendanceSummary.mockResolvedValue({ cycleId: "cycle", confirmedRsvps: 120 });
  });
  it("defaults to the actual main entrance when meals are also configured", async () => {
    render(<OrganizerEventOperations initialCheckpoints={[lunch, entrance]} initialCounts={[]} />);
    await screen.findByRole("heading", { name: "Who’s checked in · Main entrance" });
    await waitFor(() => expect(api.listOrganizerRedemptions).toHaveBeenCalledWith({ checkpointId: "entrance", limit: 500 }));
  });
  it("shows more than five people, deduplicates them and searches within the checkpoint", async () => {
    api.listOrganizerRedemptions.mockResolvedValue({ items: [...Array.from({ length: 12 }, (_, index) => row(String(index))), row("repeat", "entrance", "0"), row("other", "lunch")] });
    render(<CheckpointAttendance checkpoint={entrance} refreshKey={0} onRefresh={vi.fn()} />);
    await screen.findByText("Person 11");
    expect(screen.getAllByRole("listitem")).toHaveLength(12);
    expect(screen.queryByText("Person other")).toBeNull();
    expect(screen.getAllByText(/Sep 26.*10:00 AM/)).toHaveLength(12);
    fireEvent.change(screen.getByLabelText("Search checked-in names"), { target: { value: "  PERSON 11 " } });
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    fireEvent.change(screen.getByLabelText("Search checked-in names"), { target: { value: "missing" } });
    expect(screen.getByText("No matching names in this attendance list.")).toBeTruthy();
  });
  it("does not label a 500-scan capped response as complete", async () => {
    api.listOrganizerRedemptions.mockResolvedValue({ items: Array.from({ length: 500 }, (_, index) => row(String(index))) });
    render(<CheckpointAttendance checkpoint={entrance} refreshKey={0} onRefresh={vi.fn()} />);
    await screen.findByText(/may omit older check-ins/);
  });
  it("ignores an old checkpoint response after switching to a meal", async () => {
    let resolve!: (data: unknown) => void;
    api.listOrganizerRedemptions.mockReturnValueOnce(new Promise((done) => { resolve = done; })).mockResolvedValueOnce({ items: [row("meal", "lunch")] });
    render(<OrganizerEventOperations initialCheckpoints={[entrance, lunch]} initialCounts={[]} />);
    await waitFor(() => expect(api.listOrganizerRedemptions).toHaveBeenCalledTimes(1));
    fireEvent.change(screen.getByLabelText("Show check-ins for"), { target: { value: "lunch" } });
    await screen.findByText("Person meal");
    await act(async () => resolve({ items: [row("old")] }));
    expect(screen.queryByText("Person old")).toBeNull();
    expect(api.listOrganizerRedemptions).toHaveBeenLastCalledWith({ checkpointId: "lunch", limit: 500 });
  });
  it("hides old rows on refresh until the new response arrives", async () => {
    api.listOrganizerRedemptions.mockResolvedValueOnce({ items: [row("old")] }).mockResolvedValueOnce({ items: [row("new")] });
    const view = render(<CheckpointAttendance checkpoint={entrance} refreshKey={0} onRefresh={vi.fn()} />);
    await screen.findByText("Person old");
    view.rerender(<CheckpointAttendance checkpoint={entrance} refreshKey={1} onRefresh={vi.fn()} />);
    expect(screen.queryByText("Person old")).toBeNull();
    await screen.findByText("Person new");
  });
  it("does not issue an unfiltered cross-checkpoint request", () => {
    render(<CheckpointAttendance checkpoint={null} refreshKey={0} onRefresh={vi.fn()} />);
    expect(api.listOrganizerRedemptions).not.toHaveBeenCalled();
  });
});

it("sends checkpoint and limit through the real API client", async () => {
  const { createApiClient: actualClient } = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [] }), { status: 200 }));
  const originalFetch = globalThis.fetch;
  globalThis.fetch = fetcher;
  try {
    await actualClient({ getToken: async () => "synthetic-test-token" }).listOrganizerRedemptions({ checkpointId: "entrance", limit: 500 });
    expect(String(fetcher.mock.calls[0][0])).toContain("/v1/admin/redemptions?checkpointId=entrance&limit=500");
  } finally { globalThis.fetch = originalFetch; }
});
