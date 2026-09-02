import { readFileSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { STAGING_API, assertStagingTarget, fixedResume, RESUME_BYTES } from "./profile-contract.mjs";

assertStagingTarget(process.env.API_BASE_URL);
async function readJSON(url, headers = {}) {
  const response = await fetch(url, { headers, redirect: "error", signal: AbortSignal.timeout(15000) });
  if (!response.ok) throw new Error(`Benchmark context lookup failed with HTTP ${response.status}`);
  return response.json();
}
const version = await readJSON(STAGING_API + "/versionz");
const apps = await readJSON("https://api.digitalocean.com/v2/apps?per_page=200", { Authorization: "Bearer " + process.env.DIGITALOCEAN_TOKEN });
const matches = apps.apps.filter((app) => app.spec.name === "hackatlantic-api-staging");
if (matches.length !== 1) throw new Error("Expected exactly one staging application");
const app = matches[0];
const service = app.active_deployment.spec.services.find((service) => service.name === "api");
const deployment = { gitSha: version.gitSha, digest: service.image.digest, instanceSize: service.instance_size_slug, instanceCount: service.instance_count, region: app.region?.slug ?? app.spec.region };
const path = ".tmp/benchmark-context.json";
if (process.argv[2] === "start") {
  writeFileSync(path, JSON.stringify({
    startedAt: new Date().toISOString(), deployment, testCommit: process.env.GITHUB_SHA,
    runID: process.env.GITHUB_RUN_ID, repetition: process.env.K6_REPETITION,
    runner: "GitHub-hosted ubuntu-24.04; physical region not controlled",
    profile: process.env.K6_SCANNER_PROFILE ?? process.env.K6_APPLICANT_PROFILE,
    resumeBytes: RESUME_BYTES, resumeSHA256: createHash("sha256").update(fixedResume()).digest("hex"),
  }, null, 2) + "\n");
} else if (process.argv[2] === "finish") {
  const context = JSON.parse(readFileSync(path, "utf8"));
  const summary = JSON.parse(readFileSync(process.env.K6_SUMMARY_PATH, "utf8"));
  context.finishedAt = new Date().toISOString();
  context.deploymentUnchanged = JSON.stringify(context.deployment) === JSON.stringify(deployment);
  const seconds = (Date.parse(context.finishedAt) - Date.parse(context.startedAt)) / 1000;
  context.elapsedSeconds = seconds;
  context.completedScansPerMinute = summary.metrics.scanner_completed ? summary.metrics.scanner_completed.count / (seconds / 60) : null;
  writeFileSync(path, JSON.stringify(context, null, 2) + "\n");
  console.log(JSON.stringify(context));
  if (!context.deploymentUnchanged) throw new Error("Deployment changed during benchmark; results cannot be compared");
} else throw new Error("Expected start or finish");
