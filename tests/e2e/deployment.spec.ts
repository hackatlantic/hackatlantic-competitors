import { expect, request as playwrightRequest, test } from "@playwright/test";

const apiBaseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
const expectedSha = process.env.EXPECTED_GIT_SHA;

test.describe("public deployment contract", () => {
  test("API is ready and exposes the promoted build", async () => {
    const api = await playwrightRequest.newContext({ baseURL: apiBaseURL });
    const readiness = await api.get("/readyz");
    expect(readiness.ok()).toBeTruthy();
    await expect(readiness.json()).resolves.toEqual({ status: "ready" });

    const version = await api.get("/versionz");
    expect(version.ok()).toBeTruthy();
    const body = await version.json();
    expect(body).toEqual({
      version: expect.any(String),
      gitSha: expect.any(String),
      builtAt: expect.any(String),
      environment: expect.stringMatching(/^(staging|production)$/),
    });
    if (expectedSha) expect(body.gitSha).toBe(expectedSha);
    await api.dispose();
  });

  test("frontend renders without a server failure", async ({ page }) => {
    const response = await page.goto("/");
    expect(response?.status()).toBeLessThan(500);
    await expect(page.locator("body")).not.toContainText("Internal Server Error");
  });
});
