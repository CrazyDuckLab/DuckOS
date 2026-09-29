// @ts-check
/**
 * Configurator UI tests.
 *
 * The agent's verified coordinator is not connected on the
 * qualification VM (BD-001 in
 * docs/ui/V1_LIVE_QUALIFICATION_STATUS.md). Every Safe Apply attempt
 * therefore returns 503 config_runtime_unavailable. The UI's
 * contract is to surface that failure truthfully — never fake
 * success — and let the operator re-edit or discard.
 *
 * Serial flow with one login per spec to avoid the backend auth
 * rate-limit (UD-003).
 */
const { test, expect } = require("@playwright/test");
const { loginViaUI } = require("../fixtures/auth");

const CONFIGURATOR_ROUTES = [
  { route: "/interfaces", label: "Interfaces" },
  { route: "/wan/connections", label: "WAN Connections" },
  { route: "/dns/resolver", label: "DNS Resolver" },
  { route: "/firewall/rules", label: "Firewall Rules" },
  { route: "/tunnels/wireguard", label: "WireGuard" },
  { route: "/routing/routes", label: "Routes" },
  { route: "/services", label: "Services" },
  { route: "/diagnostics/monitoring", label: "Monitoring" },
];

test.describe.serial("Configurator UI surfaces", () => {
  test("every configurator surface renders without crashing", async ({ browser }) => {
    test.setTimeout(120_000);
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await loginViaUI(page);

    for (const { route } of CONFIGURATOR_ROUTES) {
      const res = await page.goto(route, { waitUntil: "domcontentloaded" });
      expect(res, `no response for ${route}`).toBeTruthy();
      await expect(page.locator(".v1-shell").first()).toBeVisible({ timeout: 10_000 });
      const body = (await page.locator("body").innerText({ timeout: 5_000 })).trim();
      expect(body.length, `${route} body should have content`).toBeGreaterThan(20);
      const lower = body.toLowerCase();
      expect(lower.includes("minified react error")).toBe(false);
      expect(lower.includes("uncaught error")).toBe(false);
    }
    await ctx.close();
  });
});

test.describe.serial("Safe Apply UI error honesty", () => {
  test("API surfaces 503 truthfully for /api/v1/config/diff", async ({ request }) => {
    const login = await request.post("/api/v1/auth/login", {
      data: { username: "admin", password: "test-password-12345" },
    });
    expect(login.ok()).toBeTruthy();
    const { token } = await login.json();
    const diff = await request.post("/api/v1/config/diff", {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        expected: "",
        candidate: { version: 1 },
        actor: "qa",
        reason: "qualification test",
        window: 60,
      },
    });
    // Backend currently returns 503 because the verified coordinator
    // is not connected (BD-001). The UI must surface this truthfully.
    expect(diff.status()).toBe(503);
    const body = await diff.json();
    expect(body.error?.code ?? body.code).toMatch(
      /config_runtime_unavailable|unauthorized/,
    );
  });
});
