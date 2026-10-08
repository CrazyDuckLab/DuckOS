import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  testMatch: /live-ui-smoke\.spec\.ts$/,
  timeout: 60_000,
  workers: 1,
  reporter: [["list"]],
  use: {
    headless: true,
    viewport: { width: 1280, height: 800 },
    actionTimeout: 15_000,
    navigationTimeout: 20_000,
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
});
