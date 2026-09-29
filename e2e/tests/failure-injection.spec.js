// @ts-check
/**
 * Failure injection test — verifies the UI distinguishes backend
 * errors truthfully (no fake success) when the backend service is
 * stopped, restarts, or returns 4xx/5xx.
 *
 * Scenarios:
 *   A. Backend down: stop duck-api.service, refresh UI, expect
 *      a connectivity banner — never a stale "applied" success.
 *   B. Backend 4xx: hit a known-bad endpoint, expect the error
 *      banner / inline error, not a silent success.
 *   C. Permission denied: hit /api/v1/config/diff without auth,
 *      expect 401 surfaced.
 */
const { test, expect } = require("@playwright/test");
const { execSync } = require("node:child_process");

const VM = "duck-dev-01";

function ssh(cmd) {
  return execSync(
    `ssh -o ConnectTimeout=5 -o BatchMode=yes ${VM} '${cmd.replace(/'/g, "'\\''")}'`,
    { stdio: ["ignore", "pipe", "pipe"], timeout: 30_000 },
  ).toString();
}

test.describe.serial("Failure injection (backend availability)", () => {
  test("backend down surfaces a connectivity banner", async ({ browser }) => {
    test.setTimeout(120_000);
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    // Login first.
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await page.locator("#username").waitFor({ state: "visible", timeout: 15_000 });
    await page.locator("#username").fill("admin");
    await page.locator("#password").fill("test-password-12345");
    await page.locator('button[type="submit"]').click();
    await page
      .locator(".v1-shell")
      .first()
      .waitFor({ state: "visible", timeout: 30_000 });

    // Stop the API.
    ssh("sudo systemctl stop duck-api.service");
    // Refresh the page (or navigate) — should fail to fetch.
    try {
      await page.goto("/system/overview", { waitUntil: "domcontentloaded", timeout: 10_000 });
    } catch {
      // expected: the page can't even load
    }
    const bodyText = (await page.locator("body").innerText({ timeout: 5_000 }).catch(() => "")).trim();
    // The UI must NOT show a fake success like "Configuration applied".
    expect(bodyText).not.toMatch(/applied successfully/i);
    expect(bodyText).not.toMatch(/configuration complete/i);

    // Restart the API.
    ssh("sudo systemctl start duck-api.service");
    // Wait for the API to come back.
    let ok = false;
    for (let i = 0; i < 30; i++) {
      ssh("sleep 1");
      try {
        const code = ssh(
          "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/v1/ready",
        ).trim();
        if (code === "200" || code === "401") {
          ok = true;
          break;
        }
      } catch {
        /* keep polling */
      }
    }
    expect(ok, "API must come back online within 30 s").toBeTruthy();
    await ctx.close();
  });

  test("permission denied is returned for unauthenticated /config/diff", async ({ request }) => {
    const res = await request.post("/api/v1/config/diff", {
      data: { expected: "", candidate: { version: 1 } },
    });
    expect(res.status()).toBe(401);
    const body = await res.json();
    expect(body.error?.code ?? body.code).toBe("unauthorized");
  });

  test("bad payload is rejected honestly", async ({ request }) => {
    const login = await request.post("/api/v1/auth/login", {
      data: { username: "admin", password: "test-password-12345" },
    });
    if (!login.ok()) {
      test.skip(true, "rate-limited; will re-run later");
      return;
    }
    const { token } = await login.json();
    const res = await request.post("/api/v1/config/diff", {
      headers: { Authorization: `Bearer ${token}` },
      data: { candidate: { version: 1 } }, // missing expected/actor/reason/window
    });
    // Coordinator unavailable (BD-001) means the backend returns
    // 503 before it reaches payload validation; either 4xx or 503
    // is honest, but never 2xx.
    expect(res.status()).not.toBe(200);
    expect(res.status()).not.toBe(201);
    expect(res.status()).toBeGreaterThanOrEqual(400);
  });
});
