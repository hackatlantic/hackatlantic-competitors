import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { faultySpec, FAULT_PATH, runDrill } from "../../scripts/staging-fault-drill.mjs";
import { STAGING_API } from "./profile-contract.mjs";

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

test("drill rejects a health-check failure and restores the exact original digest with mocked cloud requests", async () => {
  const directory = mkdtempSync(join(tmpdir(), "hackatlantic-drill-contract-"));
  const previousDirectory = process.cwd();
  const originalFetch = globalThis.fetch;
  const environment = { API_BASE_URL: process.env.API_BASE_URL, DRILL_CONFIRMATION: process.env.DRILL_CONFIRMATION, DIGITALOCEAN_TOKEN: process.env.DIGITALOCEAN_TOKEN };
  const requests = [];
  let injected = false, restored = false, pinned = true;
  const app = () => ({ id: "staging-only", ...(pinned ? { pinned_deployment: { id: "existing-pin" } } : {}), spec: injected && !restored ? faultySpec(spec) : spec, active_deployment: { id: restored ? "restored" : "original", phase: "ACTIVE", spec } });
  globalThis.fetch = async (url, options = {}) => {
    const method = options.method ?? "GET";
    requests.push({ method, url });
    let response;
    if (url === STAGING_API + FAULT_PATH) return new Response("missing", { status: 404 });
    if (url === STAGING_API + "/readyz") response = { status: "ready" };
    else if (url === STAGING_API + "/versionz") response = { gitSha: "known-test-sha" };
    else if (url.endsWith("/v2/apps?per_page=200")) response = { apps: [app()] };
    else if (method === "PUT") { assert.deepEqual(JSON.parse(options.body).spec, faultySpec(spec)); injected = true; response = { app: app() }; }
    else if (url.endsWith("/rollback")) { assert.deepEqual(JSON.parse(options.body), { deployment_id: "original", skip_pin: true }); restored = true; response = { deployment: { id: "restored" } }; }
    else if (url.endsWith("/deployments?per_page=20")) response = { deployments: [{ id: "candidate", phase: "ERROR", spec: faultySpec(spec), created_at: new Date().toISOString(), progress: { steps: [{ reason: { code: "ContainerHealthChecksFailed" } }] } }] };
    else if (url.endsWith("/v2/apps/staging-only")) response = { app: app() };
    else throw new Error("Unexpected cloud request in drill contract");
    return new Response(JSON.stringify(response), { status: 200, headers: { "Content-Type": "application/json" } });
  };
  try {
    process.chdir(directory);
    Object.assign(process.env, { API_BASE_URL: STAGING_API, DRILL_CONFIRMATION: "ROLLBACK STAGING", DIGITALOCEAN_TOKEN: "fake-unit-test-token" });
    await assert.rejects(runDrill("inject"), /already pinned/);
    assert.equal(requests.filter((request) => request.method !== "GET").length, 0);
    pinned = false;
    await runDrill("inject"); await runDrill("detect"); await runDrill("restore");
    const report = JSON.parse(readFileSync(".tmp/staging-fault-report.json", "utf8"));
    assert.equal(report.healthCheckFailure, true); assert.equal(report.originalDigestRestored, true);
    assert.equal(report.candidateHealthy, false); assert.equal(report.readinessPassed, true);
    assert.equal(report.deploymentUnpinned, true);
    assert.ok(report.recoverySeconds < 300);
    assert.equal(requests.filter((request) => request.method === "PUT").length, 1);
    assert.equal(JSON.stringify(report).includes("fake-unit-test-token"), false);
  } finally {
    globalThis.fetch = originalFetch;
    for (const [key, value] of Object.entries(environment)) { if (value === undefined) delete process.env[key]; else process.env[key] = value; }
    process.chdir(previousDirectory);
    rmSync(directory, { recursive: true });
  }
});
