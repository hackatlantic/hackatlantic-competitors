"use client";

import { useAuth } from "@clerk/nextjs";
import { ApiError, createApiClient } from "@/lib/api";
import { useEffect, useMemo, useState } from "react";

export function AdminResumeViewer({ applicationId }: { applicationId: string }) {
  const { getToken, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [url, setUrl] = useState<string | null>(null);
  const [state, setState] = useState<"loading" | "ready" | "missing" | "error">("loading");

  useEffect(() => {
    if (!isLoaded) return;
    let objectUrl: string | null = null;
    let cancelled = false;
    void client.downloadAdminApplicationResume(applicationId)
      .then((blob) => {
        if (cancelled) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
        setState("ready");
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setState(error instanceof ApiError && error.status === 404 ? "missing" : "error");
      });
    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [applicationId, client, isLoaded]);

  if (state === "loading") return <p className="staff-muted">Loading resume…</p>;
  if (state === "missing") return <p className="staff-muted">No resume was attached to this application.</p>;
  if (state === "error" || !url) return <p className="error-message">The resume could not be displayed.</p>;

  return (
    <div className="resume-viewer">
      <iframe src={url} title="Applicant resume PDF" />
      <p className="staff-muted">
        If your browser cannot render PDFs, <a className="staff-link" href={url} target="_blank" rel="noreferrer">open the resume in a new tab</a>.
      </p>
    </div>
  );
}
