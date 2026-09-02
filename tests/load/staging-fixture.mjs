import { execFileSync } from "node:child_process";
import { createHmac, createHash, randomUUID } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { assertStagingTarget, applicantAnswers as expectedAnswers, fixedResume, RESUME_BYTES, shouldUpload } from "./profile-contract.mjs";

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
  assertStagingTarget(apiBaseURL);
  if (!apiBaseURL || !databaseURL || !loadTestAuthSecret) {
    throw new Error("API_BASE_URL, DATABASE_URL, and LOAD_TEST_AUTH_SECRET are required");
  }
  const decodedSecret = Buffer.from(loadTestAuthSecret, "base64");
  if (decodedSecret.length < 32 || decodedSecret.toString("base64") !== loadTestAuthSecret) {
    throw new Error("LOAD_TEST_AUTH_SECRET must be standard base64 containing at least 32 bytes");
  }
  const db = new URL(databaseURL);
  if (db.hostname !== "aws-0-ca-central-1.pooler.supabase.com" || decodeURIComponent(db.username) !== "postgres.ovzrhurmiwqthfgycamx") {
    throw new Error("Fixtures require the known isolated staging database");
  }
  if (!Number.isInteger(applicantCount) || applicantCount < 0 || applicantCount > 250) {
    throw new Error("K6_APPLICANT_COUNT must be between 0 and 250");
  }
  if (!Number.isInteger(scannerPassCount) || scannerPassCount < 0 || scannerPassCount > 3500) {
    throw new Error("K6_SCANNER_PASS_COUNT must be between 0 and 3500");
  }
  if (!Number.isInteger(scannerIdentityCount) || scannerIdentityCount < 0 || scannerIdentityCount > 25 || (scannerPassCount > 0 && scannerIdentityCount < 10)) {
    throw new Error("K6_SCANNER_IDENTITIES must be 0 for applicant-only profiles or between 10 and 25 for scanner profiles");
  }
}

async function api(path, token, options = {}) {
  const response = await fetch(apiBaseURL + path, {
    ...options,
    redirect: "error",
    signal: AbortSignal.timeout(30000),
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
  const args = ["-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-q"];
  for (const [name, value] of Object.entries(variables)) args.push("-v", name + "=" + value);
  try { return execFileSync("psql", args, {
    env: databaseEnvironment(),
    input: sql + "\n",
    stdio: ["pipe", "pipe", "pipe"],
    encoding: "utf8",
  }).trim(); } catch { throw new Error("Staging database operation failed (details suppressed to protect fixture data)"); }
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
  '{"resumeRequired":false,"resumeAfterQuestionKey":"school","questions":[{"key":"fullName","label":"Name","type":"string","required":true,"section":"Build your profile","control":"text"},{"key":"email","label":"Email","type":"string","required":true,"section":"Build your profile","control":"email","help":"Verified through your signed-in account."},{"key":"school","label":"School","type":"string","required":true,"section":"Build your profile","control":"text"},{"key":"dietaryRestrictions","label":"Dietary restrictions","type":"string","required":true,"section":"Build your profile","control":"text","help":"Enter None if you do not have any."},{"key":"hackAtlanticExcitement","label":"What are you most excited about at Hack Atlantic?","type":"string","required":true,"section":"Hackathon Specific Questions","control":"textarea","maxWords":100,"help":"Maximum 100 words."},{"key":"priorHackathonExperience","label":"Prior hackathon experience","type":"string","required":true,"section":"Hackathon Specific Questions","control":"select","options":["This is my first","1–3","3+"]},{"key":"desiredTeammateNames","label":"Desired teammate names (Optional)","type":"string","required":false,"section":"Hackathon Specific Questions","control":"text"},{"key":"hardwareProject","label":"Are you looking to make a hardware project?","type":"boolean","required":true,"section":"Hackathon Specific Questions"},{"key":"hardwareEquipment","label":"What equipment are you looking to use?","type":"string","required":true,"section":"Hackathon Specific Questions","control":"textarea","showWhen":{"key":"hardwareProject","equals":true}}]}'::jsonb,
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
    const payload = Buffer.from(JSON.stringify({ sub: identity.userId, iat: now - 5, exp: now + 595 }));
    const signature = createHmac("sha256", secret).update(payload).digest();
    return "hat_load_v1." + payload.toString("base64url") + "." + signature.toString("base64url");
  });
}

function sessionWindows(identity) {
  const now = Math.floor(Date.now() / 1000);
  return { tokenWindows: Array.from({ length: 3 }, (_, index) => {
    const iat = now - 5 + index * 540;
    const exp = iat + 600;
    const payload = Buffer.from(JSON.stringify({ sub: identity.userId, iat, exp }));
    const signature = createHmac("sha256", Buffer.from(loadTestAuthSecret, "base64")).update(payload).digest();
    return { iat, exp, token: "hat_load_v1." + payload.toString("base64url") + "." + signature.toString("base64url") };
  }) };
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

async function issueScannerPasses(attendees, adminIdentity) {
  const passes = [];
  const batchSize = 20;
  for (let start = 0; start < attendees.length; start += batchSize) {
    const [adminToken] = await sessionTokens([adminIdentity]);
    const batch = attendees.slice(start, start + batchSize);
    const issued = await Promise.all(batch.map((attendee) =>
      api("/v1/admin/attendees/" + attendee.attendee_id + "/passes", adminToken, { method: "POST" }),
    ));
    passes.push(...issued.map((pass) => ({ qrToken: pass.qrToken, attendeeId: pass.attendeeId })));
  }
  return passes;
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
        body: JSON.stringify({ lockVersion: application.lockVersion, answers: expectedAnswers(form, applicant.email, index) }),
      });
      if (form.resumeRequired) {
        await api("/v1/applications/" + application.id + "/resume", applicant.token, {
          method: "PUT",
          body: fixedResume(),
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
  const runID = (process.env.GITHUB_RUN_ID ?? Date.now()) + "-" + (process.env.GITHUB_RUN_ATTEMPT ?? 1) + "-" + (process.env.K6_REPETITION ?? 1);
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
  fixture.form = form;
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
    scannerPasses = await issueScannerPasses(scannerAttendees, admin);
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
    userId: identity.userId,
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

function refreshAll() {
  requireConfiguration();
  const fixture = loadFixture();
  fixture.scanner.sessions = fixture.scanner.identities.map(sessionWindows);
  fixture.applicants = fixture.applicants.map((identity) => ({ ...identity, ...sessionWindows(identity) }));
  saveFixture(fixture);
  console.log("Refreshed overlapping synthetic token windows; each token retains its ten-minute lifetime.");
}

function writeEvidence(name, evidence) {
  mkdirSync(".tmp", { recursive: true });
  writeFileSync(`.tmp/${name}.json`, JSON.stringify(evidence, null, 2) + "\n");
  console.log(JSON.stringify(evidence));
}

function verifyScanner() {
  requireConfiguration();
  const fixture = loadFixture();
  const checkpointID = fixture.scanner?.checkpointId;
  if (!checkpointID) throw new Error("Synthetic scanner checkpoint is missing from the fixture");
  const profile = process.env.K6_SCANNER_PROFILE ?? "release";
  const summaryPath = process.env.K6_SUMMARY_PATH;
  const summary = summaryPath ? JSON.parse(readFileSync(summaryPath, "utf8")) : null;
  const expectedRedemptions = profile === "contention" ? 1 : profile === "repeatability" ? summary?.metrics.scanner_completed?.count : scannerPassCount;
  if (!Number.isInteger(expectedRedemptions) || expectedRedemptions < 1) throw new Error("Missing successful scan count for ledger verification");
  psql(`SELECT CASE WHEN count(*) = :'expected_redemptions'::bigint
      AND count(*) = count(DISTINCT attendee_id)
      AND count(*) = count(DISTINCT idempotency_key)
      AND min(ordinal) = 1 AND max(ordinal) = 1
    THEN 'true' ELSE 'false' END AS verified
  FROM ats.redemptions
  WHERE checkpoint_id = :'checkpoint_id'::uuid
  \\gset
  \\if :verified
  \\else
    \\quit 1
  \\endif`, { checkpoint_id: checkpointID, expected_redemptions: expectedRedemptions });
  console.log("Verified " + expectedRedemptions + " atomic redemption ledger entries with no duplicate attendees.");
  if (profile === "contention") {
    const requests = Number(psql("SELECT count(*) FROM ats.redemption_requests WHERE checkpoint_id = :'checkpoint_id'::uuid", { checkpoint_id: checkpointID }));
    if (requests !== Number(process.env.K6_SCANNER_ITERATIONS ?? 100)) throw new Error("Replays created extra request ledger entries");
  }
  writeEvidence("scanner-ledger", { verified: true, profile, redemptions: expectedRedemptions, duplicateAttendees: 0, overLimitRedemptions: 0 });
}

function verifyApplicants() {
  requireConfiguration();
  const fixture = loadFixture();
  const expected = fixture.applicants.map((applicant, index) => ({
    user_id: applicant.userId,
    answers: expectedAnswers(fixture.form, applicant.email, index, applicantProfile === "sustained" ? 2 : 1),
    upload: shouldUpload(fixture.form, index, applicantProfile),
  }));
  const report = JSON.parse(psql(`
WITH expected AS (SELECT * FROM jsonb_to_recordset(:'expected'::jsonb) AS e(user_id text, answers jsonb, upload boolean)),
actual AS (
  SELECT e.*, a.id, a.status, a.submitted_at,
    (SELECT jsonb_object_agg(question_key, value_json) FROM ats.application_answers WHERE application_id = a.id) AS saved_answers,
    r.byte_size, encode(r.sha256, 'hex') AS resume_hash,
    count(a.id) OVER (PARTITION BY e.user_id) AS application_count
  FROM expected e LEFT JOIN ats.users u ON u.clerk_user_id = e.user_id
  LEFT JOIN ats.applications a ON a.applicant_user_id = u.id AND a.cycle_id = :'cycle_id'::uuid
  LEFT JOIN ats.application_resumes r ON r.application_id = a.id
)
SELECT jsonb_build_object(
  'expected', (SELECT count(*) FROM expected),
  'submitted', count(*) FILTER (WHERE status = 'submitted' AND submitted_at IS NOT NULL),
  'duplicateApplications', count(*) FILTER (WHERE application_count > 1),
  'lostAnswers', count(*) FILTER (WHERE status = 'submitted' AND saved_answers IS DISTINCT FROM answers),
  'resumeMismatches', count(*) FILTER (WHERE status = 'submitted' AND upload AND
    (byte_size IS DISTINCT FROM :'resume_bytes'::bigint OR resume_hash IS DISTINCT FROM :'resume_hash')),
  'persistedResumes', count(byte_size)
) FROM actual`, {
    expected: JSON.stringify(expected), cycle_id: fixture.form.cycleId, resume_bytes: RESUME_BYTES,
    resume_hash: createHash("sha256").update(fixedResume()).digest("hex"),
  }));
  writeEvidence("applicant-ledger", report);
  if (report.submitted < Math.ceil(expected.length * 0.99) || report.duplicateApplications || report.lostAnswers || report.resumeMismatches) throw new Error("Applicant persistence acceptance criteria failed");
}

async function cleanup() {
  let fixture;
  try {
    fixture = loadFixture();
  } catch {
    return;
  }
  requireConfiguration();
  const scannerIdentities = fixture.scanner?.identities ?? fixture.identities.filter((identity) => /_scanner_\d+$/.test(identity.userId));
  let cleanupFailed = false;
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
      cleanupFailed = true;
      console.warn("Could not remove the temporary scanner role; manual staging cleanup is required.");
    }
  }
  const adminEmail = fixture.scanner?.adminEmail ?? fixture.identities?.[0]?.email;
  if (adminEmail) {
    try {
      psql("DELETE FROM ats.admin_email_allowlist WHERE normalized_email = lower(:'email')", { email: adminEmail });
    } catch {
      cleanupFailed = true;
      console.warn("Could not remove the temporary admin allowlist entry; manual staging cleanup is required.");
    }
  }
  if (cleanupFailed) throw new Error("Synthetic staff-access cleanup needs attention");
  console.log("Removed temporary staging admin and scanner privileges. Append-only synthetic redemption records remain isolated by their hat_load run identifiers.");
}

if (command === "prepare") await prepare();
else if (command === "refresh-scanner") await refreshScanner();
else if (command === "refresh-all") refreshAll();
else if (command === "verify-scanner") verifyScanner();
else if (command === "verify-applicants") verifyApplicants();
else if (command === "cleanup") await cleanup();
else throw new Error("usage: node tests/load/staging-fixture.mjs <prepare|refresh-scanner|verify-scanner|cleanup>");
