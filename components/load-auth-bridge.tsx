"use client";

import { useClerk } from "@clerk/nextjs";
import { useEffect } from "react";

export function LoadAuthBridge() {
  const clerk = useClerk();
  useEffect(() => {
    Object.assign(window, { Clerk: clerk });
    return () => {
      Reflect.deleteProperty(window, "Clerk");
    };
  }, [clerk]);
  return null;
}
