import { execFileSync } from "node:child_process";
import { createHmac } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";

const command = process.argv[2];
const fixturePath = process.env.K6_FIXTURE_PATH ?? ".tmp/k6-staging-fixture.json";
const apiBaseURL = process.env.API_BASE_URL;
const databaseURL = process.env.DATABASE_URL;
const loadTestAuthSecret = process.env.LOAD_TEST_AUTH_SECRET;
const applicantCount = Number(process.env.K6_APPLICANT_VUS ?? 100);

function requireConfiguration() {
  if (!apiBaseURL || !databaseURL || !loadTestAuthSecret) {
    throw new Error("API_BASE_URL, DATABASE_URL, and LOAD_TEST_AUTH_SECRET are required");
  }
  const decodedSecret = Buffer.from(loadTestAuthSecret, "base64");
  if (decodedSecret.length < 32 || decodedSecret.toString("base64") !== loadTestAuthSecret) {
    throw new Error("LOAD_TEST_AUTH_SECRET must be standard base64 containing at least 32 bytes");
  }
  if (!Number.isInteger(applicantCount) || applicantCount < 1 || applicantCount > 200) {
    throw new Error("K6_APPLICANT_VUS must be between 1 and 200");
  }
}

async function api(path, token, options = {}) {
  const response = await fetch(apiBaseURL + path, {
    ...options,
    headers: {
      Authorization: "Bearer " + token,
      "Content-Type": "application/json",
      ...options.headers,
    },
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error("API " + path + " failed with " + response.status + ": " + (body.code ?? "unknown"));
  }
  return body;
}

function databaseEnvironment() {
  const parsed = new URL(databaseURL);
  return {
    ...process.env,
    PGHOST: parsed.hostname,
    PGPORT: parsed.port || "5432",
    PGUSER: decodeURIComponent(parsed.username),
    PGPASSWORD: decodeURIComponent(parsed.password),
    PGDATABASE: parsed.pathname.slice(1),
    PGSSLMODE: parsed.searchParams.get("sslmode") ?? "require",
  };
}

function psql(sql, variables = {}) {
  const args = ["-v", "ON_ERROR_STOP=1", "-q"];
  for (const [name, value] of Object.entries(variables)) args.push("-v", name + "=" + value);
  execFileSync("psql", args, {
    env: databaseEnvironment(),
    input: sql + "\n",
    stdio: ["pipe", "ignore", "pipe"],
  });
}

function loadFixture() {
  return JSON.parse(readFileSync(fixturePath, "utf8"));
}

function saveFixture(fixture) {
  writeFileSync(fixturePath, JSON.stringify(fixture) + "\n", { mode: 0o600 });
}

function ensureOpenApplicationForm(adminClerkUserID, runID) {
  psql(`
INSERT INTO ats.application_cycles (
  slug,
  name,
  applications_open_at,
  applications_close_at,
  active
)
SELECT
  'load-test-' || :'run_id',
  'Synthetic load-test cycle',
  CURRENT_TIMESTAMP - INTERVAL '1 day',
  CURRENT_TIMESTAMP + INTERVAL '30 days',
  true
WHERE NOT EXISTS (SELECT 1 FROM ats.application_cycles WHERE active);

UPDATE ats.application_cycles
SET applications_open_at = CURRENT_TIMESTAMP - INTERVAL '1 day',
    applications_close_at = CURRENT_TIMESTAMP + INTERVAL '30 days',
    updated_at = CURRENT_TIMESTAMP
WHERE active
  AND NOT (applications_open_at <= CURRENT_TIMESTAMP AND CURRENT_TIMESTAMP < applications_close_at);

INSERT INTO ats.application_forms (
  cycle_id,
  version,
  schema_json,
  published_at,
  created_by
)
SELECT
  cycle.id,
  COALESCE((SELECT MAX(existing.version) + 1 FROM ats.application_forms AS existing WHERE existing.cycle_id = cycle.id), 1),
  '{"resumeRequired":true,"questions":[{"key":"full_name","label":"Full name","type":"string","required":true},{"key":"email","label":"Email","type":"string","required":true},{"key":"school","label":"School","type":"string","required":true}]}'::jsonb,
  CURRENT_TIMESTAMP,
  creator.id
FROM ats.application_cycles AS cycle
JOIN ats.users AS creator ON creator.clerk_user_id = :'admin_clerk_user_id'
WHERE cycle.active
  AND NOT EXISTS (
    SELECT 1
    FROM ats.application_forms AS published
    WHERE published.cycle_id = cycle.id
      AND published.published_at IS NOT NULL
  );
`, { admin_clerk_user_id: adminClerkUserID, run_id: runID });
}

function createIdentity(runID, role, sequence = 0) {
  const normalizedRunID = runID.replace(/[^a-z0-9]/gi, "_").toLowerCase();
  const suffix = sequence > 0 ? "_" + sequence : "";
  const userId = "hat_load_" + normalizedRunID + "_" + role + suffix;
  return { userId, email: userId + "@loadtest.invalid" };
}

async function sessionTokens(identities) {
  const now = Math.floor(Date.now() / 1_000);
  const secret = Buffer.from(loadTestAuthSecret, "base64");
  return identities.map((identity) => {
    const payload = Buffer.from(JSON.stringify({ sub: identity.userId, iat: now - 5, exp: now + 540 }));
    const signature = createHmac("sha256", secret).update(payload).digest();
    return "hat_load_v1." + payload.toString("base64url") + "." + signature.toString("base64url");
  });
}

function answersFor(form, email) {
  return Object.fromEntries(form.questions.map((question) => {
    const description = question.key + " " + question.label;
    if (question.type === "boolean") return [question.key, true];
    if (question.type === "number") return [question.key, 1];
    if (/email/i.test(description)) return [question.key, email];
    if (/school/i.test(description)) return [question.key, "Synthetic Atlantic University"];
    return [question.key, "Synthetic fixture response for " + question.label];
  }));
}

async function createSubmittedApplication(identity, token) {
  const form = await api("/v1/application-forms/current", token);
  const application = await api("/v1/applications", token, { method: "POST" });
  const draft = await api("/v1/applications/" + application.id + "/draft", token, {
    method: "PUT",
    body: JSON.stringify({ lockVersion: application.lockVersion, answers: answersFor(form, identity.email) }),
  });
  if (form.resumeRequired) {
    await api("/v1/applications/" + application.id + "/resume", token, {
      method: "PUT",
      body: "%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
      headers: { "Content-Type": "application/pdf", "X-File-Name": "synthetic-fixture.pdf" },
    });
  }
  await api("/v1/applications/" + application.id + "/submit", token, {
    method: "POST",
    body: JSON.stringify({ lockVersion: draft.lockVersion }),
  });
  return { form, application };
}

async function prepare() {
  requireConfiguration();
  const runID = (process.env.GITHUB_RUN_ID ?? Date.now()) + "-" + (process.env.GITHUB_RUN_ATTEMPT ?? 1);
  const fixture = { runID, identities: [], applicants: [], scanner: null };
  saveFixture(fixture);

  const admin = createIdentity(runID, "admin");
  fixture.identities.push(admin);
  saveFixture(fixture);
  const scanner = createIdentity(runID, "scanner");
  fixture.identities.push(scanner);
  saveFixture(fixture);
  const attendee = createIdentity(runID, "attendee");
  fixture.identities.push(attendee);
  saveFixture(fixture);

  psql("DELETE FROM ats.admin_email_allowlist WHERE normalized_email LIKE 'hat_load_%@loadtest.invalid'; INSERT INTO ats.admin_email_allowlist (normalized_email) VALUES (lower(:'email')) ON CONFLICT (normalized_email) DO NOTHING", { email: admin.email });
  const [adminToken, scannerToken, attendeeToken] = await sessionTokens([admin, scanner, attendee]);
  await api("/v1/me", adminToken);
  ensureOpenApplicationForm(admin.userId, runID);
  const scannerUser = await api("/v1/me", scannerToken);
  await api("/v1/admin/users/" + scannerUser.id + "/roles/scanner", adminToken, { method: "PUT", body: "{}" });
  await api("/v1/me", attendeeToken);

  const { form, application } = await createSubmittedApplication(attendee, attendeeToken);
  const reviewDraft = await api("/v1/reviewer/applications/" + application.id + "/review", adminToken, {
    method: "PUT",
    body: JSON.stringify({ lockVersion: 0, score: 5, recommendation: "strong_yes", internalNotes: "Synthetic load fixture" }),
  });
  await api("/v1/reviewer/applications/" + application.id + "/review/submit", adminToken, {
    method: "POST",
    body: JSON.stringify({ lockVersion: reviewDraft.review.lockVersion }),
  });
  const decision = await api("/v1/admin/applications/" + application.id + "/decisions", adminToken, {
    method: "POST",
    body: JSON.stringify({ outcome: "accepted", internalReason: "Synthetic load fixture" }),
  });
  await api("/v1/admin/decisions/" + decision.id + "/release", adminToken, { method: "POST" });
  const detail = await api("/v1/admin/applications/" + application.id, adminToken);
  const issuedPass = await api("/v1/admin/attendees/" + detail.attendeePass.attendeeId + "/passes", adminToken, { method: "POST" });
  const checkpoint = await api("/v1/admin/checkpoints", adminToken, {
    method: "POST",
    body: JSON.stringify({
      cycleId: form.cycleId,
      activityId: null,
      slug: "load-" + runID,
      name: "Synthetic load checkpoint",
      opensAt: null,
      closesAt: null,
      defaultAllowed: true,
      defaultMaxRedemptions: applicantCount,
      active: true,
    }),
  });
  fixture.scanner = {
    adminToken,
    scannerEmail: scanner.email,
    scannerClerkUserId: scanner.userId,
    scannerUserId: scannerUser.id,
    qrToken: issuedPass.qrToken,
    checkpointId: checkpoint.id,
    adminEmail: admin.email,
  };
  saveFixture(fixture);

  const batchSize = 8;
  for (let start = 0; start < applicantCount; start += batchSize) {
    const batch = Array.from({ length: Math.min(batchSize, applicantCount - start) }, (_, offset) => start + offset);
    const created = batch.map((index) => createIdentity(runID, "applicant", index + 1));
    fixture.identities.push(...created);
    saveFixture(fixture);
  }

  const applicantIdentities = fixture.identities.slice(3);
  const applicantTokens = await sessionTokens(applicantIdentities);
  fixture.applicants.push(...applicantIdentities.map((identity, index) => ({
    email: identity.email,
    token: applicantTokens[index],
  })));
  saveFixture(fixture);
  console.log("Prepared " + fixture.applicants.length + " synthetic applicants and one scanner fixture.");
}

async function refreshScanner() {
  requireConfiguration();
  const fixture = loadFixture();
  const scanner = fixture.identities.find((identity) => identity.userId === fixture.scanner.scannerClerkUserId);
  if (!scanner) throw new Error("Synthetic scanner identity is missing from the fixture");
  [fixture.scanner.token] = await sessionTokens([scanner]);
  saveFixture(fixture);
  console.log("Refreshed the synthetic scanner token.");
}

async function cleanup() {
  let fixture;
  try {
    fixture = loadFixture();
  } catch {
    return;
  }
  requireConfiguration();
  const scannerClerkUserId = fixture.scanner?.scannerClerkUserId ?? fixture.identities?.[1]?.userId;
  if (scannerClerkUserId) {
    try {
      psql("DELETE FROM ats.user_roles WHERE role = 'scanner' AND user_id = (SELECT id FROM ats.users WHERE clerk_user_id = :'clerk_user_id')", {
        clerk_user_id: scannerClerkUserId,
      });
    } catch {
      console.warn("Could not remove the temporary scanner role; manual staging cleanup is required.");
    }
  }
  const adminEmail = fixture.scanner?.adminEmail ?? fixture.identities?.[0]?.email;
  if (adminEmail) {
    try {
      psql("DELETE FROM ats.admin_email_allowlist WHERE normalized_email = lower(:'email')", { email: adminEmail });
    } catch {
      console.warn("Could not remove the temporary admin allowlist entry; manual staging cleanup is required.");
    }
  }
  console.log("Removed temporary staff access for " + fixture.identities.length + " staging-local synthetic identities.");
}

if (command === "prepare") await prepare();
else if (command === "refresh-scanner") await refreshScanner();
else if (command === "cleanup") await cleanup();
else throw new Error("usage: node tests/load/staging-fixture.mjs <prepare|refresh-scanner|cleanup>");
