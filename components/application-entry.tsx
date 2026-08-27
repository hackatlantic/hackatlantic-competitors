"use client";

import { SignInButton } from "@clerk/nextjs";
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
      <div className="application-start-copy">
        <h1 id="application-start-heading">
          Ready to start your Hack Atlantic application?
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
