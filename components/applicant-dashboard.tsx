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
} from "@/lib/api";
import { hackAtlantic2026ApplicationQuestions } from "@/lib/application-form";
import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";

type FieldErrors = Record<string, string>;
type BusyAction = "submitting" | "uploading-resume" | null;
type ApplicationStep = 1 | 2;

type ApplicantDashboardProps = {
  previewMode?: boolean;
};

const profileQuestionKeys = new Set([
  "name",
  "email",
  "school",
  "dietaryRestrictions",
]);
const shortTextQuestionKeys = new Set([
  "name",
  "email",
  "school",
  "desiredTeammates",
  "hardwareEquipment",
  "dietaryRestrictions",
]);
const stringChoiceOptions: Record<string, string[]> = {
  priorHackathonExperience: [
    "This is my first hackathon",
    "1-3 hackathons",
    "3+ hackathons",
  ],
};

const questionPlaceholders: Record<string, string> = {
  name: "Your full name",
  email: "you@example.com",
  school: "Your university or institution",
  hackAtlanticExcitement: "Tell us what excites you most about this event...",
  desiredTeammates: "e.g. Alex Chen, Jordan Smith",
  hardwareEquipment: "e.g. Raspberry Pi, Arduino, sensors...",
  dietaryRestrictions: "None, vegetarian, vegan, gluten-free, etc.",
};

const previewTimestamp = "2026-08-26T12:00:00.000Z";

const previewForm: CurrentApplicationForm = {
  id: "preview-form",
  cycleId: "preview-cycle",
  version: 4,
  resumeRequired: false,
  questions: hackAtlantic2026ApplicationQuestions,
};

function createPreviewApplication(
  answers: ApplicationAnswers = {},
  overrides: Partial<ApplicantApplication> = {},
): ApplicantApplication {
  return {
    id: "preview-application",
    cycleId: previewForm.cycleId,
    formId: previewForm.id,
    formVersion: previewForm.version,
    status: "draft",
    lockVersion: 1,
    answers,
    createdAt: previewTimestamp,
    updatedAt: previewTimestamp,
    ...overrides,
  };
}

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

function wordCount(value: string): number {
  return value.trim().split(/\s+/).filter(Boolean).length;
}

function answerWordLimitErrors(
  questions: CurrentApplicationForm["questions"],
  answers: ApplicationAnswers,
): FieldErrors {
  const result: FieldErrors = {};
  for (const question of questions) {
    if (!question.maxWords || question.type !== "string") {
      continue;
    }
    const value = answers[question.key];
    if (typeof value === "string" && wordCount(value) > question.maxWords) {
      result[question.key] = `Use ${question.maxWords} words or fewer.`;
    }
  }
  return result;
}

function requiredAnswerErrors(
  questions: CurrentApplicationForm["questions"],
  answers: ApplicationAnswers,
): FieldErrors {
  const result: FieldErrors = {};
  for (const question of questions) {
    if (!question.required) {
      continue;
    }
    const value = answers[question.key];
    if (
      value === undefined ||
      (question.type === "string" && String(value).trim() === "")
    ) {
      result[question.key] = "This question is required.";
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

export function ApplicantDashboard({ previewMode = false }: ApplicantDashboardProps) {
  const { getToken, isLoaded } = useAuth();
  const effectivePreviewMode =
    previewMode || process.env.NEXT_PUBLIC_APPLICATION_PREVIEW_MODE === "true";
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const [currentForm, setCurrentForm] = useState<CurrentApplicationForm | null>(
    null,
  );
  const [application, setApplication] = useState<ApplicantApplication | null>(
    null,
  );
  const [resume, setResume] = useState<ApplicationResume | null>(null);
  const [answers, setAnswers] = useState<ApplicationAnswers>({});
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [notice, setNotice] = useState("");
  const [loadError, setLoadError] = useState("");
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [loading, setLoading] = useState(true);
  const [applicationStep, setApplicationStep] = useState<ApplicationStep>(1);
  const [decision, setDecision] = useState<ApplicantReleasedDecision | null>(null);
  const [decisionState, setDecisionState] = useState<DecisionLoadState>("loading");

  const replaceApplication = useCallback((next: ApplicantApplication) => {
    setApplication(next);
    setAnswers(next.answers);
    setFieldErrors({});
  }, []);

  const loadReleasedDecision = useCallback(
    async (applicationId: string) => {
      if (effectivePreviewMode) {
        setDecision(null);
        setDecisionState("empty");
        return;
      }
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
    [client, effectivePreviewMode],
  );
  const loadResume = useCallback(
    async (applicationId: string) => {
      if (effectivePreviewMode) {
        setResume(null);
        return;
      }
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
    [client, effectivePreviewMode],
  );
  const loadDashboard = useCallback(async () => {
    setLoadError("");
    setNotice("");

    try {
      if (effectivePreviewMode) {
        const previewApplication = createPreviewApplication();
        setCurrentForm(previewForm);
        replaceApplication(previewApplication);
        setDecision(null);
        setDecisionState("empty");
        setResume(null);
        return;
      }

      const applications = await client.getMyApplications();
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
  }, [client, effectivePreviewMode, loadReleasedDecision, loadResume, replaceApplication]);

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
      if (effectivePreviewMode) {
        setResume({
          applicationId: application.id,
          originalFilename: file.name,
          mediaType: "application/pdf",
          byteSize: file.size,
          uploadedAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        });
        setNotice("Resume uploaded.");
        return;
      }
      setResume(await client.uploadApplicationResume(application.id, file));
      setNotice("Resume uploaded.");
    } catch (error) {
      setNotice(errorMessage(error, "Unable to upload your resume."));
    } finally {
      setBusyAction(null);
    }
  };

  useEffect(() => {
    if (!effectivePreviewMode && !isLoaded) {
      return;
    }
    const scheduledLoad = window.setTimeout(() => {
      void loadDashboard();
    }, 0);
    return () => window.clearTimeout(scheduledLoad);
  }, [effectivePreviewMode, isLoaded, loadDashboard]);

  const updateAnswer = (key: string, value: ApplicationAnswer | undefined) => {
    setAnswers((current) => {
      const next = { ...current };
      if (value === undefined) {
        delete next[key];
      } else {
        next[key] = value;
      }
      if (key === "hardwareProject" && value !== true) {
        delete next.hardwareEquipment;
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
  };

  const submitApplication = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (applicationStep === 1) {
      if (currentForm) {
        const clientErrors = requiredAnswerErrors(profileQuestions, answers);
        if (Object.keys(clientErrors).length > 0) {
          setFieldErrors(clientErrors);
          setNotice("Complete the highlighted questions.");
          return;
        }
      }
      setApplicationStep(2);
      return;
    }
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

    const clientErrors = {
      ...requiredAnswerErrors(currentForm.questions, answers),
      ...answerWordLimitErrors(currentForm.questions, answers),
    };
    if (Object.keys(clientErrors).length > 0) {
      setFieldErrors(clientErrors);
      setNotice("Check the highlighted answers.");
      setBusyAction(null);
      return;
    }

    try {
      if (effectivePreviewMode) {
        replaceApplication({
          ...application,
          answers,
          status: "submitted",
          submittedAt: new Date().toISOString(),
          lockVersion: application.lockVersion + 1,
          updatedAt: new Date().toISOString(),
        });
        setNotice("Your application has been submitted.");
        return;
      }
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
  const profileQuestions = questions.filter((question) =>
    profileQuestionKeys.has(question.key),
  );
  const hackathonQuestions = questions.filter(
    (question) => !profileQuestionKeys.has(question.key),
  );
  const submitted = application.status === "submitted";
  const activeStep = submitted ? 2 : applicationStep;
  const stepTitle =
    applicationStep === 1
      ? "Part 1 - build your profile."
      : "Part 2 - hackathon questions.";
  const progressSteps = [
    {
      number: "01",
      label: "Profile",
      state: activeStep > 1 || submitted ? "complete" : "current",
    },
    {
      number: "02",
      label: "Questions",
      state: submitted ? "complete" : activeStep === 2 ? "current" : "upcoming",
    },
    {
      number: "03",
      label: "Decision",
      state: decisionState === "ready" ? "complete" : submitted ? "current" : "upcoming",
    },
  ];
  const renderQuestion = (
    question: CurrentApplicationForm["questions"][number],
    index: number,
  ) => {
    if (question.key === "hardwareEquipment" && answers.hardwareProject !== true) {
      return null;
    }
    const value = answers[question.key];
    const helpId = `question-${question.key}-${index}-help`;
    const errorId = `question-${question.key}-${index}-error`;
    const describedBy = [
      question.help ? helpId : undefined,
      fieldErrors[question.key] ? errorId : undefined,
    ]
      .filter(Boolean)
      .join(" ") || undefined;
    const stringOptions = stringChoiceOptions[question.key];

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
            <label htmlFor={`question-${question.key}-${index}`}>
              {question.label}
              {question.required ? <span aria-hidden="true"> *</span> : null}
              {!question.required ? <span className="optional-label"> (optional)</span> : null}
            </label>
            {question.help ? <p id={helpId}>{question.help}</p> : null}
            {question.type === "number" ? (
              <input
                aria-describedby={describedBy}
                aria-invalid={Boolean(fieldErrors[question.key])}
                aria-required={question.required}
                id={`question-${question.key}-${index}`}
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
            ) : stringOptions ? (
              <div className="choice-options">
                {stringOptions.map((option) => (
                  <label key={option}>
                    <input
                      checked={value === option}
                      name={question.key}
                      onChange={() => updateAnswer(question.key, option)}
                      type="radio"
                    />
                    {option}
                  </label>
                ))}
              </div>
            ) : shortTextQuestionKeys.has(question.key) ? (
              <input
                aria-describedby={describedBy}
                aria-invalid={Boolean(fieldErrors[question.key])}
                aria-required={question.required}
                id={`question-${question.key}-${index}`}
                onChange={(event) =>
                  updateAnswer(question.key, event.target.value || undefined)
                }
                placeholder={questionPlaceholders[question.key]}
                type={question.key === "email" ? "email" : "text"}
                value={typeof value === "string" ? value : ""}
              />
            ) : (
              <textarea
                aria-describedby={describedBy}
                aria-invalid={Boolean(fieldErrors[question.key])}
                aria-required={question.required}
                id={`question-${question.key}-${index}`}
                onChange={(event) =>
                  updateAnswer(question.key, event.target.value || undefined)
                }
                placeholder={questionPlaceholders[question.key]}
                rows={3}
                value={typeof value === "string" ? value : ""}
              />
            )}
            {question.maxWords && typeof value === "string" ? (
              <p className="question-limit">
                {wordCount(value)} / {question.maxWords}
              </p>
            ) : null}
          </>
        )}
        {fieldErrors[question.key] ? (
          <p className="field-error" id={errorId} role="alert">
            {fieldErrors[question.key]}
          </p>
        ) : null}
      </div>
    );
  };

  return (
    <motion.section
      className="application-panel"
      aria-labelledby="application-heading"
      layout
      transition={{ layout: { duration: 0.28, ease: [0.22, 1, 0.36, 1] } }}
    >
      <ol className="application-progress" aria-label="Application progress">
        {progressSteps.map((step) => (
          <li className={`progress-step ${step.state}`} key={step.number}>
            <span>{step.number}</span>
            <strong>{step.label}</strong>
          </li>
        ))}
      </ol>
      <div className="application-heading">
        <div>
          <h1 id="application-heading">{submitted ? "Application submitted" : stepTitle}</h1>
        </div>
        {submitted ? (
          <motion.span className="status-pill submitted" layout>
            Submitted
          </motion.span>
        ) : null}
      </div>

      {submitted || !currentForm ? (
        <p className="application-summary">
          {submitted
            ? "Submitted applications cannot be edited."
            : "The application window is closed."}
        </p>
      ) : null}

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
          <div className={applicationStep === 1 ? "profile-question-grid" : undefined}>
            {(applicationStep === 1 ? profileQuestions : hackathonQuestions).map(
              renderQuestion,
            )}
          </div>

          {currentForm && applicationStep === 1 ? (
            <div className="question-field resume-upload-field">
              <p className="question-label">
                Resume {currentForm.resumeRequired ? <span aria-hidden="true">*</span> : <span>(optional)</span>}
              </p>
              <p>Upload one PDF, up to 5 MB.</p>
              <input
                accept="application/pdf,.pdf"
                className="resume-file-input"
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
              <label className="resume-file-trigger" htmlFor="application-resume">
                <span>Choose PDF</span>
                <small>{resume?.originalFilename ?? "No file selected"}</small>
              </label>
              {busyAction === "uploading-resume" ||
              resume ||
              currentForm.resumeRequired ? (
                <p
                  aria-live="polite"
                  className={resume ? "resume-summary" : "field-error"}
                >
                  {busyAction === "uploading-resume"
                    ? "Uploading resume…"
                    : resume
                      ? `${resume.originalFilename} · ${Math.ceil(resume.byteSize / 1024)} KB`
                      : "A PDF resume is required before submission."}
                </p>
              ) : null}
            </div>
          ) : null}

          <div className="application-actions">
            <button className="button primary" disabled={busyAction !== null} type="submit">
              {applicationStep === 1
                ? "Next"
                : busyAction === "submitting"
                  ? "Submitting…"
                  : "Submit application"}
            </button>
            {applicationStep === 2 ? (
              <button
                className="button secondary"
                disabled={busyAction !== null}
                onClick={() => setApplicationStep(1)}
                type="button"
              >
                Back
              </button>
            ) : null}
          </div>
        </form>
      ) : (
        <div className="submitted-confirmation" aria-live="polite">
          <h2>Application window closed</h2>
          <p>
            This application can no longer be changed or submitted.
          </p>
        </div>
      )}
    </motion.section>
  );
}
