"use client";

import { useAuth } from "@clerk/nextjs";
import { useRouter } from "next/navigation";
import { type FormEvent, useMemo, useState } from "react";
import {
  ApiError,
  createApiClient,
  type ReviewerRecommendation,
  type ReviewerReview,
  type ReviewScore,
} from "@/lib/api";

type ReviewAction = "idle" | "saving" | "submitting";

type ReviewerReviewFormProps = {
  applicationId: string;
  initialReview: ReviewerReview | null | undefined;
};

const recommendationOptions: Array<{
  value: ReviewerRecommendation;
  label: string;
}> = [
  { value: "strong_yes", label: "Strong yes" },
  { value: "yes", label: "Yes" },
  { value: "neutral", label: "Neutral" },
  { value: "no", label: "No" },
  { value: "strong_no", label: "Strong no" },
];

export function ReviewerReviewForm({
  applicationId,
  initialReview,
}: ReviewerReviewFormProps) {
  const reviewKey = `${applicationId}:${initialReview?.id ?? "new"}:${initialReview?.lockVersion ?? 0}`;

  return (
    <ReviewerReviewFormFields
      key={reviewKey}
      applicationId={applicationId}
      initialReview={initialReview}
    />
  );
}

function ReviewerReviewFormFields({
  applicationId,
  initialReview,
}: ReviewerReviewFormProps) {
  const { getToken, isLoaded } = useAuth();
  const client = useMemo(() => createApiClient({ getToken }), [getToken]);
  const router = useRouter();
  const [review, setReview] = useState<ReviewerReview | null>(initialReview ?? null);
  const [score, setScore] = useState<ReviewScore | null>(initialReview?.score ?? null);
  const [recommendation, setRecommendation] = useState<ReviewerRecommendation | "">(
    initialReview?.recommendation ?? "",
  );
  const [internalNotes, setInternalNotes] = useState(initialReview?.internalNotes ?? "");
  const [action, setAction] = useState<ReviewAction>("idle");
  const [message, setMessage] = useState("");

  const validateReview = (): boolean => {
    if (score && recommendation) {
      return true;
    }

    setMessage("Choose a score from 1 to 5 and a recommendation before saving.");
    return false;
  };

  const saveDraft = async (): Promise<ReviewerReview | null> => {
    if (!validateReview() || !score || !recommendation) {
      return null;
    }

    const savedApplication = await client.saveReviewDraft(applicationId, {
      lockVersion: review?.lockVersion ?? 0,
      score,
      recommendation,
      ...(internalNotes.trim() ? { internalNotes } : {}),
    });
    if (!savedApplication.review) {
      throw new Error("The saved review was unavailable in the server response.");
    }
    setReview(savedApplication.review);
    return savedApplication.review;
  };

  const handleSave = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    setAction("saving");

    try {
      const savedReview = await saveDraft();
      if (savedReview) {
        setMessage("Review draft saved.");
      }
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        router.refresh();
        setMessage("This review changed elsewhere. The latest saved review was reloaded.");
      } else {
        setMessage(error instanceof Error ? error.message : "Unable to save the review.");
      }
    } finally {
      setAction("idle");
    }
  };

  const handleSubmit = async () => {
    setMessage("");
    setAction("submitting");

    try {
      const savedReview = await saveDraft();
      if (!savedReview) {
        return;
      }
      const submittedApplication = await client.submitReview(
        applicationId,
        savedReview.lockVersion,
      );
      if (!submittedApplication.review) {
        throw new Error("The submitted review was unavailable in the server response.");
      }
      setReview(submittedApplication.review);
      setMessage("Review submitted. Submitted reviews cannot be edited.");
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        router.refresh();
        setMessage("This review changed elsewhere. The latest saved review was reloaded.");
      } else {
        setMessage(error instanceof Error ? error.message : "Unable to submit the review.");
      }
    } finally {
      setAction("idle");
    }
  };

  if (!isLoaded) {
    return <p className="staff-muted">Loading review controls…</p>;
  }

  if (review?.status === "submitted") {
    return (
      <section className="submitted-confirmation" aria-live="polite">
        <h2>Review submitted</h2>
        <p>
          Score {review.score} · {review.recommendation.replace("_", " ")}
          {review.submittedAt ? ` · Submitted ${review.submittedAt}` : ""}
        </p>
        <p>Submitted reviews are locked and remain internal to the staff workflow.</p>
      </section>
    );
  }

  return (
    <form className="application-form" noValidate onSubmit={handleSave}>
      <fieldset className="review-rubric">
        <legend>Review rubric</legend>
        <p className="staff-summary">Score and recommendation are required.</p>

        <div>
          <label htmlFor="review-score">Score</label>
          <select
            id="review-score"
            onChange={(event) => {
              const nextScore = Number(event.target.value);
              setScore(
                nextScore >= 1 && nextScore <= 5 ? (nextScore as ReviewScore) : null,
              );
            }}
            required
            value={score ?? ""}
          >
            <option value="">Choose a score</option>
            <option value="1">1</option>
            <option value="2">2</option>
            <option value="3">3</option>
            <option value="4">4</option>
            <option value="5">5</option>
          </select>
        </div>

        <div>
          <label htmlFor="review-recommendation">Recommendation</label>
          <select
            id="review-recommendation"
            onChange={(event) =>
              setRecommendation(event.target.value as ReviewerRecommendation | "")
            }
            required
            value={recommendation}
          >
            <option value="">Choose a recommendation</option>
            {recommendationOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label htmlFor="review-internal-notes">Internal notes (optional)</label>
          <textarea
            id="review-internal-notes"
            onChange={(event) => setInternalNotes(event.target.value)}
            rows={6}
            value={internalNotes}
          />
        </div>
      </fieldset>

      {message ? (
        <p
          className={
            message === "Review draft saved."
              ? "application-notice"
              : "error-message"
          }
          role={message === "Review draft saved." ? "status" : "alert"}
        >
          {message}
        </p>
      ) : null}

      <div className="application-actions">
        <button className="button secondary" disabled={action !== "idle"} type="submit">
          {action === "saving" ? "Saving…" : "Save draft"}
        </button>
        <button
          className="button primary"
          disabled={action !== "idle"}
          onClick={() => void handleSubmit()}
          type="button"
        >
          {action === "submitting" ? "Submitting…" : "Submit review"}
        </button>
      </div>
    </form>
  );
}
