import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OrganizerPassActions } from "@/components/organizer-pass-actions";

const api = vi.hoisted(() => ({ issueAttendeePass: vi.fn(), reissueAttendeePass: vi.fn(), revokeAttendeePass: vi.fn() }));
const refresh = vi.hoisted(() => vi.fn());
const getToken = vi.hoisted(() => vi.fn());
vi.mock("@clerk/nextjs", () => ({ useAuth: () => ({ getToken }) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }) }));
vi.mock("@/lib/api", () => ({ createApiClient: () => api }));
vi.mock("@/components/pass-qr-code", () => ({ PassQrCode: () => <span>Local QR preview</span> }));
const pass = { id: "pass", attendeeId: "attendee", displayName: "Test attendee", status: "active" as const, issuedAt: "2026-09-03T12:00:00Z" };
const summary = { attendeeId: "attendee", pass: null };

describe("Organizer pass release after RSVP", () => {
  beforeEach(() => { vi.resetAllMocks(); api.issueAttendeePass.mockResolvedValue({ ...pass, qrToken: "local-test-only" }); });
  afterEach(cleanup);

  it("blocks pass release when RSVP is not confirmed", () => {
    render(<OrganizerPassActions initialAttendeePass={summary} rsvpConfirmed={false} />);
    const button = screen.getByRole("button", { name: "Issue pass" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(button);
    expect(api.issueAttendeePass).not.toHaveBeenCalled();
    expect(screen.getByText(/unavailable until the attendee confirms/)).not.toBeNull();
  });

  it("requires an explicit admin action even after RSVP confirmation", async () => {
    render(<OrganizerPassActions initialAttendeePass={summary} rsvpConfirmed />);
    expect(api.issueAttendeePass).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Issue pass" }));
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    expect(api.issueAttendeePass).toHaveBeenCalledExactlyOnceWith("attendee");
    expect(await screen.findByText(/pass-link message has been queued/)).not.toBeNull();
  });

  it("blocks reissue but preserves explicit revoke access for a declined RSVP", async () => {
    api.revokeAttendeePass.mockResolvedValue({ ...pass, status: "revoked" });
    render(<OrganizerPassActions initialAttendeePass={{ attendeeId: "attendee", pass }} rsvpConfirmed={false} />);
    expect((screen.getByRole("button", { name: "Reissue pass" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Revoke pass" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm revoke" }));
    await waitFor(() => expect(api.revokeAttendeePass).toHaveBeenCalledExactlyOnceWith("pass"));
    expect(api.reissueAttendeePass).not.toHaveBeenCalled();
  });

  it("disables a pending reissue confirmation if RSVP eligibility changes", () => {
    const initialAttendeePass = { attendeeId: "attendee", pass };
    const view = render(<OrganizerPassActions initialAttendeePass={initialAttendeePass} rsvpConfirmed />);
    fireEvent.click(screen.getByRole("button", { name: "Reissue pass" }));
    view.rerender(<OrganizerPassActions initialAttendeePass={initialAttendeePass} rsvpConfirmed={false} />);
    const button = screen.getByRole("button", { name: "Confirm reissue" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(button);
    expect(api.reissueAttendeePass).not.toHaveBeenCalled();
  });
});
