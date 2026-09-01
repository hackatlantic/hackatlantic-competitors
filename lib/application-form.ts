import type {
  ApplicationAnswers,
  ApplicationFormQuestion,
  CurrentApplicationForm,
} from "@/lib/api";

export function applicationWordCount(value: string): number {
  const trimmed = value.trim();
  return trimmed ? trimmed.split(/\s+/u).length : 0;
}

export function isApplicationQuestionVisible(
  question: ApplicationFormQuestion,
  answers: ApplicationAnswers,
): boolean {
  if (!question.showWhen) {
    return true;
  }
  return answers[question.showWhen.key] === question.showWhen.equals;
}

function validEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/u.test(value.trim());
}

export function validateApplicationAnswers(
  form: CurrentApplicationForm,
  answers: ApplicationAnswers,
  requireComplete: boolean,
): Record<string, string> {
  const errors: Record<string, string> = {};

  for (const question of form.questions) {
    if (!isApplicationQuestionVisible(question, answers)) {
      continue;
    }

    const value = answers[question.key];
    const empty =
      value === undefined ||
      (typeof value === "string" && value.trim().length === 0);

    if (requireComplete && question.required && empty) {
      errors[question.key] = "This question is required.";
      continue;
    }
    if (empty) {
      continue;
    }
    if (question.type === "string" && typeof value !== "string") {
      errors[question.key] = "Enter a valid response.";
      continue;
    }
    if (question.type === "boolean" && typeof value !== "boolean") {
      errors[question.key] = "Choose Yes or No.";
      continue;
    }
    if (question.type === "number" && typeof value !== "number") {
      errors[question.key] = "Enter a valid number.";
      continue;
    }
    if (
      typeof value === "string" &&
      question.options?.length &&
      !question.options.includes(value)
    ) {
      errors[question.key] = "Choose one of the available options.";
    }
    if (
      typeof value === "string" &&
      question.maxWords &&
      applicationWordCount(value) > question.maxWords
    ) {
      errors[question.key] = `Keep your response to ${question.maxWords} words or fewer.`;
    }
    if (
      typeof value === "string" &&
      question.control === "email" &&
      !validEmail(value)
    ) {
      errors[question.key] = "Enter a valid email address.";
    }
  }

  return errors;
}
