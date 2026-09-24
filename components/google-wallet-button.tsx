"use client";

import { useAuth } from "@clerk/nextjs";
import Image from "next/image";
import { useEffect, useMemo, useRef, useState } from "react";
import { ApiError, createApiClient } from "@/lib/api";
import { navigateToGoogleWallet } from "@/lib/google-wallet";

export function GoogleWalletButton() {
  const { getToken } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const mounted = useRef(false);
  const inFlight = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  async function save() {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      const result = await client.createGoogleWalletPass();
      // Account changes unmount this component; never open the old user's pass.
      if (mounted.current) navigateToGoogleWallet(result.saveUrl);
    } catch (failure) {
      if (mounted.current) setError(failure instanceof ApiError && failure.status === 404
        ? "This pass is no longer available. Refresh the page to check your pass."
        : "Google Wallet couldn’t open. Try again, or use the QR code above.");
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(false);
    }
  }

  return (
    <div className="ticket-wallet">
      <button type="button" className="google-wallet-button" onClick={() => void save()} disabled={busy} aria-busy={busy} aria-label="Add to Google Wallet" aria-describedby="wallet-description">
        <Image src="/google-wallet/add-to-google-wallet.svg" alt="" width={199} height={55} unoptimized />
      </button>
      <p id="wallet-description">Save your pass to your Google account for easy access.</p>
      <p role="status" aria-live="polite">{busy ? "Opening Google Wallet…" : error}</p>
    </div>
  );
}
