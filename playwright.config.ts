import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  outputDir: "test-results/playwright",
  use: {
    baseURL: process.env.WEB_BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects:
    process.env.CLERK_PUBLISHABLE_KEY && process.env.CLERK_SECRET_KEY
      ? [
          { name: "setup", testMatch: /global\.setup\.ts/ },
          {
            name: "chromium",
            use: { browserName: "chromium" },
            dependencies: ["setup"],
            testIgnore: /global\.setup\.ts/,
          },
        ]
      : [{ name: "chromium", use: { browserName: "chromium" }, testIgnore: /global\.setup\.ts/ }],
});
