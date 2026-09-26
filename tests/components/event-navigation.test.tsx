import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EventNavigation } from "@/components/event-navigation";

const auth = vi.hoisted(() => ({ getToken: vi.fn(), userId: "volunteer" as string | null, isLoaded: true }));
const api = vi.hoisted(() => ({ getCurrentUser: vi.fn() }));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => auth }));
vi.mock("@/lib/api", () => ({ createApiClient: () => api }));

describe("Event navigation", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.resetAllMocks();
    auth.userId = "volunteer"; auth.isLoaded = true;
    api.getCurrentUser.mockResolvedValue({ roles: ["applicant", "scanner"] });
  });

  it.each(["scanner", "pass"] as const)("connects volunteer %s directly to scanner and pass, never the dashboard", async (current) => {
    render(<EventNavigation current={current} />);
    await screen.findByRole("link", { name: "Scanner" });
    await waitFor(() => expect(api.getCurrentUser).toHaveBeenCalledTimes(1));
    expect(screen.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual(["/scanner", "/event-pass"]);
    expect(screen.getByRole("link", { name: current === "scanner" ? "Scanner" : "My pass" }).getAttribute("aria-current")).toBe("page");
    expect(screen.queryByRole("link", { name: /dashboard/i })).toBeNull();
  });

  it("does not flash a dashboard link before roles resolve", async () => {
    let resolve!: (user: unknown) => void;
    api.getCurrentUser.mockReturnValue(new Promise((done) => { resolve = done; }));
    render(<EventNavigation current="pass" />);
    expect(screen.queryByRole("link", { name: /dashboard/i })).toBeNull();
    await act(async () => resolve({ roles: ["scanner"] }));
    expect(screen.getByRole("link", { name: "Scanner" }).getAttribute("href")).toBe("/scanner");
    expect(screen.queryByRole("link", { name: /dashboard/i })).toBeNull();
  });

  it("keeps organizers connected to their admin workspace", async () => {
    api.getCurrentUser.mockResolvedValue({ roles: ["admin", "scanner"] });
    render(<EventNavigation current="pass" />);
    expect((await screen.findByRole("link", { name: "Admin" })).getAttribute("href")).toBe("/organizer/operations");
    expect(screen.getByRole("link", { name: "Scanner" })).toBeTruthy();
  });

  it("keeps ordinary attendees connected to their dashboard without showing scanner access", async () => {
    api.getCurrentUser.mockResolvedValue({ roles: ["applicant"] });
    render(<EventNavigation current="pass" />);
    expect((await screen.findByRole("link", { name: "Dashboard" })).getAttribute("href")).toBe("/");
    expect(screen.queryByRole("link", { name: "Scanner" })).toBeNull();
  });

  it("offers recovery when role lookup fails, without guessing a dashboard destination", async () => {
    api.getCurrentUser.mockRejectedValueOnce(new Error("Offline"));
    render(<EventNavigation current="pass" />);
    fireEvent.click(await screen.findByRole("button", { name: "Reload navigation" }));
    await screen.findByRole("link", { name: "Scanner" });
    expect(screen.queryByRole("link", { name: /dashboard/i })).toBeNull();
    expect(api.getCurrentUser).toHaveBeenCalledTimes(2);
  });

  it("does not retain an admin link when switching accounts", async () => {
    api.getCurrentUser.mockResolvedValueOnce({ roles: ["admin"] });
    const view = render(<EventNavigation current="pass" />);
    await screen.findByRole("link", { name: "Admin" });
    auth.userId = "another-volunteer";
    api.getCurrentUser.mockReturnValue(new Promise(() => {}));
    view.rerender(<EventNavigation current="pass" />);
    expect(screen.queryByRole("link", { name: "Admin" })).toBeNull();
    expect(screen.queryByRole("link", { name: /dashboard/i })).toBeNull();
  });

  it("renders no authenticated navigation when signed out", () => {
    auth.userId = null;
    render(<EventNavigation current="pass" />);
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(api.getCurrentUser).not.toHaveBeenCalled();
  });
});
