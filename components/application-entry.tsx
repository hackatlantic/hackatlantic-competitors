"use client";

import { useState } from "react";
import { ApplicantDashboard } from "@/components/applicant-dashboard";

type ApplicationEntryProps = {
  previewMode?: boolean;
};

export function ApplicationEntry({ previewMode = false }: ApplicationEntryProps) {
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
      <button
        className="button primary"
        onClick={() => setStarted(true)}
        type="button"
      >
        Start application
      </button>
    </section>
  );
}
