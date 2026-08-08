import { createClerkClient } from "@clerk/backend";
import { clerk } from "@clerk/testing/playwright";
import { expect, request as playwrightRequest, test, type APIResponse, type Browser, type Page } from "@playwright/test";

const apiBaseURL = process.env.API_BASE_URL ?? "http://localhost:8080";

async function tokenFor(page: Page, emailAddress: string): Promise<string> {
  await page.goto("/");
  await clerk.signIn({ page, emailAddress });
  const token = await page.evaluate(() => window.Clerk.session?.getToken() ?? null);
  if (!token) throw new Error(`No Clerk session token for ${emailAddress}`);
  return token;
}

async function tokenInIsolatedContext(browser: Browser, emailAddress: string) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const token = await tokenFor(page, emailAddress);
  return { token, context };
}

async function expectJSON(response: APIResponse, status: number) {
  const body = await response.json().catch(() => ({ nonJSON: true }));
  expect(response.status(), JSON.stringify(body)).toBe(status);
  return body;
}

test("applicant submission becomes a reviewed acceptance, attendee pass, and scanner lookup", async ({ browser }) => {
  test.skip(process.env.E2E_FULL_LIFECYCLE !== "true", "Set E2E_FULL_LIFECYCLE=true for the staging release gate.");
  test.skip(!process.env.E2E_ADMIN_EMAIL || !process.env.E2E_SCANNER_EMAIL, "Staging admin and scanner identities are required.");
  test.skip(!process.env.CLERK_SECRET_KEY, "A Clerk development secret key is required.");

  const unique = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const applicantEmail = `applicant+clerk_test_${unique}@example.com`;
  const clerkClient = createClerkClient({ secretKey: process.env.CLERK_SECRET_KEY! });
  const applicantUser = await clerkClient.users.createUser({
    emailAddress: [applicantEmail],
    firstName: "Synthetic",
    lastName: "Applicant",
    skipPasswordRequirement: true,
    skipLegalChecks: true,
  });

  const contexts: Array<{ close(): Promise<void> }> = [];
  try {
    const applicantIdentity = await tokenInIsolatedContext(browser, applicantEmail);
    contexts.push(applicantIdentity.context);
    const adminIdentity = await tokenInIsolatedContext(browser, process.env.E2E_ADMIN_EMAIL!);
    contexts.push(adminIdentity.context);
    const scannerIdentity = await tokenInIsolatedContext(browser, process.env.E2E_SCANNER_EMAIL!);
    contexts.push(scannerIdentity.context);

    const applicant = await playwrightRequest.newContext({ baseURL: apiBaseURL, extraHTTPHeaders: { Authorization: `Bearer ${applicantIdentity.token}` } });
    const admin = await playwrightRequest.newContext({ baseURL: apiBaseURL, extraHTTPHeaders: { Authorization: `Bearer ${adminIdentity.token}` } });
    const scanner = await playwrightRequest.newContext({ baseURL: apiBaseURL, extraHTTPHeaders: { Authorization: `Bearer ${scannerIdentity.token}` } });
    contexts.push(
      { close: () => applicant.dispose() },
      { close: () => admin.dispose() },
      { close: () => scanner.dispose() },
    );

    await expectJSON(await applicant.get("/v1/me"), 200);
    const form = await expectJSON(await applicant.get("/v1/application-forms/current"), 200);
    const application = await expectJSON(await applicant.post("/v1/applications"), 200);

    const answers = Object.fromEntries(
      form.questions
        .filter((question: { required: boolean }) => question.required)
        .map((question: { key: string; label: string; type: string }) => {
          if (question.type === "boolean") return [question.key, true];
          if (question.type === "number") return [question.key, 1];
          if (/email/i.test(`${question.key} ${question.label}`)) return [question.key, applicantEmail];
          if (/school/i.test(`${question.key} ${question.label}`)) return [question.key, "Synthetic Atlantic University"];
          return [question.key, `Synthetic response for ${question.label}`];
        }),
    );
    const draft = await expectJSON(
      await applicant.put(`/v1/applications/${application.id}/draft`, { data: { lockVersion: application.lockVersion, answers } }),
      200,
    );

    const pdf = Buffer.from("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n");
    await expectJSON(
      await applicant.put(`/v1/applications/${application.id}/resume`, {
        data: pdf,
        headers: { "Content-Type": "application/pdf", "X-File-Name": "synthetic-resume.pdf" },
      }),
      200,
    );
    const submitted = await expectJSON(
      await applicant.post(`/v1/applications/${application.id}/submit`, { data: { lockVersion: draft.lockVersion } }),
      200,
    );
    expect(submitted.status).toBe("submitted");

    const privateDetail = await expectJSON(await admin.get(`/v1/admin/applications/${application.id}`), 200);
    expect(privateDetail.applicant.email).toBe(applicantEmail);
    const resume = await admin.get(`/v1/admin/applications/${application.id}/resume`);
    expect(resume.status()).toBe(200);
    expect(resume.headers()["content-type"]).toContain("application/pdf");

    const reviewDraft = await expectJSON(
      await admin.put(`/v1/reviewer/applications/${application.id}/review`, {
        data: { lockVersion: 0, score: 5, recommendation: "strong_yes", internalNotes: "Synthetic release verification" },
      }),
      200,
    );
    const reviewSubmitted = await expectJSON(
      await admin.post(`/v1/reviewer/applications/${application.id}/review/submit`, { data: { lockVersion: reviewDraft.review.lockVersion } }),
      200,
    );
    expect(reviewSubmitted.review.status).toBe("submitted");

    const decision = await expectJSON(
      await admin.post(`/v1/admin/applications/${application.id}/decisions`, {
        data: { outcome: "accepted", internalReason: "Synthetic end-to-end verification" },
      }),
      201,
    );
    await expectJSON(await admin.post(`/v1/admin/decisions/${decision.id}/release`), 200);

    const applicantDecision = await expectJSON(await applicant.get(`/v1/applications/${application.id}/decision`), 200);
    expect(applicantDecision).toMatchObject({ applicationId: application.id, outcome: "accepted" });
    expect(JSON.stringify(applicantDecision)).not.toMatch(/internal|review|score|recommendation/i);

    const acceptedDetail = await expectJSON(await admin.get(`/v1/admin/applications/${application.id}`), 200);
    const attendeeId = acceptedDetail.attendeePass?.attendeeId;
    expect(attendeeId).toEqual(expect.any(String));
    const issuedPass = await expectJSON(await admin.post(`/v1/admin/attendees/${attendeeId}/passes`), 201);
    expect(issuedPass.qrToken).toEqual(expect.any(String));
    expect(issuedPass.claimToken).toEqual(expect.any(String));

    const webPass = await expectJSON(await applicant.get("/v1/attendee/pass"), 200);
    expect(webPass.qrToken).toBe(issuedPass.qrToken);
    expect(JSON.stringify(webPass)).not.toMatch(/claimToken|claimUrl|email|answers|review/i);

    const lookup = await expectJSON(await scanner.post("/v1/scans/lookup", { data: { qrToken: webPass.qrToken } }), 200);
    expect(lookup.attendee.displayName).toBe("Synthetic Applicant");
    expect(JSON.stringify(lookup)).not.toMatch(/email|answers|review|decision|claim/i);
  } finally {
    await Promise.allSettled(contexts.map((context) => context.close()));
    await clerkClient.users.deleteUser(applicantUser.id).catch(() => undefined);
  }
});
