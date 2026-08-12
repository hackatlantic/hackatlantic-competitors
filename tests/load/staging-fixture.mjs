import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

const command = process.argv[2];
const fixturePath = process.env.K6_FIXTURE_PATH ?? ".tmp/k6-staging-fixture.json";
const apiBaseURL = process.env.API_BASE_URL;
const webBaseURL = process.env.WEB_BASE_URL;
const clerkSecret = process.env.CLERK_SECRET_KEY;
const databaseURL = process.env.DATABASE_URL;
const vercelAutomationBypass = process.env.VERCEL_AUTOMATION_BYPASS_SECRET;
const applicantCount = Number(process.env.K6_APPLICANT_VUS ?? 100);

function requireConfiguration() {
  if (!apiBaseURL || !webBaseURL || !clerkSecret || !databaseURL) {
    throw new Error("API_BASE_URL, WEB_BASE_URL, CLERK_SECRET_KEY, and DATABASE_URL are required");
  }
  if (!Number.isInteger(applicantCount) || applicantCount < 1 || applicantCount > 200) {
    throw new Error("K6_APPLICANT_VUS must be between 1 and 200");
  }
}

async function clerk(path, options = {}) {
  const response = await fetch("https://api.clerk.com/v1" + path, {
    ...options,
    headers: {
      Authorization: "Bearer " + clerkSecret,
      "Content-Type": "application/json",
      ...options.headers,
    },
  });
  if (!response.ok) throw new Error("Clerk " + path + " failed with " + response.status);
  return response.json();
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

function clerkHostedSignInURL(ticket) {
  const accountsHost = process.env.CLERK_FRONTEND_API_URL?.replace(/\.clerk\.accounts\.dev$/, ".accounts.dev");
  if (!accountsHost || accountsHost === process.env.CLERK_FRONTEND_API_URL) {
    throw new Error("CLERK_FRONTEND_API_URL must identify a Clerk development instance");
  }
  const signInURL = new URL("https://" + accountsHost + "/sign-in");
  signInURL.searchParams.set("__clerk_ticket", ticket);
  signInURL.searchParams.set("redirect_url", webBaseURL);
  return signInURL.toString();
}

async function createIdentity(email, firstName, lastName) {
  const password = "Load!" + crypto.randomUUID() + "Aa9";
  const user = await clerk("/users", {
    method: "POST",
    body: JSON.stringify({
      email_address: [email],
      first_name: firstName,
      last_name: lastName,
      password,
      skip_password_checks: true,
      skip_legal_checks: true,
    }),
  });
  return { userId: user.id, email };
}

async function browserTokens(identities) {
  const [{ chromium }, { clerkSetup, setupClerkTestingToken }] = await Promise.all([
    import("@playwright/test"),
    import("@clerk/testing/playwright"),
  ]);
  await clerkSetup({ frontendApiUrl: process.env.CLERK_FRONTEND_API_URL });
  const browser = await chromium.launch();
  const tokens = [];
  try {
    const batchSize = 8;
    for (let start = 0; start < identities.length; start += batchSize) {
      const batch = identities.slice(start, start + batchSize);
      const batchTokens = await Promise.all(batch.map(async (identity) => {
        const context = await browser.newContext();
        try {
          await setupClerkTestingToken({
            context,
            options: { frontendApiUrl: process.env.CLERK_FRONTEND_API_URL },
          });
          if (vercelAutomationBypass) {
            const webOrigin = new URL(webBaseURL).origin;
            await context.route(webOrigin + "/**", async (route) => {
              await route.continue({
                headers: {
                  ...route.request().headers(),
                  "x-vercel-protection-bypass": vercelAutomationBypass,
                },
              });
            });
          }
          const page = await context.newPage();
          const authenticatedRequest = page.waitForRequest((request) =>
            request.url().startsWith(apiBaseURL + "/") &&
            request.headers().authorization?.startsWith("Bearer "),
          );
          const signIn = await clerk("/sign_in_tokens", {
            method: "POST",
            body: JSON.stringify({ user_id: identity.userId, expires_in_seconds: 60 }),
          });
          await page.goto(clerkHostedSignInURL(signIn.token));
          const authorization = (await authenticatedRequest).headers().authorization;
          if (!authorization?.startsWith("Bearer ")) {
            throw new Error("The staging app did not issue an authenticated API request");
          }
          return authorization.slice("Bearer ".length);
        } finally {
          await context.close();
        }
      }));
      tokens.push(...batchTokens);
    }
  } finally {
    await browser.close();
  }
  return tokens;
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

  const admin = await createIdentity("loadtest+clerk_test_admin_" + runID + "@example.com", "Synthetic", "Admin");
  fixture.identities.push(admin);
  saveFixture(fixture);
  const scanner = await createIdentity("loadtest+clerk_test_scanner_" + runID + "@example.com", "Synthetic", "Scanner");
  fixture.identities.push(scanner);
  saveFixture(fixture);
  const attendee = await createIdentity("loadtest+clerk_test_attendee_" + runID + "@example.com", "Synthetic", "Attendee");
  fixture.identities.push(attendee);
  saveFixture(fixture);

  psql("DELETE FROM ats.admin_email_allowlist WHERE normalized_email LIKE 'loadtest+admin-%@example.com' OR normalized_email LIKE 'loadtest+clerk_test_admin_%@example.com'; INSERT INTO ats.admin_email_allowlist (normalized_email) VALUES (lower(:'email')) ON CONFLICT (normalized_email) DO NOTHING", { email: admin.email });
  const [adminToken, scannerToken, attendeeToken] = await browserTokens([admin, scanner, attendee]);
  await api("/v1/me", adminToken);
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
    const created = await Promise.all(batch.map((index) => createIdentity(
      "loadtest+clerk_test_applicant_" + runID + "_" + (index + 1) + "@example.com",
      "Synthetic",
      "Applicant " + (index + 1),
    )));
    fixture.identities.push(...created);
    saveFixture(fixture);
    await new Promise((resolve) => setTimeout(resolve, 300));
  }

  const applicantIdentities = fixture.identities.slice(3);
  const applicantTokens = await browserTokens(applicantIdentities);
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
  [fixture.scanner.token] = await browserTokens([scanner]);
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
  const batchSize = 8;
  for (let start = 0; start < fixture.identities.length; start += batchSize) {
    await Promise.all(fixture.identities.slice(start, start + batchSize).map((identity) =>
      clerk("/users/" + identity.userId, { method: "DELETE" }).catch(() => undefined),
    ));
  }
  console.log("Deleted " + fixture.identities.length + " synthetic Clerk identities and removed temporary staff access.");
}

if (command === "prepare") await prepare();
else if (command === "refresh-scanner") await refreshScanner();
else if (command === "cleanup") await cleanup();
else throw new Error("usage: node tests/load/staging-fixture.mjs <prepare|refresh-scanner|cleanup>");
