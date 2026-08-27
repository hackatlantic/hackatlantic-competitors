"use client";

import { SignInButton, SignUpButton } from "@clerk/nextjs";
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
      <h1 id="application-start-heading">
        Ready to Start your
        <br />
        Hack Atlantic Application?
      </h1>
      {requiresAuth ? (
        <div className="application-start-actions">
          <SignUpButton>
            <button className="button primary" type="button">
              Start application
            </button>
          </SignUpButton>
          <SignInButton>
            <button className="button secondary" type="button">
              Sign in
            </button>
          </SignInButton>
        </div>
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
