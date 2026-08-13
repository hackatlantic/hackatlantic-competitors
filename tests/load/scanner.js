import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate } from "k6/metrics";
import { SharedArray } from "k6/data";
import exec from "k6/execution";

const profile = __ENV.K6_SCANNER_PROFILE ?? "release";
const fixturePath = __ENV.K6_SCANNER_FIXTURES;
const scannerFixture = new SharedArray("synthetic scanner fixture", () => {
  if (!fixturePath) throw new Error("K6_SCANNER_FIXTURES is required");
  const scanner = JSON.parse(open(fixturePath)).scanner;
  return [{ passes: scanner.passes, tokens: scanner.tokens }];
})[0];
const virtualUsers = Number(__ENV.K6_SCANNER_VUS ?? (profile === "release" ? 20 : 100));
const iterations = Number(__ENV.K6_SCANNER_ITERATIONS ?? scannerFixture.passes.length);

function scannerScenario() {
  switch (profile) {
    case "release":
      return {
        executor: "shared-iterations",
        vus: virtualUsers,
        iterations,
        maxDuration: __ENV.K6_SCANNER_MAX_DURATION ?? "10m",
      };
    case "spike":
      return {
        executor: "constant-arrival-rate",
        rate: Number(__ENV.K6_SCANNER_RATE ?? 5),
        timeUnit: "1s",
        duration: __ENV.K6_SCANNER_DURATION ?? "20s",
        preAllocatedVUs: Math.min(virtualUsers, 20),
        maxVUs: virtualUsers,
      };
    case "contention":
      return {
        executor: "shared-iterations",
        vus: virtualUsers,
        iterations,
        maxDuration: __ENV.K6_SCANNER_MAX_DURATION ?? "2m",
      };
    default:
      throw new Error("K6_SCANNER_PROFILE must be release, spike, or contention");
  }
}

function scannerThresholds() {
  const common = {
    scanner_system_failures: ["rate<0.01"],
    http_req_failed: ["rate<0.01"],
  };
  if (profile === "release") {
    return {
      ...common,
      checks: ["rate>0.99"],
      scanner_domain_failures: ["rate==0"],
      "http_req_duration{operation:lookup}": ["p(95)<750"],
      "http_req_duration{operation:redemption}": ["p(95)<1000"],
    };
  }
  if (profile === "contention") {
    return {
      ...common,
      scanner_idempotency_mismatches: ["rate==0"],
      scanner_redeemed: ["count==1"],
      scanner_already_exhausted: [`count==${iterations - 1}`],
    };
  }
  return { ...common, dropped_iterations: ["count==0"] };
}

export const options = {
  scenarios: { ["scanner_" + profile]: scannerScenario() },
  thresholds: scannerThresholds(),
};

const systemFailures = new Rate("scanner_system_failures");
const domainFailures = new Rate("scanner_domain_failures");
const idempotencyMismatches = new Rate("scanner_idempotency_mismatches");
const redeemed = new Counter("scanner_redeemed");
const alreadyExhausted = new Counter("scanner_already_exhausted");
const unauthorized = new Counter("scanner_http_401");
const forbidden = new Counter("scanner_http_403");
const rateLimited = new Counter("scanner_http_429");
const serverErrors = new Counter("scanner_http_5xx");
const otherHTTPFailures = new Counter("scanner_http_other_failures");
const baseURL = __ENV.API_BASE_URL;
const checkpointID = __ENV.CHECKPOINT_ID;
const expectedHTTP = http.expectedStatuses(200);

function operationID() {
  const vu = String(exec.vu.idInTest).padStart(3, "0").slice(-3);
  const iteration = String(exec.scenario.iterationInTest).padStart(9, "0").slice(-9);
  return `00000000-0000-4000-8000-${vu}${iteration}`;
}

function recordSystemResult(response) {
  const failed = response.status !== 200;
  systemFailures.add(failed);
  if (failed) {
    if (response.status === 401) unauthorized.add(1);
    else if (response.status === 403) forbidden.add(1);
    else if (response.status === 429) rateLimited.add(1);
    else if (response.status >= 500) serverErrors.add(1);
    else otherHTTPFailures.add(1);
  }
  return !failed;
}

function releasePacing() {
  if (profile === "release") sleep(2 + Math.random() * 3);
}

function selectedPass() {
  if (profile === "contention") return scannerFixture.passes[0];
  return scannerFixture.passes[exec.scenario.iterationInTest];
}

export default function scannerLoad() {
  if (!baseURL || !checkpointID || scannerFixture.tokens.length === 0) {
    exec.test.abort("API_BASE_URL, CHECKPOINT_ID, and scanner tokens are required");
  }
  const pass = selectedPass();
  if (!pass) exec.test.abort("The profile scheduled more scans than distinct fixture passes");
  const token = scannerFixture.tokens[(exec.vu.idInTest - 1) % scannerFixture.tokens.length];
  const headers = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
  const lookup = http.post(`${baseURL}/v1/scans/lookup`, JSON.stringify({ qrToken: pass.qrToken }), {
    headers,
    tags: { operation: "lookup" },
    responseCallback: expectedHTTP,
  });
  const lookupOK = recordSystemResult(lookup);
  check(lookup, { "lookup is HTTP 200": () => lookupOK });
  if (!lookupOK) {
    releasePacing();
    return;
  }

  const idempotencyKey = operationID();
  const requestBody = JSON.stringify({ qrToken: pass.qrToken, checkpointId: checkpointID, idempotencyKey });
  const redemption = http.post(`${baseURL}/v1/redemptions`, requestBody, {
    headers,
    tags: { operation: "redemption" },
    responseCallback: expectedHTTP,
  });
  const redemptionOK = recordSystemResult(redemption);
  const outcome = redemptionOK ? redemption.json("outcome") : null;
  if (profile === "contention") {
    if (outcome === "redeemed") redeemed.add(1);
    if (outcome === "already_exhausted") alreadyExhausted.add(1);
    check(redemption, {
      "contention returns an allowed domain outcome": () => redemptionOK && ["redeemed", "already_exhausted"].includes(outcome),
    });
    const replay = http.post(`${baseURL}/v1/redemptions`, requestBody, {
      headers,
      tags: { operation: "idempotency_replay" },
      responseCallback: expectedHTTP,
    });
    const replayOK = recordSystemResult(replay);
    const mismatch = !replayOK || replay.json("outcome") !== outcome || replay.json("redemptionId") !== redemption.json("redemptionId");
    idempotencyMismatches.add(mismatch);
    check(replay, { "idempotency replay is identical": () => !mismatch });
  } else {
    const domainFailed = !redemptionOK || outcome !== "redeemed";
    domainFailures.add(domainFailed);
    check(redemption, { "distinct pass is redeemed": () => !domainFailed });
  }

  releasePacing();
}
