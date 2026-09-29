// @ts-check
/**
 * Playwright fixtures and helpers for DuckOS V1 qualification.
 *
 * The SPA stores its session token in sessionStorage under the key
 * `duckos.v1.session.v1`. We bootstrap the session by minting a
 * /api/v1/auth/login token via fetch (once per worker), then inject
 * the sessionStorage value into every new page via addInitScript so
 * each test skips the login UI entirely. This also avoids the
 * auth-endpoint rate-limit (UD-003) that would otherwise block a
 * Playwright suite that logs in per-test.
 */
const { test: base, expect } = require("@playwright/test");
const { readFileSync } = require("node:fs");
const { resolve } = require("node:path");

const DUCKOS_TOKEN = (process.env.DUCKOS_E2E_TOKEN ?? "").trim();
const DUCKOS_BASE_URL = (process.env.DUCKOS_E2E_URL ?? "http://127.0.0.1:8080").trim();
const DUCKOS_USERNAME = process.env.DUCKOS_E2E_USERNAME ?? "admin";
const DUCKOS_PASSWORD = process.env.DUCKOS_E2E_PASSWORD ?? "test-password-12345";
const SESSION_KEY = "duckos.v1.session.v1";

function loadTokenFromFile() {
  const file = resolve(__dirname, "../.token");
  try {
    return readFileSync(file, "utf8").trim();
  } catch {
    return "";
  }
}

const API_BOOTSTRAP_TOKEN = DUCKOS_TOKEN || loadTokenFromFile();
const TEST_TOKEN = API_BOOTSTRAP_TOKEN;

async function ensureEnrolled() {
  if (!API_BOOTSTRAP_TOKEN) return;
  const res = await fetch(`${DUCKOS_BASE_URL}/api/v1/auth/enroll`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${API_BOOTSTRAP_TOKEN}`,
    },
    body: JSON.stringify({ username: DUCKOS_USERNAME, password: DUCKOS_PASSWORD }),
  });
  if (!res.ok && res.status !== 409) {
    const text = await res.text();
    throw new Error(
      `Enrollment failed: HTTP ${res.status} ${res.statusText}: ${text}`,
    );
  }
}

async function fetchSessionToken() {
  const res = await fetch(`${DUCKOS_BASE_URL}/api/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: DUCKOS_USERNAME, password: DUCKOS_PASSWORD }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Login failed: HTTP ${res.status} ${res.statusText}: ${text}`);
  }
  const body = await res.json();
  if (!body.token || body.token.length !== 64) {
    throw new Error(`Login response missing token: ${JSON.stringify(body)}`);
  }
  return body.token;
}

/**
 * Inject the session token into sessionStorage before any page
 * navigation. Used to bypass the login screen in tests that just
 * need an authenticated page.
 */
async function injectSession(context, token) {
  await context.addInitScript(
    ([key, value]) => {
      try {
        window.sessionStorage.setItem(key, value);
      } catch {
        /* sessionStorage may be disabled in private mode */
      }
    },
    [SESSION_KEY, token],
  );
}

/**
 * Drives the real login UI for tests that want to exercise the
 * login form itself (UD-001 / UD-002 regression).
 */
async function loginViaUI(page) {
  await page.goto("/", { waitUntil: "domcontentloaded" });
  await page.locator("#username").waitFor({ state: "visible", timeout: 15_000 });
  await page.locator("#username").fill(DUCKOS_USERNAME);
  await page.locator("#password").fill(DUCKOS_PASSWORD);
  await page.locator('button[type="submit"]').click();
  await page
    .locator(".v1-shell")
    .first()
    .waitFor({ state: "visible", timeout: 15_000 });
}

const test = base.extend({
  apiToken: async ({}, use) => {
    await ensureEnrolled();
    const token = await fetchSessionToken();
    await use(token);
  },
  authenticatedPage: async ({ browser, page: _page }, use) => {
    // Mint a session token via API (one login per worker). We do
    // this from the fixture rather than from each test so the
    // /api/v1/auth/login rate-limit (UD-003) stays out of the test
    // path.
    await ensureEnrolled();
    const token = await fetchSessionToken();
    // Create an isolated context so the addInitScript only affects
    // this page's session.
    const context = await browser.newContext();
    await injectSession(context, token);
    const page = await context.newPage();
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await page
      .locator(".v1-shell")
      .first()
      .waitFor({ state: "visible", timeout: 15_000 });
    await use(page);
    await context.close();
  },
});

module.exports = {
  test,
  expect,
  // Back-compat alias for older tests.
  authenticate: loginViaUI,
  loginViaUI,
  injectSession,
  fetchSessionToken,
  ensureEnrolled,
  TEST_TOKEN,
  API_BOOTSTRAP_TOKEN,
  DUCKOS_BASE_URL,
  DUCKOS_USERNAME,
  DUCKOS_PASSWORD,
  SESSION_KEY,
};
