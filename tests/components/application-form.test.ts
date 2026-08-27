import { describe, expect, it } from "vitest";
import {
  applicationQuestionLabel,
  hackAtlantic2026ApplicationQuestions,
  orderedApplicationAnswers,
} from "@/lib/application-form";

describe("HackAtlantic application question metadata", () => {
  it("matches the published 2026 intake question keys in order", () => {
    expect(hackAtlantic2026ApplicationQuestions.map((question) => question.key)).toEqual([
      "name",
      "email",
      "school",
      "hackAtlanticExcitement",
      "priorHackathonExperience",
      "desiredTeammates",
      "hardwareProject",
      "hardwareEquipment",
      "dietaryRestrictions",
    ]);
  });

  it("renders submitted answers with UI labels and published form ordering", () => {
    const ordered = orderedApplicationAnswers({
      dietaryRestrictions: "None",
      priorHackathonExperience: "1-3 hackathons",
      name: "Ada Lovelace",
      unknownFutureQuestion: "Future answer",
    });

    expect(ordered.map(([key]) => key)).toEqual([
      "name",
      "priorHackathonExperience",
      "dietaryRestrictions",
      "unknownFutureQuestion",
    ]);
    expect(applicationQuestionLabel("priorHackathonExperience")).toBe(
      "Prior Hackathon Experience",
    );
    expect(applicationQuestionLabel("unknownFutureQuestion")).toBe(
      "unknownFutureQuestion",
    );
  });
});
