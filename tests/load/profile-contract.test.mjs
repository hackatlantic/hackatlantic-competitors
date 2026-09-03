import assert from "node:assert/strict";
import test from "node:test";
import { createHash } from "node:crypto";
import { STAGING_API, RESUME_BYTES, assertStagingTarget, tokenAt, scannerPause, scannerScenario, applicantScenario, isDeadlineBoundary, applicantAnswers, shouldUpload, fixedResume } from "./profile-contract.mjs";

test("load target rejects production, lookalike hosts, redirects and paths", () => {
  assert.doesNotThrow(() => assertStagingTarget(STAGING_API));
  for (const target of ["https://api.hackatlantic.ca", STAGING_API + ".evil.test", STAGING_API + "/", "http://localhost:8080"]) assert.throws(() => assertStagingTarget(target));
});
test("repeatability is ten minutes, not a fixed number of iterations", () => {
  assert.deepEqual(scannerScenario("repeatability", 20, 3500), { executor: "constant-vus", vus: 20, duration: "10m", gracefulStop: "30s" });
  for (let vu = 1; vu <= 20; vu++) {
    let seconds = 0, scans = 0;
    while (seconds < 600) { const pause = scannerPause(vu, scans++); assert.ok(pause >= 2 && pause <= 5); seconds += pause; }
    assert.ok(scans * 20 < 3500, "fixture capacity exceeds even zero-latency scans");
  }
});
test("deadline arrivals span ten minutes with enough distinct applicants", () => {
  const scenario = applicantScenario("deadline", 25, 250);
  assert.equal(scenario.executor, "constant-arrival-rate");
  assert.equal(scenario.rate, 25); assert.equal(scenario.timeUnit, "1m"); assert.equal(scenario.duration, "10m");
  assert.throws(() => applicantScenario("deadline", 25, 25));
});
test("token rotation never selects future or nearly expired tokens", () => {
  const identity = { tokenWindows: [{ iat: 100, exp: 700, token: "first" }, { iat: 640, exp: 1240, token: "second" }] };
  assert.equal(tokenAt(identity, 639), "first"); assert.equal(tokenAt(identity, 640), "second");
  assert.throws(() => tokenAt(identity, 99)); assert.throws(() => tokenAt(identity, 1210));
});

test("deadline skips only the extra end-boundary callback, never missing work", () => {
  assert.equal(isDeadlineBoundary("deadline", 250, 250, 600000), true);
  assert.equal(isDeadlineBoundary("deadline", 250, 250, 599999), true);
  for (const args of [
    ["deadline", 249, 250, 600000], ["deadline", 251, 250, 600000],
    ["deadline", 250, 249, 600000], ["deadline", 250, 250, 590000],
    ["sustained", 250, 250, 600000],
  ]) assert.equal(isDeadlineBoundary(...args), false);
});
test("optional uploads cannot silently skip the resume workload", () => {
  assert.equal(Array.from({ length: 50 }, (_, i) => shouldUpload({ resumeRequired: false }, i, "sustained")).filter(Boolean).length, 25);
  const pdf = fixedResume(); assert.equal(Buffer.byteLength(pdf), RESUME_BYTES);
  assert.ok(pdf.startsWith("%PDF-1.4")); assert.ok(pdf.endsWith("%%EOF\n"));
  assert.equal(createHash("sha256").update(pdf).digest("hex"), createHash("sha256").update(fixedResume()).digest("hex"));
});
test("second draft changes answers and preserves conditional hardware fields", () => {
  const form = { questions: [{ key: "name", type: "string", label: "Name" }, { key: "hardware", type: "boolean", label: "Hardware" }, { key: "equipment", type: "string", label: "Equipment", showWhen: { key: "hardware", equals: true } }] };
  assert.notDeepEqual(applicantAnswers(form, "test@loadtest.invalid", 0, 1), applicantAnswers(form, "test@loadtest.invalid", 0, 2));
  assert.ok(applicantAnswers(form, "test@loadtest.invalid", 0).equipment);
});
