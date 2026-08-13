import http from "k6/http";
import { check } from "k6";
import { SharedArray } from "k6/data";
import exec from "k6/execution";

const virtualUsers = Number(__ENV.K6_SCANNER_VUS ?? 25);
const iterations = Number(__ENV.K6_SCANNER_ITERATIONS ?? 0);
const fixturePath = __ENV.K6_SCANNER_FIXTURES;
const scannerFixtures = new SharedArray("synthetic scanner fixtures", () => {
  if (!fixturePath) throw new Error("K6_SCANNER_FIXTURES is required");
  const scanner = JSON.parse(open(fixturePath)).scanner;
  return scanner.passes.map((pass, index) => ({
    ...pass,
    token: scanner.tokens[index % scanner.tokens.length],
  }));
});

export const options = {
  scenarios: {
    scanner_lookup: iterations > 0
      ? {
          executor: "per-vu-iterations",
          vus: virtualUsers,
          iterations,
          maxDuration: __ENV.K6_SCANNER_MAX_DURATION ?? "2m",
        }
      : {
          executor: "constant-vus",
          vus: virtualUsers,
          duration: __ENV.K6_SCANNER_DURATION ?? "5m",
        },
  },
  thresholds: {
    "http_req_duration{operation:lookup}": ["p(95)<500"],
    "http_req_duration{operation:redemption}": ["p(95)<750"],
    http_req_failed: ["rate<0.01"],
  },
};

const baseURL = __ENV.API_BASE_URL;
const checkpointID = __ENV.CHECKPOINT_ID;
const acceptedDomainStatuses = http.expectedStatuses(200, 409, 422);

function operationID() {
  const vu = String(exec.vu.idInTest).padStart(3, "0").slice(-3);
  const iteration = String(exec.scenario.iterationInTest).padStart(9, "0").slice(-9);
  return `00000000-0000-4000-8000-${vu}${iteration}`;
}

export default function scannerLoad() {
  if (!baseURL || !checkpointID || scannerFixtures.length < virtualUsers) {
    exec.test.abort("API_BASE_URL, CHECKPOINT_ID, and one pass per virtual user are required");
  }
  const scanner = scannerFixtures[exec.vu.idInTest - 1];
  const qrToken = scanner.qrToken;
  const headers = { Authorization: `Bearer ${scanner.token}`, "Content-Type": "application/json" };
  const lookup = http.post(`${baseURL}/v1/scans/lookup`, JSON.stringify({ qrToken }), {
    headers,
    tags: { operation: "lookup" },
    responseCallback: http.expectedStatuses(200),
  });
  check(lookup, { "lookup is accepted": (response) => response.status === 200 });

  const redemption = http.post(
    `${baseURL}/v1/redemptions`,
    JSON.stringify({ qrToken, checkpointId: checkpointID, idempotencyKey: operationID() }),
    {
      headers,
      tags: { operation: "redemption" },
      responseCallback: acceptedDomainStatuses,
    },
  );
  check(redemption, {
    "redemption has a domain outcome": (response) => [200, 409, 422].includes(response.status),
  });
}
