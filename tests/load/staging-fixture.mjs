import { execFileSync } from "node:child_process";
import { createHmac, randomUUID } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

const command = process.argv[2];
const fixturePath = process.env.K6_FIXTURE_PATH ?? ".tmp/k6-staging-fixture.json";
const apiBaseURL = process.env.API_BASE_URL;
const databaseURL = process.env.DATABASE_URL;
const loadTestAuthSecret = process.env.LOAD_TEST_AUTH_SECRET;
const applicantCount = Number(process.env.K6_APPLICANT_COUNT ?? process.env.K6_APPLICANT_VUS ?? 0);
const applicantProfile = process.env.K6_APPLICANT_PROFILE ?? "sustained";
const scannerPassCount = Number(process.env.K6_SCANNER_PASS_COUNT ?? 0);
const scannerIdentityCount = Number(process.env.K6_SCANNER_IDENTITIES ?? 20);

function requireConfiguration() {
  if (!apiBaseURL || !databaseURL || !loadTestAuthSecret) {
    throw new Error("API_BASE_URL, DATABASE_URL, and LOAD_TEST_AUTH_SECRET are required");
  }
  const decodedSecret = Buffer.from(loadTestAuthSecret, "base64");
  if (decodedSecret.length < 32 || decodedSecret.toString("base64") !== loadTestAuthSecret) {
    throw new Error("LOAD_TEST_AUTH_SECRET must be standard base64 containing at least 32 bytes");
  }
  if (!Number.isInteger(applicantCount) || applicantCount < 0 || applicantCount > 200) {
    throw new Error("K6_APPLICANT_COUNT must be between 0 and 200");
  }
  if (!Number.isInteger(scannerPassCount) || scannerPassCount < 0 || scannerPassCount > 3000) {
    throw new Error("K6_SCANNER_PASS_COUNT must be between 0 and 3000");
  }
  if (!Number.isInteger(scannerIdentityCount) || scannerIdentityCount < 0 || scannerIdentityCount > 25 || (scannerPassCount > 0 && scannerIdentityCount < 10)) {
    throw new Error("K6_SCANNER_IDENTITIES must be 0 for applicant-only profiles or between 10 and 25 for scanner profiles");
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
  mkdirSync(dirname(fixturePath), { recursive: true });
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

function seedAcceptedAttendees(form, runID, count) {
  const attendees = Array.from({ length: count }, (_, index) => {
    const identity = createIdentity(runID, "scanner_attendee", index + 1);
    return {
      clerk_user_id: identity.userId,
      email: identity.email,
      user_id: randomUUID(),
      application_id: randomUUID(),
      attendee_id: randomUUID(),
      display_name: "Synthetic attendee " + (index + 1),
    };
  });
  for (let start = 0; start < attendees.length; start += 100) {
    const rows = JSON.stringify(attendees.slice(start, start + 100));
    psql(`
WITH input AS (
  SELECT *
  FROM jsonb_to_recordset(:'rows'::jsonb) AS row(
    clerk_user_id text,
    email text,
    user_id uuid,
    application_id uuid,
    attendee_id uuid,
    display_name text
  )
)
INSERT INTO ats.users (id, clerk_user_id, primary_email, display_name)
SELECT user_id, clerk_user_id, email, display_name FROM input;

WITH input AS (
  SELECT *
  FROM jsonb_to_recordset(:'rows'::jsonb) AS row(
    clerk_user_id text,
    email text,
    user_id uuid,
    application_id uuid,
    attendee_id uuid,
    display_name text
  )
)
INSERT INTO ats.user_roles (user_id, role)
SELECT user_id, 'applicant' FROM input;

WITH input AS (
  SELECT *
  FROM jsonb_to_recordset(:'rows'::jsonb) AS row(
    clerk_user_id text,
    email text,
    user_id uuid,
    application_id uuid,
    attendee_id uuid,
    display_name text
  )
)
INSERT INTO ats.applications (
  id,
  cycle_id,
  form_id,
  applicant_user_id,
  status,
  submitted_at,
  current_decision,
  decision_released_at
)
SELECT
  application_id,
  :'cycle_id'::uuid,
  :'form_id'::uuid,
  user_id,
  'accepted',
  CURRENT_TIMESTAMP,
  'accepted',
  CURRENT_TIMESTAMP
FROM input;

WITH input AS (
  SELECT *
  FROM jsonb_to_recordset(:'rows'::jsonb) AS row(
    clerk_user_id text,
    email text,
    user_id uuid,
    application_id uuid,
    attendee_id uuid,
    display_name text
  )
)
INSERT INTO ats.attendees (id, cycle_id, application_id, user_id, display_name, email)
SELECT attendee_id, :'cycle_id'::uuid, application_id, user_id, display_name, email FROM input;

WITH input AS (
  SELECT *
  FROM jsonb_to_recordset(:'rows'::jsonb) AS row(
    clerk_user_id text,
    email text,
    user_id uuid,
    application_id uuid,
    attendee_id uuid,
    display_name text
  )
)
INSERT INTO ats.attendee_roles (attendee_id, role)
SELECT attendee_id, 'hacker' FROM input;
`, { rows, cycle_id: form.cycleId, form_id: form.id });
  }
  return attendees;
}

async function issueScannerPasses(attendees, adminToken) {
  const passes = [];
  const batchSize = 20;
  for (let start = 0; start < attendees.length; start += batchSize) {
    const batch = attendees.slice(start, start + batchSize);
    const issued = await Promise.all(batch.map((attendee) =>
      api("/v1/admin/attendees/" + attendee.attendee_id + "/passes", adminToken, { method: "POST" }),
    ));
    passes.push(...issued.map((pass) => ({ qrToken: pass.qrToken, attendeeId: pass.attendeeId })));
  }
  return passes;
}

function applicantAnswers(form, email, sequence) {
  return Object.fromEntries(form.questions.map((question) => {
    const description = question.key + " " + question.label;
    if (question.type === "boolean") return [question.key, true];
    if (question.type === "number") return [question.key, sequence + 1];
    if (/email/i.test(description)) return [question.key, email];
    if (/school/i.test(description)) return [question.key, "Synthetic Atlantic University"];
    return [question.key, "Synthetic deadline response " + (sequence + 1)];
  }));
}

async function prepareDeadlineApplicants(applicants, form) {
  const prepared = [];
  for (let start = 0; start < applicants.length; start += 4) {
    const batch = applicants.slice(start, start + 4);
    const results = await Promise.all(batch.map(async (applicant, offset) => {
      const index = start + offset;
      const application = await api("/v1/applications", applicant.token, { method: "POST" });
      const draft = await api("/v1/applications/" + application.id + "/draft", applicant.token, {
        method: "PUT",
        body: JSON.stringify({ lockVersion: application.lockVersion, answers: applicantAnswers(form, applicant.email, index) }),
      });
      if (form.resumeRequired) {
        await api("/v1/applications/" + application.id + "/resume", applicant.token, {
          method: "PUT",
          body: "%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
          headers: { "Content-Type": "application/pdf", "X-File-Name": "synthetic-deadline-" + (index + 1) + ".pdf" },
        });
      }
      return { ...applicant, applicationId: application.id, lockVersion: draft.lockVersion };
    }));
    prepared.push(...results);
  }
  return prepared;
}

async function prepare() {
  requireConfiguration();
  const runID = (process.env.GITHUB_RUN_ID ?? Date.now()) + "-" + (process.env.GITHUB_RUN_ATTEMPT ?? 1);
  const fixture = { runID, identities: [], applicants: [], scanner: null };
  saveFixture(fixture);

  const admin = createIdentity(runID, "admin");
  fixture.identities.push(admin);
  saveFixture(fixture);
  const scanners = Array.from({ length: scannerIdentityCount }, (_, index) => createIdentity(runID, "scanner", index + 1));
  fixture.identities.push(...scanners);
  saveFixture(fixture);
  psql("INSERT INTO ats.admin_email_allowlist (normalized_email) VALUES (lower(:'email')) ON CONFLICT (normalized_email) DO NOTHING", { email: admin.email });
  const [adminToken] = await sessionTokens([admin]);
  await api("/v1/me", adminToken);
  ensureOpenApplicationForm(admin.userId, runID);
  const scannerTokens = await sessionTokens(scanners);
  for (let start = 0; start < scanners.length; start += 4) {
    const batch = scanners.slice(start, start + 4);
    await Promise.all(batch.map(async (_scanner, offset) => {
      const scannerUser = await api("/v1/me", scannerTokens[start + offset]);
      await api("/v1/admin/users/" + scannerUser.id + "/roles/scanner", adminToken, { method: "PUT", body: "{}" });
    }));
  }
  const form = await api("/v1/application-forms/current", adminToken);
  let checkpoint = null;
  let scannerPasses = [];
  if (scannerPassCount > 0) {
    checkpoint = await api("/v1/admin/checkpoints", adminToken, {
      method: "POST",
      body: JSON.stringify({
        cycleId: form.cycleId,
        activityId: null,
        slug: "load-" + runID,
        name: "Synthetic load checkpoint",
        opensAt: null,
        closesAt: null,
        defaultAllowed: true,
        defaultMaxRedemptions: 1,
        active: true,
      }),
    });
    const scannerAttendees = seedAcceptedAttendees(form, runID, scannerPassCount);
    scannerPasses = await issueScannerPasses(scannerAttendees, adminToken);
  }
  fixture.scanner = {
    identities: scanners,
    tokens: scannerTokens,
    passes: scannerPasses,
    checkpointId: checkpoint?.id ?? null,
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

  const applicantIdentities = fixture.identities.slice(1 + scanners.length);
  const applicantTokens = await sessionTokens(applicantIdentities);
  fixture.applicants.push(...applicantIdentities.map((identity, index) => ({
    email: identity.email,
    token: applicantTokens[index],
  })));
  if (applicantProfile === "deadline") fixture.applicants = await prepareDeadlineApplicants(fixture.applicants, form);
  saveFixture(fixture);
  console.log("Prepared " + fixture.applicants.length + " synthetic applicants and " + fixture.scanner.passes.length + " distinct passes across " + fixture.scanner.identities.length + " scanner identities.");
}

async function refreshScanner() {
  requireConfiguration();
  const fixture = loadFixture();
  if (!Array.isArray(fixture.scanner?.identities) || fixture.scanner.identities.length === 0) {
    throw new Error("Synthetic scanner identities are missing from the fixture");
  }
  fixture.scanner.tokens = await sessionTokens(fixture.scanner.identities);
  saveFixture(fixture);
  console.log("Refreshed the synthetic scanner token.");
}

function verifyScanner() {
  requireConfiguration();
  const fixture = loadFixture();
  const checkpointID = fixture.scanner?.checkpointId;
  if (!checkpointID) throw new Error("Synthetic scanner checkpoint is missing from the fixture");
  const expectedRedemptions = (process.env.K6_SCANNER_PROFILE ?? "release") === "contention" ? 1 : scannerPassCount;
  psql(`SELECT CASE WHEN count(*) = :'expected_redemptions'::bigint
      AND count(*) = count(DISTINCT attendee_id)
    THEN 'true' ELSE 'false' END AS verified
  FROM ats.redemptions
  WHERE checkpoint_id = :'checkpoint_id'::uuid
  \\gset
  \\if :verified
  \\else
    \\quit 1
  \\endif`, { checkpoint_id: checkpointID, expected_redemptions: expectedRedemptions });
  console.log("Verified " + expectedRedemptions + " atomic redemption ledger entries with no duplicate attendees.");
}

async function cleanup() {
  let fixture;
  try {
    fixture = loadFixture();
  } catch {
    return;
  }
  requireConfiguration();
  const scannerIdentities = fixture.scanner?.identities ?? [];
  if (scannerIdentities.length > 0) {
    try {
      psql(`DELETE FROM ats.user_roles
        WHERE role = 'scanner'
          AND user_id IN (
            SELECT ats.users.id
            FROM ats.users
            JOIN jsonb_array_elements_text(:'clerk_user_ids'::jsonb) AS ids(clerk_user_id)
              ON ats.users.clerk_user_id = ids.clerk_user_id
          )`, { clerk_user_ids: JSON.stringify(scannerIdentities.map((identity) => identity.userId)) });
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
  console.log("Removed temporary staging admin and scanner privileges. Append-only synthetic redemption records remain isolated by their hat_load run identifiers.");
}

if (command === "prepare") await prepare();
else if (command === "refresh-scanner") await refreshScanner();
else if (command === "verify-scanner") verifyScanner();
else if (command === "cleanup") await cleanup();
else throw new Error("usage: node tests/load/staging-fixture.mjs <prepare|refresh-scanner|verify-scanner|cleanup>");
