import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { isDeepStrictEqual } from "node:util";
import { STAGING_API, assertStagingTarget } from "../tests/load/profile-contract.mjs";

export const FAULT_PATH = "/__staging_rollback_drill_missing_health__";
const statePath = ".tmp/staging-fault-private.json";
const reportPath = ".tmp/staging-fault-report.json";

export function faultySpec(spec) {
  if (spec.name !== "hackatlantic-api-staging") throw new Error("Refusing a non-staging application");
  const copy = structuredClone(spec);
  const api = copy.services?.find((service) => service.name === "api");
  if (!api?.image?.digest?.match(/^sha256:[a-f0-9]{64}$/) || api.health_check?.http_path !== "/readyz") throw new Error("Expected a digest-pinned staging API with the normal readiness probe");
  api.health_check.http_path = FAULT_PATH;
  return copy;
}

async function cloud(path, method = "GET", body) {
  const response = await fetch("https://api.digitalocean.com/v2/apps" + path, {
    method, headers: { Authorization: "Bearer " + process.env.DIGITALOCEAN_TOKEN, "Content-Type": "application/json" },
    ...(body ? { body: JSON.stringify(body) } : {}), redirect: "error", signal: AbortSignal.timeout(20000),
  });
  if (!response.ok) throw new Error(`DigitalOcean ${method} failed with HTTP ${response.status}; response body suppressed`);
  return response.json();
}
async function publicJSON(path) {
  const response = await fetch(STAGING_API + path, { redirect: "error", signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error("Staging endpoint is not healthy");
  return response.json();
}
const save = (path, value) => { mkdirSync(".tmp", { recursive: true }); writeFileSync(path, JSON.stringify(value, null, 2) + "\n", { mode: 0o600 }); };
const read = (path) => JSON.parse(readFileSync(path, "utf8"));
const sleep = () => new Promise((resolve) => setTimeout(resolve, 5000));
function apiService(spec) { return spec.services.find((service) => service.name === "api"); }

export async function runDrill(phase) {
  assertStagingTarget(process.env.API_BASE_URL);
  if (process.env.DRILL_CONFIRMATION !== "ROLLBACK STAGING" || !process.env.DIGITALOCEAN_TOKEN) throw new Error("Explicit staging confirmation and credential required");
  if (phase === "inject") {
    const apps = (await cloud("?per_page=200")).apps.filter((app) => app.spec.name === "hackatlantic-api-staging");
    if (apps.length !== 1) throw new Error("Expected exactly one staging application");
    const app = apps[0];
    if (app.in_progress_deployment || app.pending_deployment) throw new Error("A deployment is already in progress; refusing to interfere");
    const original = app.active_deployment;
    if (original?.phase !== "ACTIVE") throw new Error("Staging must have an active healthy deployment");
    if (!isDeepStrictEqual(app.spec, original.spec)) throw new Error("Desired and active app configuration differ; reconcile before a drill");
    const version = await publicJSON("/versionz");
    if ((await publicJSON("/readyz")).status !== "ready") throw new Error("Staging was not ready before the drill");
    const missing = await fetch(STAGING_API + FAULT_PATH, { redirect: "error", signal: AbortSignal.timeout(5000) });
    if (missing.status !== 404) throw new Error("The fault route must return 404 before injection");
    const spec = faultySpec(app.spec);
    if (apiService(original.spec).image.digest !== apiService(app.spec).image.digest) throw new Error("Active and desired digests differ before the drill");
    // This file is private and is NEVER an uploaded artifact. Save before mutation
    // so an ambiguous update response can still be followed by restoration.
    const state = { appID: app.id, originalID: original.id, originalSpec: app.spec, originalDigest: apiService(app.spec).image.digest, originalSHA: version.gitSha, injectedAt: new Date().toISOString() };
    save(statePath, state);
    save(reportPath, { appID: state.appID, originalDeploymentID: state.originalID, injectedAt: state.injectedAt, originalDigest: state.originalDigest, originalSHA: state.originalSHA, candidateHealthy: false, fault: "readiness path only; image and liveness unchanged" });
    await cloud("/" + app.id, "PUT", { spec });
    console.log("Submitted one staging readiness-probe fault; production was not targeted.");
    return;
  }
  const state = read(statePath);
  const app = (await cloud("/" + state.appID)).app;
  if (app.spec.name !== "hackatlantic-api-staging") throw new Error("Stored app ID is not staging");
  const report = read(reportPath);
  if (phase === "detect") {
    const limit = Date.now() + 12 * 60 * 1000;
    while (Date.now() < limit) {
      const deployments = (await cloud(`/${state.appID}/deployments?per_page=20`)).deployments;
      const candidate = deployments.find((deployment) => deployment.id !== state.originalID && apiService(deployment.spec)?.health_check?.http_path === FAULT_PATH && Date.parse(deployment.created_at) >= Date.parse(state.injectedAt) - 5000);
      if (candidate) {
        report.candidateID = candidate.id;
        report.candidatePhase = candidate.phase;
        if (candidate.phase === "ACTIVE") { report.candidateHealthy = true; save(reportPath, report); throw new Error("Faulty candidate unexpectedly became active"); }
        if (["ERROR", "CANCELED", "SUPERSEDED"].includes(candidate.phase)) {
          report.detectedAt = new Date().toISOString();
          report.detectionSeconds = (Date.parse(report.detectedAt) - Date.parse(state.injectedAt)) / 1000;
          // Prove health-check rejection, not an unrelated registry/build failure.
          report.healthCheckFailure = JSON.stringify(candidate.progress ?? {}).includes("ContainerHealthChecksFailed");
          save(reportPath, report);
          if (!report.healthCheckFailure) throw new Error("Candidate failed, but health-check failure was not established");
          console.log(`Detected staging health-check failure after ${report.detectionSeconds}s.`);
          return;
        }
      }
      await sleep();
    }
    throw new Error("Timed out waiting for candidate health-check rejection");
  }
  if (phase === "restore") {
    report.rollbackRequestedAt = new Date().toISOString();
    save(reportPath, report);
    // Cancel only our own pending candidate, if detection timed out. Never cancel
    // a different deployment. The original deployment is still the rollback target.
    let current = app.in_progress_deployment ?? app.pending_deployment;
    if (current && !current.spec) current = (await cloud(`/${state.appID}/deployments/${current.id}`)).deployment;
    if (current && apiService(current.spec)?.health_check?.http_path === FAULT_PATH) await cloud(`/${state.appID}/deployments/${current.id}/cancel`, "POST", {});
    try { await cloud(`/${state.appID}/rollback`, "POST", { deployment_id: state.originalID }); }
    catch {
      await cloud("/" + state.appID, "PUT", { spec: state.originalSpec });
      throw new Error("Rollback request failed; original spec resubmitted and operator verification required");
    }
    const limit = Date.now() + 5 * 60 * 1000;
    while (Date.now() < limit) {
      const restored = (await cloud("/" + state.appID)).app;
      const active = restored.active_deployment;
      if (active?.phase === "ACTIVE" && active.id !== state.originalID && apiService(active.spec)?.image?.digest === state.originalDigest && apiService(active.spec)?.health_check?.http_path === "/readyz" && apiService(restored.spec)?.health_check?.http_path === "/readyz") {
        try {
          if ((await publicJSON("/readyz")).status !== "ready" || (await publicJSON("/versionz")).gitSha !== state.originalSHA) throw new Error("Readiness or version differs");
          report.restoredAt = new Date().toISOString();
          report.recoverySeconds = (Date.parse(report.restoredAt) - Date.parse(report.rollbackRequestedAt)) / 1000;
          report.originalDigestRestored = true;
          report.readinessPassed = true;
          save(reportPath, report);
          console.log(JSON.stringify(report));
          return;
        } catch { /* Wait for ingress to converge; do not report readiness early. */ }
      }
      await sleep();
    }
    // Fallback reconciles the saved desired spec even when rollback is slow.
    // Keep the drill failed: a fallback is not proof of a five-minute recovery.
    await cloud("/" + state.appID, "PUT", { spec: state.originalSpec });
    throw new Error("Recovery exceeded five minutes; original spec resubmitted and operator verification required");
  }
  throw new Error("Expected inject, detect, or restore");
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  runDrill(process.argv[2]).catch((error) => { console.error(error.message); process.exitCode = 1; });
}
