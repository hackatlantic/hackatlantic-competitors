"use client";

import { SignInButton } from "@clerk/nextjs";
import Image from "next/image";
import { useState } from "react";
import { ApplicantDashboard } from "@/components/applicant-dashboard";

type ApplicationEntryProps = {
  previewMode?: boolean;
  requiresAuth?: boolean;
};

export function ApplicationEntry({
  previewMode = false,
  requiresAuth = false,
}: ApplicationEntryProps) {
  const [started, setStarted] = useState(false);

  if (started) {
    return <ApplicantDashboard previewMode={previewMode} />;
  }

  return (
    <section
      className="application-start-screen"
      aria-labelledby="application-start-heading"
    >
      <Image
        alt="Hack Atlantic logo"
        className="application-start-logo"
        height={180}
        priority
        src="/hackatlantic-starter-logo.jpg"
        width={225}
      />
      <div className="application-start-copy">
        <p>Applications are open</p>
        <h1 id="application-start-heading">
          Ready to start your
          <span>Hack Atlantic application?</span>
        </h1>
      </div>
      {requiresAuth ? (
        <SignInButton mode="modal">
          <button className="button primary" type="button">
            Start application
          </button>
        </SignInButton>
      ) : (
        <button
          className="button primary"
          onClick={() => setStarted(true)}
          type="button"
        >
          Start application
        </button>
      )}
    </section>
  );
}
