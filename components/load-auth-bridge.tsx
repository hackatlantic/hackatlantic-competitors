"use client";

import { useClerk } from "@clerk/nextjs";
import { useEffect } from "react";

export function LoadAuthBridge() {
  const clerk = useClerk();
  useEffect(() => {
    Object.assign(window, {
      Clerk: {
        loaded: true,
        client: clerk.client,
        setActive: clerk.setActive,
        get user() {
          return clerk.user;
        },
        get session() {
          return clerk.session;
        },
      },
    });
    return () => {
      Reflect.deleteProperty(window, "Clerk");
    };
  }, [clerk]);
  return null;
}
