import type { Metadata } from "next";
import Link from "next/link";
import { BrandMark } from "@/components/brand-mark";
import { LegalFooter } from "@/components/legal-footer";

export const metadata: Metadata = {
  title: "Privacy Notice · HackAtlantic",
  description: "How HackAtlantic collects, uses, protects, and retains applicant information."
};

export default function PrivacyPage() {
  return (
    <main className="legal-page">
      <header className="legal-header">
        <BrandMark />
        <Link className="button secondary" href="/">Back to applications</Link>
      </header>

      <article className="legal-document">
        <div className="legal-title-block">
          <p className="coordinate-label">HA / LEGAL–01</p>
          <h1>Privacy notice</h1>
          <p className="legal-updated">Effective and last updated August 31, 2026</p>
        </div>

        <div className="legal-summary">
          <strong>The short version</strong>
          <p>
            We use your information to operate HackAtlantic applications and the event. We do not
            sell applicant information or use it for targeted advertising. Access is limited by role,
            and you can contact us about your information at any time.
          </p>
        </div>

        <section>
          <h2>1. Who this notice covers</h2>
          <p>
            This notice applies to people who use the HackAtlantic application portal, including
            applicants, reviewers, organizers, and event staff. HackAtlantic is responsible for the
            application and event-administration information described here.
          </p>
        </section>

        <section>
          <h2>2. Information we collect</h2>
          <ul>
            <li><strong>Account and identity information:</strong> name, email address, profile details, and account identifiers provided through Clerk or a sign-in provider such as Google.</li>
            <li><strong>Application information:</strong> school and education details, application answers, experience, interests, availability, and any other information you choose to submit.</li>
            <li><strong>Documents:</strong> your résumé and related upload metadata.</li>
            <li><strong>Review and event records:</strong> application status, internal review records, decisions, attendee eligibility, event-pass issuance, check-in, and permitted checkpoint or meal redemptions.</li>
            <li><strong>Technical and security information:</strong> request, device, browser, authentication, audit, error, and security-log information needed to operate and protect the service.</li>
          </ul>
          <p>Please do not include unnecessary sensitive personal information in free-text answers or your résumé.</p>
        </section>

        <section>
          <h2>3. How we use information</h2>
          <ul>
            <li>create and secure accounts, save drafts, and receive applications;</li>
            <li>review applications, communicate updates, and issue decisions;</li>
            <li>administer attendance, passes, check-in, meals, and other event entitlements;</li>
            <li>support users, investigate misuse, maintain audit trails, and protect the portal;</li>
            <li>measure reliability and improve the application and event experience; and</li>
            <li>meet legal, safety, and administrative obligations.</li>
          </ul>
        </section>

        <section>
          <h2>4. When information is shared</h2>
          <p>
            Authorized HackAtlantic organizers and reviewers can access only the information needed
            for their roles. Scanner staff receive limited pass and redemption information rather than
            full applications. We also use service providers to run the portal, including Clerk for
            authentication, Google when you choose Google sign-in, Vercel, DigitalOcean, Supabase and
            PostgreSQL infrastructure, and communications providers. These providers process data for
            us under their own security and privacy commitments.
          </p>
          <p>
            We may disclose information when reasonably necessary to comply with law, protect people
            or the event, investigate fraud or abuse, or complete an organizational transition. We do
            not sell personal information.
          </p>
        </section>

        <section>
          <h2>5. Storage, international processing, and security</h2>
          <p>
            Our providers may store or process information in Canada, the United States, or other
            jurisdictions where they operate. Privacy laws may differ across those locations. We use
            role-based access, private storage, authentication, encrypted connections, logging, backups,
            and other reasonable safeguards. No online service can guarantee absolute security.
          </p>
        </section>

        <section>
          <h2>6. Retention</h2>
          <p>
            We keep information only as long as reasonably needed to administer the application cycle
            and event, preserve necessary operational and security records, resolve disputes, and meet
            legal obligations. We then delete or anonymize it where practicable. Encrypted backups may
            retain information for a limited additional period before expiring under their schedules.
          </p>
        </section>

        <section>
          <h2>7. Your choices and requests</h2>
          <p>
            You may ask to access, correct, or delete your information, or withdraw consent where
            applicable. Some information may need to be retained for security, legal, or event-integrity
            reasons. Withdrawing or deleting required application information can prevent us from
            processing your application or administering your attendance.
          </p>
          <p>
            Email <a href="mailto:team@hackatlantic.ca?subject=Privacy%20request">team@hackatlantic.ca</a>
            {" "}with the subject “Privacy request.” We may need to verify your identity before responding.
          </p>
        </section>

        <section>
          <h2>8. Cookies and authentication</h2>
          <p>
            The portal and its authentication providers use cookies or similar local technologies that
            are necessary to sign you in, keep sessions secure, prevent abuse, and remember essential
            preferences. We do not use applicant data for targeted advertising.
          </p>
        </section>

        <section>
          <h2>9. Young applicants</h2>
          <p>
            The portal is intended for people eligible under HackAtlantic&apos;s published participation
            criteria. If you are below the age at which you may provide consent independently where you
            live, please involve a parent or guardian before submitting personal information.
          </p>
        </section>

        <section>
          <h2>10. Changes and contact</h2>
          <p>
            We may update this notice as the event or portal changes. The date above identifies the
            current version. Material changes will be communicated through the portal or another
            appropriate channel. Questions can be sent to
            {" "}<a href="mailto:team@hackatlantic.ca">team@hackatlantic.ca</a>.
          </p>
        </section>
      </article>

      <LegalFooter />
    </main>
  );
}
