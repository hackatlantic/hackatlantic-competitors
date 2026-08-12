import { notFound } from "next/navigation";
import { LoadAuthBridge } from "@/components/load-auth-bridge";

function loadAuthEnabled(): boolean {
  const publishableKey = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY ?? "";
  const apiBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";
  return (
    publishableKey.startsWith("pk_test_") &&
    (apiBaseURL.includes("staging") || apiBaseURL.startsWith("http://localhost:"))
  );
}

export default function LoadAuthPage() {
  if (!loadAuthEnabled()) notFound();
  return (
    <main aria-label="Staging load-test authentication">
      <LoadAuthBridge />
    </main>
  );
}
