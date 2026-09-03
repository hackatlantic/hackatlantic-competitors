import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { assertStagingTarget } from "./profile-contract.mjs";

export const FORM_SOURCE = "api/migrations/000013_hackatlantic_application_form_v2.sql";
// Read only the published schema, never execute this migration: it also moves
// existing drafts, which the staging benchmark alignment must NOT do.
const migration = readFileSync(new URL("../../" + FORM_SOURCE, import.meta.url), "utf8");
const match = migration.match(/'(\{\s*"resumeRequired"[\s\S]*?\})'::jsonb/);
if (!match) throw new Error("Current intake schema was not found in its migration");
export const currentIntakeSchema = JSON.parse(match[1]);
if (currentIntakeSchema.resumeRequired !== false || currentIntakeSchema.questions.length !== 9) throw new Error("Review the changed intake schema before using it in benchmarks");

function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") return Object.fromEntries(Object.keys(value).sort().map((key) => [key, canonical(value[key])]));
  return value;
}
export function schemaFingerprint(form) {
  const schema = { resumeRequired: form.resumeRequired, questions: form.questions, ...(form.resumeAfterQuestionKey ? { resumeAfterQuestionKey: form.resumeAfterQuestionKey } : {}) };
  return createHash("sha256").update(JSON.stringify(canonical(schema))).digest("hex");
}
export const CURRENT_SCHEMA_SHA256 = schemaFingerprint(currentIntakeSchema);
export function assertCurrentIntake(form) {
  if (schemaFingerprint(form) !== CURRENT_SCHEMA_SHA256) throw new Error("API form differs from the current nine-question optional-resume schema");
}
export function assertAlignmentScope({ apiBaseURL, applicantProfile, applicantCount, scannerIdentityCount, scannerPassCount }) {
  assertStagingTarget(apiBaseURL);
  if (!["sustained", "deadline"].includes(applicantProfile) || applicantCount < 1 || scannerIdentityCount !== 0 || scannerPassCount !== 0) throw new Error("Form alignment requires an applicant-only sustained or deadline staging run");
}

// Insert one new immutable version only if the observed current form is still
// current. A concurrent publisher causes no insert or a uniqueness failure;
// neither outcome overwrites its work. Existing drafts/submissions stay pinned.
export const PUBLISH_CURRENT_FORM_SQL = `
WITH observed AS (
  SELECT forms.* FROM ats.application_forms forms
  JOIN ats.application_cycles cycle ON cycle.id = forms.cycle_id
  WHERE cycle.id = :'cycle_id'::uuid AND cycle.active
    AND cycle.applications_open_at <= CURRENT_TIMESTAMP
    AND CURRENT_TIMESTAMP < cycle.applications_close_at
    AND forms.published_at IS NOT NULL
    AND (SELECT count(*) FROM ats.application_cycles WHERE active) = 1
  ORDER BY forms.version DESC LIMIT 1
), inserted AS (
  INSERT INTO ats.application_forms (cycle_id, version, schema_json, published_at, created_by)
  SELECT observed.cycle_id,
    (SELECT MAX(version) + 1 FROM ats.application_forms WHERE cycle_id = observed.cycle_id),
    :'schema'::jsonb, CURRENT_TIMESTAMP, creator.id
  FROM observed JOIN ats.users creator ON creator.clerk_user_id = :'admin_clerk_user_id'
  WHERE observed.id = :'expected_form_id'::uuid
    AND observed.schema_json IS DISTINCT FROM :'schema'::jsonb
  RETURNING id, version
)
SELECT jsonb_build_object('id', id, 'version', version) FROM inserted;
`;
