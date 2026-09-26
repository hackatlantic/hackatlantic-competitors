"use client";

import { motion } from "framer-motion";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  type CurrentUser,
  type CurrentUserRole,
} from "@/lib/api";

type WorkspaceLink = {
  code: string;
  href: string;
  label: string;
  role: Exclude<CurrentUserRole, "applicant">;
};

const workspaceLinks: WorkspaceLink[] = [
  { code: "ATS", href: "/organizer/applications", label: "Applications", role: "admin" },
  { code: "REV", href: "/reviewer/applications", label: "Reviews", role: "admin" },
  { code: "SCN", href: "/scanner", label: "Scanner", role: "scanner" },
];

export function RoleNavigation({ currentUser, hasApplication }: { currentUser: CurrentUser; hasApplication: boolean }) {
  const pathname = usePathname();

  const roles = new Set(currentUser.roles);
  const availableLinks = workspaceLinks.filter(
    ({ role }) => roles.has(role) || (role === "scanner" && roles.has("admin")),
  );

  if (availableLinks.length === 0) {
    return null;
  }

  const visibleLinks = [
    ...(hasApplication || roles.has("admin") || !roles.has("scanner")
      ? [{ code: "APL", href: "/", label: "My application" }]
      : []),
    ...availableLinks,
    { code: "PASS", href: "/event-pass", label: "My event pass" },
  ];
  const activeLink = visibleLinks.find(({ href }) =>
    href === "/" ? pathname === "/" : pathname.startsWith(href),
  ) ?? visibleLinks[0];

  return (
    <nav className="workspace-switcher" aria-label="Your HackAtlantic workspaces">
      <div className="workspace-switcher-heading">
        <span>Workspace</span>
        <strong>{activeLink.label}</strong>
      </div>
      <div className="workspace-switcher-links">
        {visibleLinks.map(({ code, href, label }) => {
          const active = href === "/" ? pathname === "/" : pathname.startsWith(href);
          return (
            <Link
              aria-current={active ? "page" : undefined}
              className="workspace-switcher-link"
              href={href}
              key={href}
            >
              {active ? (
                <motion.span
                  className="workspace-switcher-active"
                  layoutId="workspace-switcher-active"
                  transition={{ type: "spring", stiffness: 420, damping: 34 }}
                />
              ) : null}
              <small aria-hidden="true">{code}</small>
              <span>{label}</span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
