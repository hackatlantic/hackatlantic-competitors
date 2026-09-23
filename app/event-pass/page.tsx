import type { Metadata } from "next";
import { auth } from "@clerk/nextjs/server";
import { redirect } from "next/navigation";
import Link from "next/link";
import { EventPass } from "@/components/event-pass";
import "./ticket.css";

export const metadata: Metadata = {
  title: "Event pass · HackAtlantic",
  robots: { index: false, follow: false },
};

export default async function EventPassPage() {
  const { userId } = await auth();
  if (!userId) redirect("/");
  return (
    <main className="ticket-page">
      <div className="ticket-page-inner">
        <Link className="ticket-back" href="/">← Back to dashboard</Link>
        <EventPass />
      </div>
    </main>
  );
}
