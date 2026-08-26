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

export default function Home() {
  return (
    <main className="page portal-page">
      <header className="portal-header">
        <BrandMark />
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

      <Show when="signed-out">
        <section className="portal-hero">
          <div className="intro">
            <Image
              alt="Hack Atlantic logo"
              className="application-logo"
              height={144}
              src="/hackatlantic-logo.jpg"
              width={144}
            />
            <h1>Hack Atlantic</h1>
            <p className="intro-copy">
              2026 Application
            </p>
          </div>

          <aside className="route-card" aria-labelledby="route-heading">
            <h2 id="route-heading">Apply</h2>
            <ol className="route-steps">
              <li><span>01</span><div><strong>Create an account</strong></div></li>
              <li><span>02</span><div><strong>Complete the application</strong></div></li>
              <li><span>03</span><div><strong>Track your decision</strong></div></li>
            </ol>
            <div className="route-card-actions">
              <SignUpButton>
                <button className="button primary button-wide" type="button">Start application</button>
              </SignUpButton>
              <SignInButton>
                <button className="text-button" type="button">Sign in</button>
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
