import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { STAGING_API } from "./profile-contract.mjs";

test("k6 can initialize all workload profiles without sending requests", { skip: !process.env.K6_BINARY }, () => {
  const directory = mkdtempSync(join(tmpdir(), "hackatlantic-k6-inspect-"));
  const path = join(directory, "synthetic.json");
  const environment = { ...process.env };
  delete environment.SSLKEYLOGFILE;
  writeFileSync(path, JSON.stringify({ runID: "100-1-1", applicants: Array.from({ length: 250 }, () => ({ token: "synthetic-not-a-real-token" })), scanner: { passes: Array.from({ length: 3500 }, () => ({ qrToken: "not-a-real-pass" })), tokens: ["synthetic-not-a-real-token"] } }));
  try {
    for (const [file, key, profiles] of [
      ["scanner.js", "K6_SCANNER_PROFILE", ["release", "repeatability", "spike", "contention"]],
      ["applicant.js", "K6_APPLICANT_PROFILE", ["sustained", "deadline", "stress"]],
    ]) {
      for (const profile of profiles) {
        const result = execFileSync(process.env.K6_BINARY, ["inspect", "-e", `API_BASE_URL=${STAGING_API}`, "-e", `${key}=${profile}`, "-e", `K6_SCANNER_FIXTURES=${path}`, "-e", `K6_APPLICANT_FIXTURES=${path}`, `tests/load/${file}`], { encoding: "utf8", env: environment });
        assert.ok(JSON.parse(result).scenarios);
      }
    }
  } finally { rmSync(directory, { recursive: true }); }
});
