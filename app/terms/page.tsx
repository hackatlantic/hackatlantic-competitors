import type { Metadata } from "next";
import Link from "next/link";
import { BrandMark } from "@/components/brand-mark";
import { LegalFooter } from "@/components/legal-footer";

export const metadata: Metadata = {
  title: "Terms of Use · HackAtlantic",
  description: "Terms for using the HackAtlantic application and event portal."
};

export default function TermsPage() {
  return (
    <main className="legal-page">
      <header className="legal-header">
        <BrandMark />
        <Link className="button secondary" href="/">Back to applications</Link>
      </header>

      <article className="legal-document">
        <div className="legal-title-block legal-title-terms">
          <p className="coordinate-label">HA / LEGAL–02</p>
          <h1>Terms of use</h1>
          <p className="legal-updated">Effective and last updated August 31, 2026</p>
        </div>

        <div className="legal-summary">
          <strong>Read this before applying</strong>
          <p>
            These terms govern the HackAtlantic application and event portal. By creating an account
            or submitting an application, you agree to use the portal honestly, safely, and only for
            its intended purpose.
          </p>
        </div>

        <section>
          <h2>1. Eligibility and accounts</h2>
          <p>
            You must meet HackAtlantic&apos;s published eligibility requirements and provide accurate,
            current information. Use one account for yourself, keep your credentials secure, and notify
            us promptly if you believe your account has been compromised. If you cannot legally agree
            to these terms on your own, a parent or guardian must do so with you where required.
          </p>
        </section>

        <section>
          <h2>2. Applications and decisions</h2>
          <p>
            You are responsible for your application and for submitting it before the stated deadline.
            A submission does not guarantee acceptance, travel support, accommodation, prizes, or any
            other benefit. HackAtlantic may verify eligibility and application information. Selection
            decisions are made by the organizers using the announced process and available capacity.
          </p>
        </section>

        <section>
          <h2>3. Your content</h2>
          <p>
            You retain ownership of the résumé, written answers, and other material you submit. You give
            HackAtlantic permission to store, reproduce, and review that material only as reasonably
            needed to evaluate applications, communicate with you, administer the event, secure the
            service, and meet legal obligations. You must have the right to submit the material and must
            not infringe another person&apos;s rights.
          </p>
        </section>

        <section>
          <h2>4. Acceptable use</h2>
          <p>You must not:</p>
          <ul>
            <li>impersonate another person, submit deceptive information, or transfer your account;</li>
            <li>probe, bypass, overload, reverse engineer, or interfere with portal security or availability;</li>
            <li>upload malware, abusive material, unlawful content, or unnecessary sensitive information;</li>
            <li>scrape applicant information or access records beyond your authorized role; or</li>
            <li>use passes, QR codes, reviewer access, or scanner access for an unauthorized purpose.</li>
          </ul>
        </section>

        <section>
          <h2>5. Event passes and check-in</h2>
          <p>
            Event passes and QR credentials are personal, may be subject to eligibility checks, and must
            not be sold, copied, or transferred without organizer approval. Check-in, meal, and checkpoint
            scans may enforce defined limits. A screenshot or duplicate credential does not create an
            additional entitlement.
          </p>
        </section>

        <section>
          <h2>6. Event rules and changes</h2>
          <p>
            Participants must follow the event&apos;s published code of conduct, venue rules, safety
            instructions, and organizer directions. HackAtlantic may reasonably change application dates,
            programming, venue details, capacity, services, or the event itself. We will communicate
            significant changes through available contact channels.
          </p>
        </section>

        <section>
          <h2>7. Suspension and enforcement</h2>
          <p>
            HackAtlantic may limit or suspend access, invalidate a pass, remove an application from
            consideration, or take other proportionate action when reasonably necessary to protect people,
            the event, or the service; investigate abuse; enforce these terms; or comply with law. Where
            appropriate, we will provide notice and an opportunity to clarify the situation.
          </p>
        </section>

        <section>
          <h2>8. Service availability</h2>
          <p>
            We work to keep the portal accurate and available, but maintenance, provider failures, security
            incidents, and events outside our control can interrupt it. If a verified technical issue prevents
            a timely submission, contact us promptly with the relevant details. To the extent permitted by
            law, the portal is provided without guarantees of uninterrupted or error-free operation.
          </p>
        </section>

        <section>
          <h2>9. Privacy</h2>
          <p>
            Our <Link href="/privacy">Privacy Notice</Link> explains how personal information is collected,
            used, shared, protected, and retained. It forms part of these terms.
          </p>
        </section>

        <section>
          <h2>10. Governing terms and contact</h2>
          <p>
            These terms are governed by the laws applicable in New Brunswick and the federal laws of
            Canada, subject to any mandatory rights that apply where you live. If one provision cannot be
            enforced, the remaining provisions continue to apply. Questions or portal problems can be sent
            to <a href="mailto:team@hackatlantic.ca">team@hackatlantic.ca</a>.
          </p>
        </section>
      </article>

      <LegalFooter />
    </main>
  );
}
