import type { Metadata } from "next";
import { Show, SignInButton, SignUpButton, UserButton } from "@clerk/nextjs";
import { BrandMark } from "@/components/brand-mark";
import { LegalFooter } from "@/components/legal-footer";
import { VolunteerSignup } from "@/components/volunteer-access";
import "./volunteer.css";

export const metadata: Metadata = { title: "Volunteer · HackAtlantic", robots: { index: false, follow: false } };

export default function VolunteerPage() {
  return <main className="page portal-page">
    <header className="portal-header"><BrandMark /><Show when="signed-in"><UserButton /></Show></header>
    <section className="volunteer-panel" aria-labelledby="volunteer-title">
      <p className="volunteer-eyebrow">Hack Atlantic crew</p>
      <h1 id="volunteer-title">Ready to help?</h1>
      <p>Get scanner access for check-in and meals. No hackathon application needed.</p>
      <Show when="signed-out">
        <div className="volunteer-step">
          <h2>First, sign in</h2>
          <p>Then enter the name on your volunteer schedule. An admin will approve your account before you can scan.</p>
          <div className="volunteer-actions">
            <SignUpButton mode="modal" forceRedirectUrl="/volunteer"><button className="button primary">Create account</button></SignUpButton>
            <SignInButton mode="modal" forceRedirectUrl="/volunteer"><button className="button secondary">Sign in</button></SignInButton>
          </div>
        </div>
      </Show>
      <Show when="signed-in"><VolunteerSignup /></Show>
    </section>
    <LegalFooter />
  </main>;
}
