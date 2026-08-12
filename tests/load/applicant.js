import http from "k6/http";
import { check } from "k6";
import { SharedArray } from "k6/data";
import exec from "k6/execution";

const fixturePath = __ENV.K6_APPLICANT_FIXTURES;
const applicants = new SharedArray("synthetic applicants", () => {
  if (!fixturePath) throw new Error("K6_APPLICANT_FIXTURES is required");
  return JSON.parse(open(fixturePath)).applicants;
});
const virtualUsers = Number(__ENV.K6_APPLICANT_VUS ?? applicants.length);

export const options = {
  scenarios: {
    applicant_submission_burst: {
      executor: "per-vu-iterations",
      vus: virtualUsers,
      iterations: 1,
      maxDuration: __ENV.K6_APPLICANT_MAX_DURATION ?? "2m",
    },
  },
  thresholds: {
    checks: ["rate>0.99"],
    http_req_failed: ["rate<0.01"],
    "http_req_duration{operation:form}": ["p(95)<750"],
    "http_req_duration{operation:create}": ["p(95)<1000"],
    "http_req_duration{operation:draft}": ["p(95)<1500"],
    "http_req_duration{operation:resume}": ["p(95)<2000"],
    "http_req_duration{operation:submit}": ["p(95)<1500"],
  },
};

const baseURL = __ENV.API_BASE_URL;
const expected = http.expectedStatuses(200);

function answersFor(form, email, sequence) {
  return Object.fromEntries(form.questions.map((question) => {
    const description = question.key + " " + question.label;
    if (question.type === "boolean") return [question.key, true];
    if (question.type === "number") return [question.key, sequence + 1];
    if (/email/i.test(description)) return [question.key, email];
    if (/school/i.test(description)) return [question.key, "Synthetic Atlantic University"];
    return [question.key, "Synthetic load response " + (sequence + 1) + " for " + question.label];
  }));
}

function accepted(response, name) {
  return check(response, { [name + " succeeded"]: (value) => value.status === 200 });
}

export default function applicantSubmission() {
  if (!baseURL || applicants.length < virtualUsers) {
    exec.test.abort("API_BASE_URL and one fixture per virtual user are required");
  }
  const index = exec.vu.idInTest - 1;
  const applicant = applicants[index];
  const headers = { Authorization: "Bearer " + applicant.token, "Content-Type": "application/json" };

  const formResponse = http.get(baseURL + "/v1/application-forms/current", {
    headers, tags: { operation: "form" }, responseCallback: expected,
  });
  if (!accepted(formResponse, "form lookup")) return;
  const form = formResponse.json();

  const createResponse = http.post(baseURL + "/v1/applications", null, {
    headers, tags: { operation: "create" }, responseCallback: expected,
  });
  if (!accepted(createResponse, "application creation")) return;
  const application = createResponse.json();

  const draftResponse = http.put(
    baseURL + "/v1/applications/" + application.id + "/draft",
    JSON.stringify({ lockVersion: application.lockVersion, answers: answersFor(form, applicant.email, index) }),
    { headers, tags: { operation: "draft" }, responseCallback: expected },
  );
  if (!accepted(draftResponse, "draft save")) return;
  const draft = draftResponse.json();

  if (form.resumeRequired) {
    const resumeResponse = http.put(
      baseURL + "/v1/applications/" + application.id + "/resume",
      "%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
      {
        headers: {
          Authorization: "Bearer " + applicant.token,
          "Content-Type": "application/pdf",
          "X-File-Name": "synthetic-load-" + (index + 1) + ".pdf",
        },
        tags: { operation: "resume" },
        responseCallback: expected,
      },
    );
    if (!accepted(resumeResponse, "resume upload")) return;
  }

  const submitResponse = http.post(
    baseURL + "/v1/applications/" + application.id + "/submit",
    JSON.stringify({ lockVersion: draft.lockVersion }),
    { headers, tags: { operation: "submit" }, responseCallback: expected },
  );
  check(submitResponse, {
    "submission succeeded": (response) => response.status === 200,
    "application is submitted": (response) => response.json("status") === "submitted",
  });
}
