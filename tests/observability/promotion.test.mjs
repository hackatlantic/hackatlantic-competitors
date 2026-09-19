import assert from "node:assert/strict";
import test from "node:test";
import { alreadyCurrentProduction, promoteFrontend } from "../../scripts/promote-frontend.mjs";

const conflict = "Error: The provided deploymentId (dpl_Example123) is already the current production deployment. (409)";
const env = { FRONTEND_DEPLOYMENT_URL: "https://example-immutable.vercel.app", VERCEL_TOKEN: "test-only-secret" };

test("recognizes only the exact already-current production response", () => {
  assert.equal(alreadyCurrentProduction(conflict), true);
  for (const output of ["Error: Conflict (409)", "Error: Unauthorized (401)", "Error: Deployment not ready", `${conflict}\nError: Permission denied`, "already the current production deployment"]) {
    assert.equal(alreadyCurrentProduction(output), false);
  }
});

test("ordinary promotion and an already-current retry succeed", () => {
  for (const result of [{ status: 0 }, { status: 1, stderr: conflict }]) {
    assert.equal(promoteFrontend(env, () => result, () => {}), 0);
  }
});

test("authentication errors, conflicts, and interrupted commands still fail", () => {
  for (const result of [{ status: 1, stderr: "Error: Forbidden (403)" }, { status: 1, stderr: "Error: Conflict (409)" }, { status: null, signal: "SIGTERM", stderr: conflict }, { status: 1, error: new Error("timeout"), stderr: conflict }]) {
    assert.equal(promoteFrontend(env, () => result, () => {}), 1);
  }
});

test("passes a pinned CLI and redacts credentials from child output", () => {
  const logs = [];
  const run = (command, args, options) => {
    assert.equal(command, "npx");
    assert.ok(args.includes("vercel@58.1.0"));
    assert.ok(args.includes(env.FRONTEND_DEPLOYMENT_URL));
    assert.equal(options.timeout, 180_000);
    return { status: 1, stderr: `Error: ${env.VERCEL_TOKEN}` };
  };
  assert.equal(promoteFrontend(env, run, (line) => logs.push(line)), 1);
  assert.ok(!logs.join("").includes(env.VERCEL_TOKEN));
});

test("rejects missing credentials and non-deployment targets", () => {
  const run = () => { throw new Error("must not execute"); };
  assert.throws(() => promoteFrontend({}, run), /required/);
  for (const url of ["http://example.vercel.app", "https://example.com", "https://example.vercel.app?x=1", "https://user:password@example.vercel.app", "https://example.vercel.app/path"]) {
    assert.throws(() => promoteFrontend({ ...env, FRONTEND_DEPLOYMENT_URL: url }, run), /immutable/);
  }
});
