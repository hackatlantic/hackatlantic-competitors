import { cloneElement, type ReactElement, type ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Home from "@/app/page";

const clerkActions = vi.hoisted(() => ({
  openSignIn: vi.fn(),
  openSignUp: vi.fn(),
}));

vi.mock("@clerk/nextjs", () => ({
  Show: ({ children, when }: { children: ReactNode; when: string }) =>
    when === "signed-out" ? children : null,
  SignInButton: ({ children }: { children: ReactElement<{ onClick?: () => void }> }) =>
    cloneElement(children, { onClick: clerkActions.openSignIn }),
  SignUpButton: ({ children }: { children: ReactElement<{ onClick?: () => void }> }) =>
    cloneElement(children, { onClick: clerkActions.openSignUp }),
  UserButton: () => null,
}));

vi.mock("next/image", () => ({
  default: ({ alt }: { alt: string }) => <span aria-label={alt} role="img" />,
}));

vi.mock("@/components/applicant-dashboard", () => ({
  ApplicantDashboard: () => null,
}));

vi.mock("@/components/role-navigation", () => ({
  RoleNavigation: () => null,
}));

describe("public application entry", () => {
  afterEach(cleanup);

  beforeEach(() => {
    clerkActions.openSignIn.mockClear();
    clerkActions.openSignUp.mockClear();
  });

  it("opens Clerk sign-up and sign-in from the primary application actions", () => {
    render(<Home />);

    fireEvent.click(screen.getByRole("button", { name: /start application/i }));
    fireEvent.click(screen.getByRole("button", { name: /sign in to continue/i }));

    expect(clerkActions.openSignUp).toHaveBeenCalledOnce();
    expect(clerkActions.openSignIn).toHaveBeenCalledOnce();
  });

  it("renders the real brand mark and public legal navigation", () => {
    render(<Home />);

    expect(screen.getByRole("img", { name: "HackAtlantic lobster logo" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Privacy" }).getAttribute("href")).toBe("/privacy");
    expect(screen.getByRole("link", { name: "Terms" }).getAttribute("href")).toBe("/terms");
  });
});
