import {
  Show,
  SignInButton,
  SignUpButton,
  UserButton,
} from "@clerk/nextjs";
import { ApplicantDashboard } from "@/components/applicant-dashboard";
import { RoleNavigation } from "@/components/role-navigation";
import { BrandMark } from "@/components/brand-mark";

export default function Home() {
  return (
    <main className="page portal-page">
      <header className="portal-header">
        <BrandMark />
        <div className="portal-header-meta" aria-hidden="true">
          <span>Atlantic Canada</span>
          <span>Applicant portal · 2026</span>
        </div>
      <nav className="nav" aria-label="Account">
        <Show when="signed-out">
          <SignInButton>
            <button className="button secondary" type="button">
              Sign in
            </button>
          </SignInButton>
          <SignUpButton>
            <button className="button primary" type="button">
              Sign up
            </button>
          </SignUpButton>
        </Show>
        <Show when="signed-in">
          <UserButton />
        </Show>
      </nav>
      </header>

          <div className="portal-ticker" aria-label="HackAtlantic applications are open">
      <div className="portal-ticker-track">
        <span>Applications 2026</span>
        <span aria-hidden="true">✦</span>
        <span>Atlantic Canada</span>
        <span aria-hidden="true">✦</span>
        <span>Build something real</span>
        <span aria-hidden="true">✦</span>
      </div>
      <div className="portal-ticker-track" aria-hidden="true">
        <span>Applications 2026</span>
        <span aria-hidden="true">✦</span>
        <span>Atlantic Canada</span>
        <span aria-hidden="true">✦</span>
        <span>Build something real</span>
        <span aria-hidden="true">✦</span>
      </div>
    </div>
      <Show when="signed-out">
        <section className="portal-hero">
          <div className="intro">
            <p className="eyebrow">Atlantic Canada · Student-built</p>
            <span className="hero-stamp">Open call<br />2026</span>
            <h1>Build beyond<br />the edge.</h1>
            <p className="intro-copy">
              Your route into Atlantic Canada&apos;s largest student-run hackathon
              starts here. Bring an idea, find your people, and make something real.
            </p>
            <div className="portal-facts" aria-label="Event facts">
              <div><strong>100+</strong><span>hackers</span></div>
              <div><strong>01</strong><span>weekend</span></div>
              <div><strong>∞</strong><span>ideas</span></div>
            </div>
          </div>

          <aside className="route-card" aria-labelledby="route-heading">
            <div className="route-card-heading">
              <p className="coordinate-label">HA / AP–01</p>
              <span className="live-indicator">Applications portal</span>
            </div>
            <h2 id="route-heading">Your route in</h2>
            <ol className="route-steps">
              <li><span>01</span><div><strong>Create your profile</strong><small>One account keeps your draft safe.</small></div></li>
              <li><span>02</span><div><strong>Tell us what drives you</strong><small>School, experience, ideas, and your resume.</small></div></li>
              <li><span>03</span><div><strong>Track your decision</strong><small>Return here for your result and event pass.</small></div></li>
            </ol>
            <div className="route-card-actions">
              <SignUpButton>
                <button className="button primary button-wide" type="button">Start an application <span aria-hidden="true">↗</span></button>
              </SignUpButton>
              <SignInButton>
                <button className="text-button" type="button">Already started? Sign in</button>
              </SignInButton>
            </div>
          </aside>
        </section>
      </Show>

      <Show when="signed-in">
        <div className="signed-in-home portal-workspace">
          <RoleNavigation />
          <ApplicantDashboard />
        </div>
      </Show>
    </main>
  );
}
