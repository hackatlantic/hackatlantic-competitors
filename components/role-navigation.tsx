"use client";

import { useAuth } from "@clerk/nextjs";
import { motion } from "framer-motion";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import {
  createApiClient,
  type CurrentUser,
  type CurrentUserRole,
} from "@/lib/api";

type WorkspaceLink = {
  href: string;
  label: string;
  role: Exclude<CurrentUserRole, "applicant">;
};

const workspaceLinks: WorkspaceLink[] = [
  { href: "/organizer/applications", label: "Applications", role: "admin" },
  { href: "/reviewer/applications", label: "Reviews", role: "admin" },
  { href: "/scanner", label: "Scanner", role: "scanner" },
];

export function RoleNavigation() {
  const pathname = usePathname();
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);

  useEffect(() => {
    let cancelled = false;

    const loadCurrentUser = async () => {
      try {
        const user = await client.getCurrentUser();
        if (!cancelled) {
          setCurrentUser(user);
        }
      } catch {
        // ApplicantDashboard surfaces API errors; keep navigation unobtrusive.
      }
    };

    void loadCurrentUser();
    return () => {
      cancelled = true;
    };
  }, [client]);

  if (!currentUser) {
    return null;
  }

  const roles = new Set(currentUser.roles);
  const availableLinks = workspaceLinks.filter(
    ({ role }) => roles.has(role) || (role === "scanner" && roles.has("admin")),
  );

  if (availableLinks.length === 0) {
    return null;
  }

  const visibleLinks = [
    { href: "/", label: "My application" },
    ...availableLinks,
  ];
  const activeLink = visibleLinks.find(({ href }) =>
    href === "/" ? pathname === "/" : pathname.startsWith(href),
  ) ?? visibleLinks[0];

  return (
    <nav className="workspace-switcher" aria-label="Your HackAtlantic workspaces">
      <div className="workspace-switcher-heading">
        <span>Workspaces</span>
        <strong>{activeLink.label}</strong>
      </div>
      <div className="workspace-switcher-links">
        {visibleLinks.map(({ href, label }) => {
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
              <span>{label}</span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
