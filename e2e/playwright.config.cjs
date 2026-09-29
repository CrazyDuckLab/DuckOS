// @ts-check
const { defineConfig, devices } = require("@playwright/test");

const BASE_URL = process.env.DUCKOS_E2E_URL ?? "http://127.0.0.1:8080";
const TOKEN = (process.env.DUCKOS_E2E_TOKEN ?? "").trim();

module.exports = defineConfig({
  testDir: "./tests",
  testMatch: /.*\.spec\.js$/,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: process.env.CI
    ? [["list"], ["html", { open: "never", outputFolder: "artifacts/html-report" }]]
    : [["list"]],
  outputDir: "artifacts/test-results",
  timeout: 30_000,
  use: {
    baseURL: BASE_URL,
    headless: true,
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    trace: "retain-on-failure",
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1440, height: 900 },
      },
    },
  ],
});
