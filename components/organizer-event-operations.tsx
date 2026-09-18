"use client";

import { useAuth } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useMemo, useRef, useState, type FormEvent } from "react";
import { CheckInOverview } from "@/components/event-day-overview";
import {
  createApiClient,
  type OrganizerApplication,
  type OrganizerCheckpoint,
  type OrganizerRedemptionCount,
} from "@/lib/api";

type Props = {
  initialCheckpoints: OrganizerCheckpoint[];
  initialCounts: OrganizerRedemptionCount[];
};

export function OrganizerEventOperations({ initialCheckpoints, initialCounts }: Props) {
  const { getToken } = useAuth();
  const router = useRouter();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<OrganizerApplication[]>([]);
  const [searching, setSearching] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const requestBusy = useRef(false);

  async function searchAttendees(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!query.trim() || requestBusy.current) return;
    requestBusy.current = true;
    setSearching(true);
    setMatches([]);
    setMessage("");
    setError("");
    try {
      const result = await client.listOrganizerApplications({ status: "accepted", q: query.trim() });
      setMatches(result.items);
      setMessage(result.items.length
        ? "Open an attendee’s record for their pass and access details."
        : "No accepted attendees found. Try their email address.");
    } catch {
      setError("We couldn’t search attendees. Please try again.");
    } finally {
      requestBusy.current = false;
      setSearching(false);
    }
  }

  return (
    <div className="organizer-operations check-in-dashboard">
      <CheckInOverview checkpoints={initialCheckpoints} counts={initialCounts} onRefresh={() => router.refresh()}>
        <section className="check-in-search" aria-labelledby="find-attendee-heading">
          <h2 id="find-attendee-heading">Find an attendee</h2>
          <form onSubmit={searchAttendees}>
            <label htmlFor="check-in-search">Name or email</label>
            <div className="check-in-search-controls">
              <input
                id="check-in-search"
                type="search"
                autoComplete="off"
                value={query}
                disabled={searching}
                placeholder="Search accepted attendees"
                onChange={(event) => { setQuery(event.target.value); setMatches([]); setMessage(""); setError(""); }}
              />
              <button type="submit" className="button secondary" disabled={searching || !query.trim()}>
                {searching ? "Searching…" : "Search"}
              </button>
            </div>
          </form>
          {message ? <p className="staff-muted" role="status">{message}</p> : null}
          {error ? <p className="error-message" role="alert">{error}</p> : null}
          {matches.length ? <ul className="check-in-search-results">{matches.map((application) => (
            <li key={application.id}>
              <Link href={`/organizer/applications/${application.id}`}>
                <span><strong>{application.applicant.displayName || "Attendee"}</strong>{" "}<span>{application.applicant.email}</span></span>
                <span aria-hidden="true">↗</span>
              </Link>
            </li>
          ))}</ul> : null}
        </section>
      </CheckInOverview>
    </div>
  );
}
