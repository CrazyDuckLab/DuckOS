import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: /easy-devices\.spec\.ts$/,
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  reporter: [["list"]],
  use: {
    headless: true,
    viewport: { width: 1280, height: 800 },
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
  },
  // No webServer: the spec auto-skips when /harness.html is
  // not available on the current host. The vitest suite
  // (src/test/easyDevicesOverlap.test.tsx) covers the same
  // contract without a browser.
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
});