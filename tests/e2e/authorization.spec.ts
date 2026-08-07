import { clerk } from "@clerk/testing/playwright";
import { expect, request as playwrightRequest, test, type Page } from "@playwright/test";

const apiBaseURL = process.env.API_BASE_URL ?? "http://localhost:8080";

async function sessionToken(page: Page, emailAddress: string): Promise<string> {
  await page.goto("/");
  await clerk.signIn({ page, emailAddress });
  const token = await page.evaluate(async () => {
    const clerkInstance = (window as unknown as { Clerk?: { session?: { getToken(): Promise<string | null> } } }).Clerk;
    return clerkInstance?.session?.getToken() ?? null;
  });
  if (!token) throw new Error(`Clerk did not create a session token for ${emailAddress}`);
  return token;
}

const identities = [
  { name: "applicant", email: process.env.E2E_APPLICANT_EMAIL, expectedRoles: ["applicant"] },
  { name: "administrator", email: process.env.E2E_ADMIN_EMAIL, expectedRoles: ["applicant", "admin"] },
  { name: "scanner", email: process.env.E2E_SCANNER_EMAIL, expectedRoles: ["applicant", "scanner"] },
];

for (const identity of identities) {
  test(`${identity.name} has the expected least-privilege API projection`, async ({ page }) => {
    test.skip(!identity.email, `Set E2E_${identity.name.toUpperCase()}_EMAIL in staging.`);
    const token = await sessionToken(page, identity.email!);
    const api = await playwrightRequest.newContext({
      baseURL: apiBaseURL,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
    const response = await api.get("/v1/me");
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(body.roles).toEqual(identity.expectedRoles);
    expect(JSON.stringify(body)).not.toMatch(/resume|answers|qr|claim/i);
    await api.dispose();
  });
}

test("applicant is denied the private ATS queue", async ({ page }) => {
  test.skip(!process.env.E2E_APPLICANT_EMAIL, "Set E2E_APPLICANT_EMAIL in staging.");
  const token = await sessionToken(page, process.env.E2E_APPLICANT_EMAIL!);
  const api = await playwrightRequest.newContext({ baseURL: apiBaseURL, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });
  expect((await api.get("/v1/admin/applications")).status()).toBe(403);
  await api.dispose();
});

test("scanner reaches checkpoints but not private ATS data", async ({ page }) => {
  test.skip(!process.env.E2E_SCANNER_EMAIL, "Set E2E_SCANNER_EMAIL in staging.");
  const token = await sessionToken(page, process.env.E2E_SCANNER_EMAIL!);
  const api = await playwrightRequest.newContext({ baseURL: apiBaseURL, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });
  expect((await api.get("/v1/checkpoints")).ok()).toBeTruthy();
  expect((await api.get("/v1/admin/applications")).status()).toBe(403);
  await api.dispose();
});

test("administrator reaches the ATS queue", async ({ page }) => {
  test.skip(!process.env.E2E_ADMIN_EMAIL, "Set E2E_ADMIN_EMAIL in staging.");
  const token = await sessionToken(page, process.env.E2E_ADMIN_EMAIL!);
  const api = await playwrightRequest.newContext({ baseURL: apiBaseURL, extraHTTPHeaders: { Authorization: `Bearer ${token}` } });
  expect((await api.get("/v1/admin/applications")).ok()).toBeTruthy();
  await api.dispose();
});
