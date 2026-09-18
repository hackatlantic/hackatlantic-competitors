import Link from "next/link";

export function ApplicantPass() {
  return (
    <section className="application-pass">
      <div className="event-card-heading">
        <span className="event-card-kicker">Check-in</span>
        <h2>Event pass</h2>
      </div>
      <p>Your ticket and entry QR will be available here once your pass is issued.</p>
      <Link className="button primary" href="/event-pass" prefetch={false}>
        Event pass <span aria-hidden="true">↗</span>
      </Link>
    </section>
  );
}
