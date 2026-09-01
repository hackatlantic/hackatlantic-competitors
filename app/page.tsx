import {
  Show,
  SignInButton,
  SignUpButton,
  UserButton,
} from "@clerk/nextjs";
import Image from "next/image";
import { ApplicantDashboard } from "@/components/applicant-dashboard";
import { RoleNavigation } from "@/components/role-navigation";
import { BrandMark } from "@/components/brand-mark";
import { LegalFooter } from "@/components/legal-footer";

export default function Home() {
  return (
    <main className="page portal-page">
      <header className="portal-header">
        <BrandMark />
        <nav className="nav" aria-label="Account">
          <Show when="signed-out">
            <SignInButton mode="modal">
              <button className="button secondary" type="button">
                Sign in
              </button>
            </SignInButton>
            <SignUpButton mode="modal">
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

      <Show when="signed-out">
        <section className="portal-hero">
          <div className="intro">
            <Image
              alt="HackAtlantic lobster logo"
              className="application-logo"
              height={180}
              priority
              src="/hackatlantic-logo.jpg"
              width={180}
            />
            <h1>Hack<br />Atlantic</h1>
            <p className="intro-copy">2026 application</p>
          </div>

          <aside className="route-card" aria-labelledby="route-heading">
            <h2 id="route-heading">Apply</h2>
            <ol className="route-steps">
              <li><span>01</span><div><strong>Create an account</strong><small>Sign up securely with email or Google.</small></div></li>
              <li><span>02</span><div><strong>Complete your application</strong><small>Your draft stays available until you submit.</small></div></li>
              <li><span>03</span><div><strong>Track your decision</strong><small>Return here for your result and event pass.</small></div></li>
            </ol>
            <div className="route-card-actions">
              <SignUpButton mode="modal">
                <button className="button primary button-wide" type="button">Start application <span aria-hidden="true">↗</span></button>
              </SignUpButton>
              <SignInButton mode="modal">
                <button className="text-button" type="button">Sign in to continue</button>
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

      <LegalFooter />
    </main>
  );
}
