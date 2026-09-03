import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { currentIntakeSchema, CURRENT_SCHEMA_SHA256, schemaFingerprint, assertCurrentIntake, assertAlignmentScope, PUBLISH_CURRENT_FORM_SQL } from "./current-intake-form.mjs";
import { STAGING_API, applicantAnswers, shouldUpload } from "./profile-contract.mjs";

test("current schema matches the approved intake fixture and both published migration schemas", () => {
  const seed = readFileSync(new URL("../../api/cmd/seed-intake/main.go", import.meta.url), "utf8");
  assert.deepEqual(currentIntakeSchema, JSON.parse(seed.match(/var fixtureFormSchema = \[\]byte\(`([\s\S]*?)`\)/)[1]));
  assert.equal(currentIntakeSchema.questions.length, 9);
  assert.equal(currentIntakeSchema.resumeRequired, false);
  const migration = readFileSync(new URL("../../api/migrations/000013_hackatlantic_application_form_v2.sql", import.meta.url), "utf8");
  for (const match of migration.matchAll(/'(\{\s*"resumeRequired"[\s\S]*?\})'::jsonb/g)) assert.deepEqual(JSON.parse(match[1]), currentIntakeSchema);
});
test("schema fingerprint ignores identity and key ordering but detects question or requiredness changes", () => {
  assert.equal(schemaFingerprint({ ...currentIntakeSchema, id: "different", version: 42 }), CURRENT_SCHEMA_SHA256);
  assert.doesNotThrow(() => assertCurrentIntake({ questions: currentIntakeSchema.questions, resumeAfterQuestionKey: "school", resumeRequired: false }));
  assert.throws(() => assertCurrentIntake({ ...currentIntakeSchema, resumeRequired: true }));
  assert.throws(() => assertCurrentIntake({ ...currentIntakeSchema, questions: currentIntakeSchema.questions.slice(0, 3) }));
});
test("explicit form alignment cannot target production or scanner/stress profiles", () => {
  const config = { apiBaseURL: STAGING_API, applicantProfile: "sustained", applicantCount: 50, scannerIdentityCount: 0, scannerPassCount: 0 };
  assert.doesNotThrow(() => assertAlignmentScope(config));
  assert.doesNotThrow(() => assertAlignmentScope({ ...config, applicantProfile: "deadline", applicantCount: 250 }));
  for (const change of [{ apiBaseURL: "https://api.hackatlantic.ca" }, { applicantProfile: "stress" }, { scannerIdentityCount: 20 }, { scannerPassCount: 1 }, { applicantCount: 0 }]) assert.throws(() => assertAlignmentScope({ ...config, ...change }));
});
test("publication only inserts a version guarded by the observed current form", () => {
  assert.match(PUBLISH_CURRENT_FORM_SQL, /INSERT INTO ats\.application_forms/);
  assert.match(PUBLISH_CURRENT_FORM_SQL, /observed\.id = :'expected_form_id'::uuid/);
  assert.match(PUBLISH_CURRENT_FORM_SQL, /MAX\(version\) \+ 1/);
  assert.doesNotMatch(PUBLISH_CURRENT_FORM_SQL, /\b(UPDATE|DELETE|TRUNCATE|ALTER|DROP)\b/i);
  assert.doesNotMatch(PUBLISH_CURRENT_FORM_SQL, /ats\.(applications|application_answers|application_resumes)\b/);
});
test("current-form workload fills all nine fields and exercises optional upload and omission", () => {
  const answers = applicantAnswers(currentIntakeSchema, "fixture@loadtest.invalid", 0, 2);
  assert.equal(Object.keys(answers).length, 9);
  assert.equal(answers.hardwareProject, true);
  assert.ok(answers.hardwareEquipment);
  assert.ok(answers.hackAtlanticExcitement.split(/\s+/).length <= 100);
  assert.equal(Array.from({ length: 50 }, (_, i) => shouldUpload(currentIntakeSchema, i, "sustained")).filter(Boolean).length, 25);
  assert.equal(shouldUpload(currentIntakeSchema, 0, "deadline"), false);
});
