"use client";

import { useAuth } from "@clerk/nextjs";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { ApplicantDashboard } from "@/components/applicant-dashboard";
import { RoleNavigation } from "@/components/role-navigation";
import { createApiClient, type CurrentUser } from "@/lib/api";

type HomeState = {
  userId: string;
  user: CurrentUser;
  hasApplication: boolean;
  scannerOnly: boolean;
};

// Resolve the workspace before mounting ApplicantDashboard: mounting it for a
// volunteer can otherwise fetch a closed form or even create an unwanted draft.
export function SignedInHome() {
  const { getToken, isLoaded, userId } = useAuth();
  const router = useRouter();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [home, setHome] = useState<HomeState | null>(null);
  const [failedUserId, setFailedUserId] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!isLoaded || !userId) return;
    const accountId = userId;
    let cancelled = false;
    async function load() {
      try {
        const [user, applications] = await Promise.all([
          client.getCurrentUser(),
          client.getMyApplications(),
        ]);
        if (cancelled) return;
        const hasApplication = applications.items.length > 0;
        const scannerOnly = user.roles.includes("scanner") && !user.roles.includes("admin") && !hasApplication;
        setFailedUserId(null);
        setHome({ userId: accountId, user, hasApplication, scannerOnly });
        if (scannerOnly) router.replace("/scanner");
      } catch {
        if (!cancelled) setFailedUserId(accountId);
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [client, isLoaded, userId, router, attempt]);

  if (userId && failedUserId === userId) {
    return <section className="workspace-state" aria-labelledby="workspace-error">
      <h2 id="workspace-error">We couldn’t load your workspace</h2>
      <p role="alert">Please try again. Your application and scanner access have not changed.</p>
      <button className="button secondary" onClick={() => { setFailedUserId(null); setHome(null); setAttempt((value) => value + 1); }}>Try again</button>
    </section>;
  }

  if (!isLoaded || !userId || home?.userId !== userId) {
    return <p role="status">Loading your workspace…</p>;
  }
  if (home.scannerOnly) {
    return <section className="workspace-state" aria-label="Volunteer workspace">
      <p role="status">Opening scanner…</p>
      <div className="actions">
        <Link className="button primary" href="/scanner">Open scanner</Link>
        <Link className="button secondary" href="/event-pass">My event pass</Link>
      </div>
    </section>;
  }
  return <>
    <RoleNavigation currentUser={home.user} hasApplication={home.hasApplication} />
    <ApplicantDashboard />
  </>;
}
