import type { AttendanceRSVP } from "@/lib/api";

// End of Tuesday, September 22, 2026 in Fredericton (Atlantic Daylight Time).
// Use an explicit offset so the server's own timezone cannot move the boundary.
export const LATE_RSVP_START = "2026-09-23T00:00:00-03:00";
export const RSVP_TIME_ZONE = "America/Moncton";

export function isLateRSVP(rsvp?: AttendanceRSVP): boolean {
  if (rsvp?.status !== "confirmed" || !rsvp.respondedAt) return false;
  const timestamp = Date.parse(rsvp.respondedAt);
  return Number.isFinite(timestamp) && timestamp >= Date.parse(LATE_RSVP_START);
}

export function formatRSVPTime(value?: string): string | null {
  if (!value || !Number.isFinite(Date.parse(value))) return null;
  return new Intl.DateTimeFormat("en-US", {
    timeZone: RSVP_TIME_ZONE,
    month: "short", day: "numeric", year: "numeric",
    hour: "numeric", minute: "2-digit", timeZoneName: "short",
  }).format(new Date(value));
}
