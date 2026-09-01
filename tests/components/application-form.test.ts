import { describe, expect, it } from "vitest";
import {
  applicationWordCount,
  isApplicationQuestionVisible,
  validateApplicationAnswers,
} from "@/lib/application-form";
import type { CurrentApplicationForm } from "@/lib/api";

const form: CurrentApplicationForm = {
  id: "70000000-0000-4000-8000-000000000004",
  cycleId: "70000000-0000-4000-8000-000000000002",
  version: 2,
  resumeRequired: false,
  resumeAfterQuestionKey: "school",
  questions: [
    { key: "excitement", label: "Excitement", type: "string", required: true, maxWords: 3 },
    { key: "experience", label: "Experience", type: "string", required: true, control: "select", options: ["First", "1–3", "3+"] },
    { key: "hardware", label: "Hardware?", type: "boolean", required: true },
    { key: "equipment", label: "Equipment", type: "string", required: true, showWhen: { key: "hardware", equals: true } },
  ],
};

describe("application form rules", () => {
  it("counts words and enforces the configured maximum", () => {
    expect(applicationWordCount("  one   two three ")).toBe(3);
    expect(validateApplicationAnswers(form, {
      excitement: "one two three four",
      experience: "First",
      hardware: false,
    }, true).excitement).toMatch(/3 words/);
  });

  it("requires equipment only when hardware is selected", () => {
    const equipment = form.questions[3];
    expect(isApplicationQuestionVisible(equipment, { hardware: false })).toBe(false);
    expect(validateApplicationAnswers(form, {
      excitement: "one two",
      experience: "First",
      hardware: false,
    }, true)).toEqual({});
    expect(validateApplicationAnswers(form, {
      excitement: "one two",
      experience: "First",
      hardware: true,
    }, true).equipment).toBe("This question is required.");
  });
});

