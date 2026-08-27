"use client";

import { useAuth } from "@clerk/nextjs";
import { AnimatePresence, motion } from "framer-motion";
import { ApplicantPass } from "@/components/applicant-pass";
import {
  ApplicantDecisionStatus,
  type DecisionLoadState,
} from "@/components/applicant-decision-status";
import {
  type ApplicationAnswer,
  type ApplicantApplication,
  type ApplicantReleasedDecision,
  type ApplicationResume,
  type ApplicationAnswers,
  ApiError,
  createApiClient,
  type CurrentApplicationForm,
  type CurrentUser,
} from "@/lib/api";
import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";

type FieldErrors = Record<string, string>;
type BusyAction = "saving" | "submitting" | "uploading-resume" | null;

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function isNoCurrentApplicationFormError(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    error.status === 404 &&
    error.body.code === "current_form_not_found"
  );
}

function isNoReleasedDecisionError(error: unknown): error is ApiError {
  return (
    error instanceof ApiError &&
    error.status === 404 &&
    error.body.code === "decision_not_found"
  );
}

function validationErrors(error: unknown): FieldErrors {
  if (!(error instanceof ApiError) || !error.body.details) {
    return {};
  }

  const details = error.body.details;
  if (typeof details !== "object" || Array.isArray(details)) {
    return {};
  }

  const errorDetails = details as Record<string, unknown>;
  const fields =
    errorDetails.fields ?? errorDetails.errors ?? errorDetails.answerErrors ?? details;
  if (typeof fields !== "object" || Array.isArray(fields)) {
    return {};
  }

  const result: FieldErrors = {};
  for (const [key, value] of Object.entries(fields)) {
    if (typeof value === "string") {
      result[key] = value;
    } else if (
      Array.isArray(value) &&
      value.every((item) => typeof item === "string")
    ) {
      result[key] = value.join(" ");
    }
  }

  return result;
}

function displayTimestamp(value: string): string {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "long",
    timeStyle: "short",
  }).format(timestamp);
}

export function ApplicantDashboard() {
  const { getToken, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [currentForm, setCurrentForm] = useState<CurrentApplicationForm | null>(
    null,
  );
  const [application, setApplication] = useState<ApplicantApplication | null>(
    null,
  );
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);
  const [resume, setResume] = useState<ApplicationResume | null>(null);
  const [answers, setAnswers] = useState<ApplicationAnswers>({});
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [notice, setNotice] = useState("");
  const [loadError, setLoadError] = useState("");
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [loading, setLoading] = useState(true);
  const [isDirty, setIsDirty] = useState(false);
  const [decision, setDecision] = useState<ApplicantReleasedDecision | null>(null);
  const [decisionState, setDecisionState] = useState<DecisionLoadState>("loading");

  const replaceApplication = useCallback((next: ApplicantApplication) => {
    setApplication(next);
    setAnswers(next.answers);
    setFieldErrors({});
    setIsDirty(false);
  }, []);

  const loadReleasedDecision = useCallback(
    async (applicationId: string) => {
      setDecision(null);
      setDecisionState("loading");

      try {
        const releasedDecision = await client.getApplicationDecision(applicationId);
        setDecision(releasedDecision);
        setDecisionState("ready");
      } catch (error) {
        setDecisionState(isNoReleasedDecisionError(error) ? "empty" : "error");
      }
    },
    [client],
  );
  const loadResume = useCallback(
    async (applicationId: string) => {
      try {
        setResume(await client.getApplicationResume(applicationId));
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          setResume(null);
          return;
        }
        throw error;
      }
    },
    [client],
  );
  const loadDashboard = useCallback(async () => {
    setLoadError("");
    setNotice("");

    try {
      const [applications, user] = await Promise.all([
        client.getMyApplications(),
        client.getCurrentUser(),
      ]);
      setCurrentUser(user);
      let form: CurrentApplicationForm;
      try {
        form = await client.getCurrentApplicationForm();
      } catch (error) {
        if (!isNoCurrentApplicationFormError(error)) {
          throw error;
        }

        const mostRecentApplication = applications.items[0];
        if (!mostRecentApplication) {
          throw error;
        }

        setCurrentForm(null);
        replaceApplication(mostRecentApplication);
        await Promise.all([
          loadReleasedDecision(mostRecentApplication.id),
          loadResume(mostRecentApplication.id),
        ]);
        return;
      }

      const nextApplication =
        applications.items.find((candidate) => candidate.cycleId === form.cycleId) ??
        (await client.createApplication());

      setCurrentForm(form);
      replaceApplication(nextApplication);
      await Promise.all([
        loadReleasedDecision(nextApplication.id),
        loadResume(nextApplication.id),
      ]);
    } catch (error) {
      setLoadError(errorMessage(error, "Unable to load your application."));
    } finally {
      setLoading(false);
    }
  }, [client, loadReleasedDecision, loadResume, replaceApplication]);

  const uploadResume = async (file: File) => {
    if (!application || application.status !== "draft") {
      return;
    }
    if (file.type !== "application/pdf" || !file.name.toLowerCase().endsWith(".pdf")) {
      setNotice("Resume must be a PDF file.");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      setNotice("Resume must be 5 MB or smaller.");
      return;
    }
    setBusyAction("uploading-resume");
    setNotice("");
    try {
      setResume(await client.uploadApplicationResume(application.id, file));
      setNotice("Resume uploaded.");
    } catch (error) {
      setNotice(errorMessage(error, "Unable to upload your resume."));
    } finally {
      setBusyAction(null);
    }
  };

  useEffect(() => {
    if (!isLoaded) {
      return;
    }
    const scheduledLoad = window.setTimeout(() => {
      void loadDashboard();
    }, 0);
    return () => window.clearTimeout(scheduledLoad);
  }, [isLoaded, loadDashboard]);

  const updateAnswer = (key: string, value: ApplicationAnswer | undefined) => {
    setAnswers((current) => {
      const next = { ...current };
      if (value === undefined) {
        delete next[key];
      } else {
        next[key] = value;
      }
      return next;
    });
    setFieldErrors((current) => {
      if (!current[key]) {
        return current;
      }
      const next = { ...current };
      delete next[key];
      return next;
    });
    setNotice("");
    setIsDirty(true);
  };

  const saveDraft = async () => {
    if (!currentForm || !application || application.status !== "draft") {
      return;
    }

    setBusyAction("saving");
    setNotice("");
    setFieldErrors({});

    try {
      const savedApplication = await client.saveApplicationDraft(application.id, {
        lockVersion: application.lockVersion,
        answers,
      });
      replaceApplication(savedApplication);
      setNotice("Draft saved.");
    } catch (error) {
      setFieldErrors(validationErrors(error));
      setNotice(errorMessage(error, "Unable to save your draft."));

      if (error instanceof ApiError && error.status === 409) {
        try {
          const latestApplication = (await client.getMyApplications()).items.find(
            (candidate) => candidate.id === application.id,
          );
          if (!latestApplication) {
            setNotice("Your draft changed elsewhere. Reload the dashboard to continue.");
            return;
          }
          replaceApplication(latestApplication);
          setNotice("Your draft changed elsewhere. The latest saved answers are shown.");
        } catch (reloadError) {
          setNotice(errorMessage(reloadError, "Unable to reload the latest draft."));
        }
      }
    } finally {
      setBusyAction(null);
    }
  };

  const submitApplication = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!currentForm || !application || application.status !== "draft") {
      return;
    }
    if (currentForm.resumeRequired && !resume) {
      setNotice("Upload your PDF resume before submitting your application.");
      return;
    }

    setBusyAction("submitting");
    setNotice("");
    setFieldErrors({});

    try {
      const savedApplication = await client.saveApplicationDraft(application.id, {
        lockVersion: application.lockVersion,
        answers,
      });
      const submittedApplication = await client.submitApplication(
        savedApplication.id,
        { lockVersion: savedApplication.lockVersion },
      );
      replaceApplication(submittedApplication);
      setNotice("Your application has been submitted.");
    } catch (error) {
      setFieldErrors(validationErrors(error));
      setNotice(errorMessage(error, "Unable to submit your application."));

      if (error instanceof ApiError && error.status === 409) {
        try {
          const latestApplication = (await client.getMyApplications()).items.find(
            (candidate) => candidate.id === application.id,
          );
          if (!latestApplication) {
            setNotice("Your application changed elsewhere. Reload the dashboard to continue.");
            return;
          }
          replaceApplication(latestApplication);
          setNotice(
            "Your application state changed elsewhere. The latest saved application is shown.",
          );
        } catch (reloadError) {
          setNotice(
            errorMessage(reloadError, "Unable to reload the latest application."),
          );
        }
      }
    } finally {
      setBusyAction(null);
    }
  };

  if (!isLoaded || loading) {
    return (
      <section className="application-panel" aria-busy="true" aria-live="polite">
        <p>Loading your application…</p>
      </section>
    );
  }

  if (loadError || !application) {
    return (
      <section
        className="application-panel application-error-panel"
        aria-live="polite"
      >
        <h1>Application unavailable</h1>
        <p className="error-message" role="alert">
          {loadError || "Your application could not be loaded."}
        </p>
        <button
          className="button secondary"
          onClick={() => {
            setLoading(true);
            void loadDashboard();
          }}
          type="button"
        >
          Try again
        </button>
      </section>
    );
  }

  const questions = currentForm?.questions ?? [];
  const submitted = application.status === "submitted";

  return (
    <motion.section
      className="application-panel"
      aria-labelledby="application-heading"
      layout
      transition={{ layout: { duration: 0.28, ease: [0.22, 1, 0.36, 1] } }}
    >
      <div className="application-progress" aria-label="Application progress">
        <div className="progress-step complete"><span>01</span><strong>Account</strong></div>
        <div className={`progress-step ${submitted ? "complete" : "current"}`}><span>02</span><strong>Application</strong></div>
        <div className={`progress-step ${decisionState === "ready" ? "complete" : submitted ? "current" : ""}`}><span>03</span><strong>Decision</strong></div>
        <div className={`progress-step ${decision?.outcome === "accepted" ? "current" : ""}`}><span>04</span><strong>Event pass</strong></div>
      </div>
      <div className="application-heading">
        <div>
          <p className="eyebrow">Applicant field notes · {application.formVersion.toString().padStart(2, "0")}</p>
          <h1 id="application-heading">Your application</h1>
        </div>
        <motion.span
          className={`status-pill ${submitted ? "submitted" : "draft"}`}
          layout
        >
          {submitted ? "Submitted" : "Draft"}
        </motion.span>
      </div>

      <p className="application-summary">
        Form version {application.formVersion}.{" "}
        {submitted
          ? "Your submitted application is no longer editable."
          : currentForm
            ? "Your answers are only visible to you until you submit this application."
            : "This draft can no longer be changed or submitted because the application window is closed."}
      </p>

      <div className="application-identity" aria-label="Applicant identity">
        <div>
          <span className="coordinate-label">VERIFIED IDENTITY</span>
          <strong>{currentUser?.email ?? "Verified through Clerk"}</strong>
        </div>
        <p>Your verified account email is attached to the application when you submit.</p>
      </div>

      <AnimatePresence initial={false}>
        {notice ? (
          <motion.p
            animate={{ height: "auto", opacity: 1, y: 0 }}
            className="application-notice"
            exit={{ height: 0, opacity: 0, y: -8 }}
            initial={{ height: 0, opacity: 0, y: -8 }}
            key={notice}
            role="status"
          >
            {notice}
          </motion.p>
        ) : null}
      </AnimatePresence>

      {submitted ? (
        <>
          <div className="submitted-confirmation" aria-live="polite">
            <h2>Application submitted</h2>
            {application.submittedAt ? (
              <p>
                Submitted <time dateTime={application.submittedAt}>{displayTimestamp(application.submittedAt)}</time>.
              </p>
            ) : (
              <p>Your application was submitted successfully.</p>
            )}
          </div>
          <ApplicantDecisionStatus
            decision={decision}
            onRetry={() => void loadReleasedDecision(application.id)}
            state={decisionState}
          />
          {resume ? (
            <p className="resume-summary">
              Resume attached: <strong>{resume.originalFilename}</strong>
            </p>
          ) : null}
          {decisionState === "ready" && decision?.outcome === "accepted" ? (
            <ApplicantPass />
          ) : null}
        </>
      ) : currentForm ? (
        <form className="application-form" noValidate onSubmit={submitApplication}>
          <div className="form-intro">
            <span className="coordinate-label">SECTION A / YOUR SIGNAL</span>
            <p className="form-instructions">There is no perfect hacker profile. Give us a clear picture of what you&apos;re curious about and what you want to build.</p>
          </div>

          {questions.map((question, index) => {
            const value = answers[question.key];
            const helpId = `question-${index}-help`;
            const errorId = `question-${index}-error`;
            const describedBy = [
              question.help ? helpId : undefined,
              fieldErrors[question.key] ? errorId : undefined,
            ]
              .filter(Boolean)
              .join(" ") || undefined;

            return (
              <div className="question-field" key={question.key}>
                {question.type === "boolean" ? (
                  <fieldset
                    aria-describedby={describedBy}
                    aria-invalid={Boolean(fieldErrors[question.key])}
                    aria-required={question.required}
                  >
                    <legend>
                      {question.label}
                      {question.required ? <span aria-hidden="true"> *</span> : null}
                    </legend>
                    {question.help ? <p id={helpId}>{question.help}</p> : null}
                    <div className="boolean-options">
                      <label>
                        <input
                          checked={value === true}
                          name={question.key}
                          onChange={() => updateAnswer(question.key, true)}
                          type="radio"
                        />
                        Yes
                      </label>
                      <label>
                        <input
                          checked={value === false}
                          name={question.key}
                          onChange={() => updateAnswer(question.key, false)}
                          type="radio"
                        />
                        No
                      </label>
                    </div>
                  </fieldset>
                ) : (
                  <>
                    <label htmlFor={`question-${index}`}>
                      {question.label}
                      {question.required ? <span aria-hidden="true"> *</span> : null}
                    </label>
                    {question.help ? <p id={helpId}>{question.help}</p> : null}
                    {question.type === "number" ? (
                      <input
                        aria-describedby={describedBy}
                        aria-invalid={Boolean(fieldErrors[question.key])}
                        aria-required={question.required}
                        id={`question-${index}`}
                        inputMode="decimal"
                        onChange={(event) => {
                          const nextValue = event.target.value;
                          if (!nextValue) {
                            updateAnswer(question.key, undefined);
                            return;
                          }
                          const numberValue = Number(nextValue);
                          if (Number.isFinite(numberValue)) {
                            updateAnswer(question.key, numberValue);
                          }
                        }}
                        step="any"
                        type="number"
                        value={typeof value === "number" ? value : ""}
                      />
                    ) : (
                      <textarea
                        aria-describedby={describedBy}
                        aria-invalid={Boolean(fieldErrors[question.key])}
                        aria-required={question.required}
                        id={`question-${index}`}
                        onChange={(event) => updateAnswer(question.key, event.target.value || undefined)}
                        rows={4}
                        value={typeof value === "string" ? value : ""}
                      />
                    )}
                  </>
                )}
                {fieldErrors[question.key] ? (
                  <p className="field-error" id={errorId} role="alert">
                    {fieldErrors[question.key]}
                  </p>
                ) : null}
              </div>
            );
          })}

          {currentForm.resumeRequired ? (
            <div className="question-field resume-upload-field">
              <label htmlFor="application-resume">
                Resume <span aria-hidden="true">*</span>
              </label>
              <p>Upload one PDF, up to 5 MB. Uploading another PDF replaces the current resume.</p>
              <input
                accept="application/pdf,.pdf"
                disabled={busyAction !== null}
                id="application-resume"
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (file) {
                    void uploadResume(file);
                  }
                  event.target.value = "";
                }}
                type="file"
              />
              <p aria-live="polite" className={resume ? "resume-summary" : "field-error"}>
                {busyAction === "uploading-resume"
                  ? "Uploading resume…"
                  : resume
                    ? `${resume.originalFilename} · ${Math.ceil(resume.byteSize / 1024)} KB`
                    : "A PDF resume is required before submission."}
              </p>
            </div>
          ) : null}

          <div className="application-actions">
            <div aria-live="polite" className="save-state">
              <AnimatePresence initial={false} mode="wait">
                <motion.span
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -5 }}
                  initial={{ opacity: 0, y: 5 }}
                  key={isDirty ? "dirty" : "saved"}
                >
                  {isDirty ? "Unsaved changes" : "All changes saved"}
                </motion.span>
              </AnimatePresence>
            </div>
            <button
              className="button secondary"
              disabled={busyAction !== null}
              onClick={() => void saveDraft()}
              type="button"
            >
              {busyAction === "saving" ? "Saving…" : "Save draft"}
            </button>
            <button className="button primary" disabled={busyAction !== null} type="submit">
              {busyAction === "submitting" ? "Submitting…" : "Submit application"}
            </button>
          </div>
        </form>
      ) : (
        <div className="submitted-confirmation" aria-live="polite">
          <h2>Application window closed</h2>
          <p>
            This draft can no longer be changed or submitted because the application
            window is closed.
          </p>
        </div>
      )}
    </motion.section>
  );
}
