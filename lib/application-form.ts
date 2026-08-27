import type {
  ApplicationAnswers,
  CurrentApplicationForm,
} from "@/lib/api";

export const hackAtlantic2026ApplicationQuestions = [
  {
    key: "name",
    label: "Name",
    type: "string",
    required: true,
  },
  {
    key: "email",
    label: "Email",
    type: "string",
    required: true,
  },
  {
    key: "school",
    label: "School",
    type: "string",
    required: true,
  },
  {
    key: "hackAtlanticExcitement",
    label: "What are you most excited about at Hack Atlantic?",
    type: "string",
    required: true,
    help: "Maximum 100 words.",
    maxWords: 100,
  },
  {
    key: "priorHackathonExperience",
    label: "Prior Hackathon Experience",
    type: "string",
    required: true,
  },
  {
    key: "desiredTeammates",
    label: "Desired teammate names",
    type: "string",
    required: false,
  },
  {
    key: "hardwareProject",
    label: "Are you looking to make a hardware project?",
    type: "boolean",
    required: true,
  },
  {
    key: "hardwareEquipment",
    label: "What equipment are you looking to use?",
    type: "string",
    required: false,
  },
  {
    key: "dietaryRestrictions",
    label: "Dietary Restrictions",
    type: "string",
    required: false,
  },
] satisfies CurrentApplicationForm["questions"];

const knownQuestionsByKey = new Map(
  hackAtlantic2026ApplicationQuestions.map((question, index) => [
    question.key,
    { ...question, index },
  ]),
);

export function applicationQuestionLabel(key: string): string {
  return knownQuestionsByKey.get(key)?.label ?? key;
}

export function orderedApplicationAnswers(
  answers: ApplicationAnswers,
): Array<[string, ApplicationAnswers[string]]> {
  return Object.entries(answers).sort(([left], [right]) => {
    const leftIndex = knownQuestionsByKey.get(left)?.index ?? Number.MAX_SAFE_INTEGER;
    const rightIndex =
      knownQuestionsByKey.get(right)?.index ?? Number.MAX_SAFE_INTEGER;
    if (leftIndex !== rightIndex) {
      return leftIndex - rightIndex;
    }
    return left.localeCompare(right);
  });
}
