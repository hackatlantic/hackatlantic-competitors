import {
  SignInButton,
  SignUpButton,
  Show,
  UserButton
} from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import Image from "next/image";
import QRCode from "qrcode";
import { submitApplication } from "@/app/actions";
import { ensureQrCodeId, getOrCreateApplicant } from "@/lib/applicants";

export default async function Home() {
  const { userId } = await auth();
  const applicant = userId ? await getOrCreateApplicant(userId) : null;
  const qrCodeId = applicant ? await ensureQrCodeId(applicant) : null;
  const qrCodeDataUrl = qrCodeId
    ? await QRCode.toDataURL(qrCodeId, {
        errorCorrectionLevel: "M",
        margin: 1,
        width: 280
      })
    : null;

  return (
    <main className="page">
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

      <Show when="signed-out">
        <section className="intro">
          <p className="eyebrow">Applicant portal</p>
          <h1>HackAtlantic Competitors</h1>
          <p>Sign in or create an account to apply for the hackathon.</p>
        </section>
      </Show>

      <Show when="signed-in">
        {applicant?.accepted && qrCodeId && qrCodeDataUrl ? (
          <AcceptedView qrCodeDataUrl={qrCodeDataUrl} qrCodeId={qrCodeId} />
        ) : applicant?.applied_at ? (
          <ApplicationStatus accepted={applicant.accepted} />
        ) : (
          <ApplicationForm />
        )}
      </Show>
    </main>
  );
}

function ApplicationForm() {
  return (
    <section className="panel application">
      <p className="eyebrow">Application</p>
      <h1>Apply to HackAtlantic</h1>
      <form action={submitApplication} className="form">
        <label>
          Full name
          <input name="fullName" required type="text" />
        </label>

        <label>
          Email
          <input name="email" required type="email" />
        </label>

        <label>
          School or organization
          <input name="school" required type="text" />
        </label>

        <label>
          What is your hackathon or building experience?
          <textarea name="experience" required rows={5} />
        </label>

        <label>
          What do you want to build or learn at HackAtlantic?
          <textarea name="goals" required rows={5} />
        </label>

        <label>
          Resume
          <input
            accept=".pdf,.doc,.docx"
            name="resume"
            required
            type="file"
          />
        </label>

        <button className="button primary submit" type="submit">
          Submit application
        </button>
      </form>
    </section>
  );
}

function ApplicationStatus({ accepted }: { accepted: boolean }) {
  return (
    <section className="panel status">
      <p className="eyebrow">Application status</p>
      <h1>{accepted ? "Accepted" : "Application submitted"}</h1>
      <p>
        {accepted
          ? "You have been accepted into the hackathon."
          : "Your application is under review. Check back here for updates."}
      </p>
    </section>
  );
}

function AcceptedView({
  qrCodeDataUrl,
  qrCodeId
}: {
  qrCodeDataUrl: string;
  qrCodeId: string;
}) {
  return (
    <section className="panel accepted">
      <div>
        <p className="eyebrow">Accepted</p>
        <h1>You are in.</h1>
        <p>Your HackAtlantic QR code id is ready for event check-ins.</p>
      </div>
      <div className="qr">
        <Image
          alt="HackAtlantic QR code"
          height={220}
          src={qrCodeDataUrl}
          unoptimized
          width={220}
        />
        <code>{qrCodeId}</code>
      </div>
    </section>
  );
}
