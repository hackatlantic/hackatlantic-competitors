import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Counter, Trend } from "k6/metrics";
import { SharedArray } from "k6/data";
import exec from "k6/execution";
import { assertStagingTarget, tokenAt, applicantScenario, applicantAnswers, shouldUpload, fixedResume } from "./profile-contract.mjs";

const profile = __ENV.K6_APPLICANT_PROFILE ?? "sustained";
const applicants = new SharedArray("synthetic applicants", () => {
  if (!__ENV.K6_APPLICANT_FIXTURES) throw new Error("K6_APPLICANT_FIXTURES is required");
  return JSON.parse(open(__ENV.K6_APPLICANT_FIXTURES)).applicants;
});
const vus = Number(__ENV.K6_APPLICANT_VUS ?? (profile === "sustained" ? 20 : applicants.length));
const baseURL = __ENV.API_BASE_URL;
assertStagingTarget(baseURL);
const resume = fixedResume();
const journeys = new Rate("applicant_journeys_successful");
const completed = new Counter("applicant_journeys_completed");
const lostAnswers = new Counter("applicant_answer_mismatches");
const failures = new Counter("applicant_operation_failures");
// Wall-clock upload latency includes sending the fixed-size request body.
const uploadDuration = new Trend("applicant_resume_upload_ms", true);

export const options = {
  scenarios: { ["applicant_" + profile]: applicantScenario(profile, vus, applicants.length, __ENV) },
  maxRedirects: 0,
  thresholds: {
    http_req_failed: ["rate<0.01"],
    applicant_journeys_successful: ["rate>=0.99"],
    applicant_journeys_completed: [`count>=${Math.ceil(applicants.length * 0.99)}`],
    applicant_answer_mismatches: ["count==0"],
    ...(profile === "deadline" ? { dropped_iterations: ["count==0"] } : {}),
    ...(profile !== "stress" ? {
      "http_req_duration{operation:submit}": ["p(95)<2000"],
      ...(profile === "sustained" ? {
        "http_req_duration{operation:form}": ["p(95)<1000"],
        "http_req_duration{operation:create}": ["p(95)<1000"],
        "http_req_duration{operation:draft}": ["p(95)<1000"],
        applicant_resume_upload_ms: ["p(95)<3000"],
      } : {}),
    } : {}),
    ...(profile === "stress" ? Object.fromEntries(["form", "create", "draft", "resume", "submit"].map((op) => [`http_req_duration{operation:${op}}`, []])) : {}),
  },
};

function request(applicant, operation, method, path, body = null) {
  const isUpload = operation === "resume";
  const start = Date.now();
  const response = http.request(method, baseURL + path, body, {
    headers: {
      Authorization: "Bearer " + tokenAt(applicant),
      "Content-Type": isUpload ? "application/pdf" : "application/json",
      ...(isUpload ? { "X-File-Name": "synthetic-fixed-512k.pdf" } : {}),
    },
    tags: { operation, name: operation },
    timeout: "15s", responseCallback: http.expectedStatuses(200),
  });
  if (isUpload) uploadDuration.add(Date.now() - start);
  if (!check(response, { [operation + " succeeded"]: (value) => value.status === 200 })) {
    failures.add(1, { operation, status: String(response.status) });
    console.warn(`operation=${operation} status=${response.status} error_code=${response.error_code ?? 0}`);
    return null;
  }
  try { return response.json(); } catch { failures.add(1, { operation, status: "invalid_json" }); return null; }
}

function think() { if (profile === "sustained") sleep(20 + Math.random() * 25); }
function answersMatch(actual, expected) {
  return actual && Object.keys(actual).length === Object.keys(expected).length && Object.entries(expected).every(([key, value]) => actual[key] === value);
}

function journey(applicant, index) {
  let draft;
  if (profile === "deadline") {
    draft = { id: applicant.applicationId, lockVersion: applicant.lockVersion };
  } else {
    if (profile === "sustained" && exec.vu.iterationInScenario === 0) sleep((exec.vu.idInTest - 1) * (60 / vus));
    const form = request(applicant, "form", "GET", "/v1/application-forms/current");
    if (!form) return false;
    think();
    draft = request(applicant, "create", "POST", "/v1/applications");
    if (!draft) return false;
    think();
    const saves = profile === "sustained" ? 2 : 1;
    for (let revision = 1; revision <= saves; revision++) {
      const expected = applicantAnswers(form, applicant.email, index, revision);
      draft = request(applicant, "draft", "PUT", `/v1/applications/${draft.id}/draft`, JSON.stringify({ lockVersion: draft.lockVersion, answers: expected }));
      if (!draft) return false;
      const correct = answersMatch(draft.answers, expected);
      lostAnswers.add(correct ? 0 : 1);
      if (!correct) return false;
      think();
    }
    if (shouldUpload(form, index, profile) && !request(applicant, "resume", "PUT", `/v1/applications/${draft.id}/resume`, resume)) return false;
    think();
  }
  const submitted = request(applicant, "submit", "POST", `/v1/applications/${draft.id}/submit`, JSON.stringify({ lockVersion: draft.lockVersion }));
  return submitted?.status === "submitted";
}

export default function applicantSubmission() {
  const index = exec.scenario.iterationInTest;
  const applicant = applicants[index];
  if (!applicant) exec.test.abort("Insufficient distinct applicant fixtures");
  let success = false;
  try { success = journey(applicant, index); }
  finally { journeys.add(success); if (success) completed.add(1); lostAnswers.add(0); }
}
