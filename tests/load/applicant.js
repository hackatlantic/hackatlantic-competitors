import http from "k6/http";
import { check, sleep } from "k6";
import { SharedArray } from "k6/data";
import exec from "k6/execution";

const profile = __ENV.K6_APPLICANT_PROFILE ?? "sustained";
const fixturePath = __ENV.K6_APPLICANT_FIXTURES;
const applicants = new SharedArray("synthetic applicants", () => {
  if (!fixturePath) throw new Error("K6_APPLICANT_FIXTURES is required");
  return JSON.parse(open(fixturePath)).applicants;
});
const virtualUsers = Number(__ENV.K6_APPLICANT_VUS ?? (profile === "sustained" ? 20 : applicants.length));

function applicantScenario() {
  if (profile === "deadline") {
    return {
      executor: "shared-iterations",
      vus: Math.min(virtualUsers, applicants.length),
      iterations: applicants.length,
      maxDuration: __ENV.K6_APPLICANT_MAX_DURATION ?? "2m",
    };
  }
  if (profile === "stress") {
    return {
      executor: "per-vu-iterations",
      vus: virtualUsers,
      iterations: 1,
      maxDuration: __ENV.K6_APPLICANT_MAX_DURATION ?? "2m",
    };
  }
  if (profile !== "sustained") throw new Error("K6_APPLICANT_PROFILE must be sustained, deadline, or stress");
  return {
    executor: "shared-iterations",
    vus: virtualUsers,
    iterations: applicants.length,
    maxDuration: __ENV.K6_APPLICANT_MAX_DURATION ?? "10m",
  };
}

function applicantThresholds() {
  const common = { checks: ["rate>0.99"], http_req_failed: ["rate<0.01"] };
  if (profile === "deadline") {
    return { ...common, "http_req_duration{operation:submit}": ["p(95)<1500"] };
  }
  return {
    ...common,
    "http_req_duration{operation:form}": ["p(95)<750"],
    "http_req_duration{operation:create}": ["p(95)<1000"],
    "http_req_duration{operation:draft}": ["p(95)<1500"],
    "http_req_duration{operation:resume}": ["p(95)<2000"],
    "http_req_duration{operation:submit}": ["p(95)<1500"],
  };
}

export const options = {
  scenarios: { ["applicant_" + profile]: applicantScenario() },
  thresholds: applicantThresholds(),
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

function think() {
  if (profile === "sustained") sleep(20 + Math.random() * 25);
}

function submitPreparedApplicant(applicant, index) {
  sleep((index * 60) / applicants.length);
  const headers = { Authorization: "Bearer " + applicant.token, "Content-Type": "application/json" };
  const response = http.post(
    baseURL + "/v1/applications/" + applicant.applicationId + "/submit",
    JSON.stringify({ lockVersion: applicant.lockVersion }),
    { headers, tags: { operation: "submit" }, responseCallback: expected },
  );
  check(response, {
    "deadline submission succeeded": (value) => value.status === 200,
    "deadline application is submitted": (value) => value.json("status") === "submitted",
  });
}

export default function applicantSubmission() {
  if (!baseURL || applicants.length === 0) exec.test.abort("API_BASE_URL and applicant fixtures are required");
  const index = exec.scenario.iterationInTest;
  const applicant = applicants[index];
  if (!applicant) exec.test.abort("The profile scheduled more iterations than applicant fixtures");
  if (profile === "deadline") {
    submitPreparedApplicant(applicant, index);
    return;
  }
  const headers = { Authorization: "Bearer " + applicant.token, "Content-Type": "application/json" };

  const formResponse = http.get(baseURL + "/v1/application-forms/current", {
    headers, tags: { operation: "form" }, responseCallback: expected,
  });
  if (!accepted(formResponse, "form lookup")) return;
  const form = formResponse.json();
  think();

  const createResponse = http.post(baseURL + "/v1/applications", null, {
    headers, tags: { operation: "create" }, responseCallback: expected,
  });
  if (!accepted(createResponse, "application creation")) return;
  const application = createResponse.json();
  think();

  let draftResponse = http.put(
    baseURL + "/v1/applications/" + application.id + "/draft",
    JSON.stringify({ lockVersion: application.lockVersion, answers: answersFor(form, applicant.email, index) }),
    { headers, tags: { operation: "draft" }, responseCallback: expected },
  );
  if (!accepted(draftResponse, "draft save")) return;
  let draft = draftResponse.json();
  think();

  if (profile === "sustained") {
    draftResponse = http.put(
      baseURL + "/v1/applications/" + application.id + "/draft",
      JSON.stringify({ lockVersion: draft.lockVersion, answers: answersFor(form, applicant.email, index) }),
      { headers, tags: { operation: "draft" }, responseCallback: expected },
    );
    if (!accepted(draftResponse, "second draft save")) return;
    draft = draftResponse.json();
    think();
  }

  if (form.resumeRequired) {
    const resumeResponse = http.put(
      baseURL + "/v1/applications/" + application.id + "/resume",
      "%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
      {
        headers: { Authorization: "Bearer " + applicant.token, "Content-Type": "application/pdf", "X-File-Name": "synthetic-load-" + (index + 1) + ".pdf" },
        tags: { operation: "resume" }, responseCallback: expected,
      },
    );
    if (!accepted(resumeResponse, "resume upload")) return;
  }
  think();

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
