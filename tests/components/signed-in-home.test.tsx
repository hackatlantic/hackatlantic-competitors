import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SignedInHome } from "@/components/signed-in-home";
import { RoleNavigation } from "@/components/role-navigation";

const mocks = vi.hoisted(() => ({
  auth: { isLoaded: true, userId: "account-1", getToken: vi.fn() },
  router: { replace: vi.fn() },
  api: { getCurrentUser: vi.fn(), getMyApplications: vi.fn() },
  applicant: vi.fn(),
}));
vi.mock("@clerk/nextjs", () => ({ useAuth: () => mocks.auth }));
vi.mock("next/navigation", () => ({ useRouter: () => mocks.router, usePathname: () => "/" }));
vi.mock("@/lib/api", () => ({ createApiClient: () => mocks.api }));
vi.mock("@/components/applicant-dashboard", () => ({
  ApplicantDashboard: () => { mocks.applicant(); return <div>Applicant dashboard</div>; },
}));

const scanner = { id: "scanner-1", email: "volunteer@example.com", displayName: "Test volunteer", roles: ["applicant", "scanner"] as ("applicant" | "scanner" | "admin")[] };

describe("signed-in workspace routing", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.auth.isLoaded = true;
    mocks.auth.userId = "account-1";
    mocks.api.getCurrentUser.mockResolvedValue(scanner);
    mocks.api.getMyApplications.mockResolvedValue({ items: [] });
  });

  it("routes a volunteer with no application to Scanner without mounting the application flow", async () => {
    render(<SignedInHome />);
    await waitFor(() => expect(mocks.router.replace).toHaveBeenCalledWith("/scanner"));
    expect(mocks.applicant).not.toHaveBeenCalled();
    expect(screen.queryByText("My application")).toBeNull();
    expect(screen.getByRole("link", { name: "My event pass" }).getAttribute("href")).toBe("/event-pass");
  });

  it("preserves a scanner's actual application and both scanner and pass links", async () => {
    mocks.api.getMyApplications.mockResolvedValue({ items: [{ id: "existing" }] });
    render(<SignedInHome />);
    await screen.findByText("Applicant dashboard");
    expect(screen.getByRole("link", { name: /my application/i })).toBeTruthy();
    expect(screen.getByRole("link", { name: /scanner/i })).toBeTruthy();
    expect(screen.getByRole("link", { name: /my event pass/i })).toBeTruthy();
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("preserves ordinary applicant onboarding", async () => {
    mocks.api.getCurrentUser.mockResolvedValue({ ...scanner, roles: ["applicant"] });
    render(<SignedInHome />);
    await screen.findByText("Applicant dashboard");
    expect(screen.queryByRole("link", { name: /scanner/i })).toBeNull();
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("keeps admin workspaces accessible", async () => {
    mocks.api.getCurrentUser.mockResolvedValue({ ...scanner, roles: ["admin", "scanner"] });
    render(<SignedInHome />);
    await screen.findByText("Applicant dashboard");
    expect(screen.getByRole("link", { name: "Applications" })).toBeTruthy();
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("does not guess the workspace when application lookup fails and allows retry", async () => {
    mocks.api.getMyApplications.mockRejectedValueOnce(new Error("Unavailable"));
    render(<SignedInHome />);
    await screen.findByRole("alert");
    expect(mocks.applicant).not.toHaveBeenCalled();
    expect(mocks.router.replace).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(mocks.router.replace).toHaveBeenCalledWith("/scanner"));
  });

  it("waits for authentication before loading or showing the applicant dashboard", () => {
    mocks.auth.isLoaded = false;
    render(<SignedInHome />);
    expect(screen.getByRole("status").textContent).toContain("Loading");
    expect(mocks.api.getCurrentUser).not.toHaveBeenCalled();
    expect(mocks.applicant).not.toHaveBeenCalled();
  });

  it("never flashes or mounts the application flow while workspace lookup is pending", async () => {
    mocks.api.getMyApplications.mockReturnValue(new Promise(() => {}));
    render(<SignedInHome />);
    await waitFor(() => expect(mocks.api.getCurrentUser).toHaveBeenCalledOnce());
    expect(screen.getByRole("status").textContent).toContain("Loading");
    expect(mocks.applicant).not.toHaveBeenCalled();
    expect(mocks.router.replace).not.toHaveBeenCalled();
  });

  it("does not include an application tab in scanner-only navigation", () => {
    render(<RoleNavigation currentUser={scanner} hasApplication={false} />);
    expect(screen.queryByRole("link", { name: /my application/i })).toBeNull();
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });
});
