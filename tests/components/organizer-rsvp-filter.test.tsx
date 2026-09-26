import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import OrganizerApplicationsPage from "@/app/organizer/applications/page";
import { ApiError, type AttendanceRSVP } from "@/lib/api";
import { formatRSVPTime, isLateRSVP } from "@/lib/rsvp-deadline";

const mocks = vi.hoisted(() => ({ auth: vi.fn(), list: vi.fn(), redirect: vi.fn() }));
vi.mock("@clerk/nextjs/server", () => ({ auth: mocks.auth }));
vi.mock("next/navigation", () => ({ redirect: mocks.redirect }));
vi.mock("@/lib/api", async (original) => ({ ...(await original<typeof import("@/lib/api")>()), createApiClient: () => ({ listOrganizerApplications: mocks.list }) }));

function response(respondedAt?: string, status: AttendanceRSVP["status"] = "confirmed"): AttendanceRSVP {
  return { applicationId: "application", decisionId: "decision", lockVersion: 1, status, respondedAt };
}
const records = [
  { id: "ontime", applicant: { displayName: "On Time", email: "ontime@example.com" }, status: "accepted", rsvp: response("2026-09-23T02:59:59.999Z") },
  { id: "late", applicant: { displayName: "Late Arrival", email: "late@example.com" }, status: "accepted", rsvp: response("2026-09-23T03:00:00Z") },
  { id: "declined", applicant: { displayName: "Declined Person", email: "declined@example.com" }, status: "accepted", rsvp: response("2026-09-24T10:00:00Z", "declined") },
  { id: "pending", applicant: { displayName: "Pending Person", email: "pending@example.com" }, status: "accepted", rsvp: response(undefined, "pending") },
  { id: "unknown", applicant: { displayName: "Unknown Time", email: "unknown@example.com" }, status: "accepted", rsvp: response() },
];

describe("RSVP deadline boundary", () => {
  it.each([
    ["2026-09-22T23:59:59.999-03:00", false],
    ["2026-09-23T02:59:59.999Z", false],
    ["2026-09-23T00:00:00-03:00", true],
    ["2026-09-23T03:00:00Z", true],
    ["2026-09-22T23:00:00-04:00", true],
    ["2026-09-26T09:00:00Z", true],
    ["invalid", false],
    [undefined, false],
  ])("classifies %s in the event timezone", (value, expected) => {
    expect(isLateRSVP(response(value))).toBe(expected);
  });
  it("excludes non-confirmations and missing responses", () => {
    expect(isLateRSVP(response("2026-09-24T10:00:00Z", "declined"))).toBe(false);
    expect(isLateRSVP(response("2026-09-24T10:00:00Z", "pending"))).toBe(false);
    expect(isLateRSVP()).toBe(false);
  });
  it("formats in Atlantic time regardless of the server timezone", () => {
    expect(formatRSVPTime("2026-09-23T02:00:00Z")).toMatch(/Sep 22, 2026.*11:00 PM/);
    expect(formatRSVPTime("invalid")).toBeNull();
    expect(formatRSVPTime()).toBeNull();
  });
});

describe("Admin late RSVP filter", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.auth.mockResolvedValue({ userId: "admin", getToken: vi.fn() });
    mocks.list.mockResolvedValue({ items: records, nextCursor: null });
    mocks.redirect.mockImplementation(() => { throw new Error("redirect"); });
  });
  it("filters before rendering and counting, preserving search/status and filter selection", async () => {
    render(await OrganizerApplicationsPage({ searchParams: Promise.resolve({ rsvp: "confirmed-late", q: "test", status: "accepted" }) }));
    expect(mocks.list).toHaveBeenCalledWith({ rsvp: "confirmed", q: "test", status: "accepted" });
    expect(screen.getByRole("heading", { name: "Late Arrival" })).toBeTruthy();
    for (const name of ["On Time", "Declined Person", "Pending Person", "Unknown Time"]) expect(screen.queryByRole("heading", { name })).toBeNull();
    expect((screen.getByLabelText("RSVP") as HTMLSelectElement).value).toBe("confirmed-late");
    expect(within(screen.getByLabelText("RSVP counts in the displayed results")).getByText("1")).toBeTruthy();
    expect(screen.getByText(/Latest RSVP:/).textContent).toMatch(/Sep 23, 2026.*12:00 AM/);
    expect(screen.getByText(/late reconfirmations are included/)).toBeTruthy();
  });
  it("preserves the ordinary queue without a late filter", async () => {
    render(await OrganizerApplicationsPage({ searchParams: Promise.resolve({}) }));
    for (const record of records) expect(screen.getByRole("heading", { name: record.applicant.displayName })).toBeTruthy();
    expect(mocks.list).toHaveBeenCalledWith({});
  });
  it.each(["confirmed", "declined", "pending"])("keeps existing %s filters working", async (rsvp) => {
    render(await OrganizerApplicationsPage({ searchParams: Promise.resolve({ rsvp }) }));
    expect(mocks.list).toHaveBeenCalledWith({ rsvp });
    expect((screen.getByLabelText("RSVP") as HTMLSelectElement).value).toBe(rsvp);
  });
  it("shows a useful empty state", async () => {
    mocks.list.mockResolvedValue({ items: [records[0]], nextCursor: null });
    render(await OrganizerApplicationsPage({ searchParams: Promise.resolve({ rsvp: "confirmed-late" }) }));
    expect(screen.getByRole("heading", { name: "No applications found" })).toBeTruthy();
    expect(screen.getByText("Try a different search, status, or RSVP filter.")).toBeTruthy();
  });
  it("does not expose data when the API denies admin access", async () => {
    mocks.list.mockRejectedValue(new ApiError(403, { code: "forbidden", message: "Forbidden" }));
    render(await OrganizerApplicationsPage({ searchParams: Promise.resolve({ rsvp: "confirmed-late" }) }));
    expect(screen.getByRole("alert").textContent).toBe("Admin access is required to view applications.");
    expect(screen.queryByRole("heading", { name: "Late Arrival" })).toBeNull();
  });
  it("requires sign-in before querying applications", async () => {
    mocks.auth.mockResolvedValue({ userId: null });
    await expect(OrganizerApplicationsPage({ searchParams: Promise.resolve({ rsvp: "confirmed-late" }) })).rejects.toThrow("redirect");
    expect(mocks.redirect).toHaveBeenCalledWith("/");
    expect(mocks.list).not.toHaveBeenCalled();
  });
});
