import assert from "node:assert/strict";
import test from "node:test";
import { jsonbInput } from "./sql-json-input.mjs";
import { currentIntakeSchema } from "./current-intake-form.mjs";
import { applicantAnswers } from "./profile-contract.mjs";

test("250 current-form expectations exceed one Linux argv but round-trip through SQL stdin", () => {
  const expected = Array.from({ length: 250 }, (_, index) => ({
    user_id: `hat_load_33724749486_1_applicant_${index}`,
    answers: applicantAnswers(currentIntakeSchema, `hat_load_33724749486_1_applicant_${index}@loadtest.invalid`, index, 1),
    upload: false,
  }));
  assert.ok(Buffer.byteLength(JSON.stringify(expected)) > 128 * 1024);
  const sql = jsonbInput(expected);
  const match = sql.match(/^convert_from\(decode\('([A-Za-z0-9+/=]+)', 'base64'\), 'UTF8'\)::jsonb$/);
  assert.ok(match);
  assert.deepEqual(JSON.parse(Buffer.from(match[1], "base64").toString("utf8")), expected);
});

test("SQL input handles quotes, newlines, backslashes, and Unicode without interpolation", () => {
  const value = { text: "O'Neil\\test\nRésumé'); SELECT 1; --" };
  const sql = jsonbInput(value);
  const encoded = sql.match(/decode\('([^']+)'/)[1];
  assert.deepEqual(JSON.parse(Buffer.from(encoded, "base64").toString("utf8")), value);
  assert.ok(!sql.includes("SELECT 1"));
});
