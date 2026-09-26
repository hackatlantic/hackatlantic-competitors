"use client";

import { useAuth } from "@clerk/nextjs";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { createApiClient, type CurrentUser } from "@/lib/api";
import styles from "./event-navigation.module.css";

export function EventNavigation({ current }: { current: "scanner" | "pass" }) {
  const { getToken, userId, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [identity, setIdentity] = useState<{ accountId: string; user: CurrentUser } | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [failedAccount, setFailedAccount] = useState<string | null>(null);

  useEffect(() => {
    if (!isLoaded || !userId) return;
    let cancelled = false;
    const accountId = userId;
    async function load() {
      try {
        const user = await client.getCurrentUser();
        if (!cancelled) {
          setIdentity({ accountId, user });
          setFailedAccount(null);
        }
      } catch {
        if (!cancelled) setFailedAccount(accountId);
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [client, userId, isLoaded, attempt]);

  if (!isLoaded || !userId) return null;
  // Never show an applicant-dashboard link while the role is unresolved.
  const user = identity?.accountId === userId ? identity.user : null;
  const admin = user?.roles.includes("admin");
  const scanner = admin || user?.roles.includes("scanner");
  return (
    <nav className={styles.navigation} aria-label="Event navigation">
      {user && (admin || !scanner) ? (
        <Link className={styles.link} href={admin ? "/organizer/operations" : "/"}>
          {admin ? "Admin" : "Dashboard"}
        </Link>
      ) : null}
      {scanner || current === "scanner" ? (
        <Link className={styles.link} href="/scanner" aria-current={current === "scanner" ? "page" : undefined}>Scanner</Link>
      ) : null}
      <Link className={styles.link} href="/event-pass" aria-current={current === "pass" ? "page" : undefined}>My pass</Link>
      {failedAccount === userId ? (
        <button className={styles.retry} type="button" onClick={() => { setFailedAccount(null); setAttempt((value) => value + 1); }}>Reload navigation</button>
      ) : null}
    </nav>
  );
}
