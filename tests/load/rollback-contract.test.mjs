import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { faultySpec, FAULT_PATH } from "../../scripts/staging-fault-drill.mjs";

const spec = { name: "hackatlantic-api-staging", region: "tor", services: [{ name: "api", image: { digest: "sha256:" + "a".repeat(64) }, health_check: { http_path: "/readyz", period_seconds: 10 }, liveness_health_check: { http_path: "/healthz" }, envs: [{ key: "SECRET", value: "fake" }] }] };
test("fault injection changes only the staging readiness path", () => {
  const fault = faultySpec(spec);
  assert.equal(fault.services[0].health_check.http_path, FAULT_PATH);
  fault.services[0].health_check.http_path = "/readyz";
  assert.deepEqual(fault, spec);
  assert.equal(spec.services[0].health_check.http_path, "/readyz");
});
test("fault injection rejects production and nonstandard deployment state", () => {
  assert.throws(() => faultySpec({ ...spec, name: "hackatlantic-api" }));
  assert.throws(() => faultySpec({ ...spec, services: [] }));
});
test("real production release depends on successful staging, without always override", () => {
  const release = readFileSync(new URL("../../.github/workflows/release.yml", import.meta.url), "utf8");
  const production = release.split("\n  production:")[1].split(/\n  [a-z-]+:/)[0];
  assert.match(production, /needs: \[image, frontend-stage, staging\]/);
  assert.doesNotMatch(production, /\n    if:.*always\(/);
});
