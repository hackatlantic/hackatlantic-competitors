import { spawnSync } from "node:child_process";
import { pathToFileURL } from "node:url";

// Vercel CLI 58.1.0 reports this specific conflict when a release job is retried.
// Do not turn arbitrary 409s, auth failures, or additional errors into success.
export function alreadyCurrentProduction(output) {
  const errors = output.replace(/\u001b\[[0-9;]*m/g, "").split(/\r?\n/)
    .map((line) => line.trim()).filter((line) => line.startsWith("Error:"));
  return errors.length === 1 && /^Error: The provided deploymentId \(dpl_[A-Za-z0-9]+\) is already the current production deployment\. \(409\)$/.test(errors[0]);
}

export function promoteFrontend(env, run = spawnSync, log = console.log) {
  if (!env.FRONTEND_DEPLOYMENT_URL || !env.VERCEL_TOKEN) {
    throw new Error("FRONTEND_DEPLOYMENT_URL and VERCEL_TOKEN are required");
  }
  const url = new URL(env.FRONTEND_DEPLOYMENT_URL);
  if (url.protocol !== "https:" || !url.hostname.endsWith(".vercel.app") || url.username || url.password || url.search || url.hash || url.pathname !== "/") {
    throw new Error("Expected the immutable Vercel deployment URL");
  }
  const result = run("npx", ["--yes", "vercel@58.1.0", "promote", url.origin, "--yes", `--token=${env.VERCEL_TOKEN}`], {
    encoding: "utf8", env, timeout: 180_000,
  });
  const output = `${result.stdout ?? ""}\n${result.stderr ?? ""}`;
  // Avoid echoing credentials even if a child-process error includes arguments.
  log(output.replaceAll(env.VERCEL_TOKEN, "[REDACTED]"));
  if (result.error || result.signal || result.status === null) return 1;
  if (result.status === 0) return 0;
  if (alreadyCurrentProduction(output)) {
    log("The requested frontend is already current production; promotion is complete.");
    return 0;
  }
  return result.status || 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { process.exitCode = promoteFrontend(process.env); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
