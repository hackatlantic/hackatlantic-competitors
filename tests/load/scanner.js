import http from "k6/http";
import { check } from "k6";
import { SharedArray } from "k6/data";
import exec from "k6/execution";

const virtualUsers = Number(__ENV.K6_SCANNER_VUS ?? 25);
const iterations = Number(__ENV.K6_SCANNER_ITERATIONS ?? 0);
const fixturePath = __ENV.K6_SCANNER_FIXTURES;
const scannerPasses = new SharedArray("synthetic scanner passes", () => {
  if (!fixturePath) throw new Error("K6_SCANNER_FIXTURES is required");
  return JSON.parse(open(fixturePath)).scanner.passes;
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

export function setup() {
  if (__ENV.SCANNER_TOKEN) return { scannerToken: __ENV.SCANNER_TOKEN };
  if (!__ENV.CLERK_SECRET_KEY || !__ENV.SCANNER_USER_ID || !__ENV.CLERK_JWT_TEMPLATE) {
    exec.test.abort(
      "Set SCANNER_TOKEN, or CLERK_SECRET_KEY, SCANNER_USER_ID and CLERK_JWT_TEMPLATE",
    );
  }

  const clerkHeaders = {
    Authorization: `Bearer ${__ENV.CLERK_SECRET_KEY}`,
    "Content-Type": "application/json",
  };
  const sessions = http.get(
    `https://api.clerk.com/v1/sessions?user_id=${encodeURIComponent(__ENV.SCANNER_USER_ID)}&status=active&limit=1`,
    { headers: clerkHeaders, responseCallback: http.expectedStatuses(200) },
  );
  const sessionID = sessions.json("data.0.id");
  if (!sessionID) exec.test.abort("The synthetic scanner user has no active Clerk session");

  const token = http.post(
    `https://api.clerk.com/v1/sessions/${sessionID}/tokens/${encodeURIComponent(__ENV.CLERK_JWT_TEMPLATE)}`,
    null,
    { headers: clerkHeaders, responseCallback: http.expectedStatuses(200) },
  );
  const scannerToken = token.json("jwt");
  if (!scannerToken) exec.test.abort("Clerk did not issue the scanner load-test JWT");
  return { scannerToken };
}

function operationID() {
  const vu = String(exec.vu.idInTest).padStart(3, "0").slice(-3);
  const iteration = String(exec.scenario.iterationInTest).padStart(9, "0").slice(-9);
  return `00000000-0000-4000-8000-${vu}${iteration}`;
}

export default function scannerLoad(data) {
  if (!baseURL || !data.scannerToken || !checkpointID || scannerPasses.length < virtualUsers) {
    exec.test.abort("API_BASE_URL, scanner authentication, CHECKPOINT_ID, and one pass per virtual user are required");
  }
  const qrToken = scannerPasses[exec.vu.idInTest - 1].qrToken;
  const headers = { Authorization: `Bearer ${data.scannerToken}`, "Content-Type": "application/json" };
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
