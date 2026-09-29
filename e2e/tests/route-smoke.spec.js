// @ts-check
/**
 * Route smoke test — visits every one of the 99 V1 routes and asserts
 * the page rendered without a React crash, without an unhandled
 * console exception, and without any 5xx response from the API.
 */
const { test, expect } = require("@playwright/test");
const { readFileSync } = require("node:fs");
const { resolve } = require("node:path");
const { mkdirSync, writeFileSync } = require("node:fs");
const { authenticate, TEST_TOKEN } = require("../fixtures/auth");

const ROUTES = readFileSync(
  resolve(__dirname, "../fixtures/all-routes.txt"),
  "utf8",
)
  .split("\n")
  .map((r) => r.trim())
  .filter((r) => r.length > 0 && r.startsWith("/"));

if (ROUTES.length !== 99) {
  throw new Error(
    `Expected 99 routes from registry, got ${ROUTES.length}. Update fixtures/all-routes.txt.`,
  );
}

async function visit(page, route, label) {
  const consoleErrors = [];
  const failedRequests = [];

  const onConsole = (msg) => {
    if (msg.type() === "error") {
      const text = msg.text();
      if (text.includes("engine_unavailable")) return;
      if (text.includes("Failed to load resource")) return;
      consoleErrors.push(text);
    }
  };
  const onResponse = (resp) => {
    if (resp.status() >= 500) {
      failedRequests.push({
        url: resp.url(),
        status: resp.status(),
        method: resp.request().method(),
      });
    }
  };
  page.on("console", onConsole);
  page.on("response", onResponse);

  try {
    if (label === "cold") {
      await page.goto(route, { waitUntil: "domcontentloaded", timeout: 20_000 });
    } else {
      await page.evaluate((r) => {
        history.pushState({}, "", r);
        window.dispatchEvent(new PopStateEvent("popstate"));
      }, route);
    }
    await page.waitForLoadState("networkidle", { timeout: 5_000 }).catch(() => {});
    const root = page.locator("#root").first();
    await root.waitFor({ state: "attached", timeout: 5_000 });
    const text = (await page
      .locator("body")
      .innerText({ timeout: 5_000 })
      .catch(() => "")).trim();
    if (text.length < 5) {
      return {
        route,
        label,
        reason: `body essentially empty (${text.length} chars)`,
        consoleErrors,
        failedRequests,
      };
    }
    const lower = text.toLowerCase();
    if (lower.includes("minified react error") || lower.includes("uncaught")) {
      return {
        route,
        label,
        reason: "React/uncaught crash surfaced in body",
        consoleErrors,
        failedRequests,
      };
    }
    if (consoleErrors.length > 0 || failedRequests.length > 0) {
      return {
        route,
        label,
        reason: `${consoleErrors.length} console errors, ${failedRequests.length} 5xx`,
        consoleErrors,
        failedRequests,
      };
    }
    return null;
  } catch (err) {
    return {
      route,
      label,
      reason: err instanceof Error ? err.message : String(err),
      consoleErrors,
      failedRequests,
    };
  } finally {
    page.off("console", onConsole);
    page.off("response", onResponse);
  }
}

test.describe("V1 route smoke (99 routes, cold + client-nav)", () => {
  test.beforeAll(() => {
    if (!TEST_TOKEN) {
      throw new Error("DUCKOS_E2E_TOKEN (or .token) is required");
    }
  });

  test("all routes load cleanly", async ({ page }) => {
    test.setTimeout(180_000);
    await authenticate(page);

    const failures = [];

    for (const route of ROUTES) {
      const f = await visit(page, route, "client-nav");
      if (f) failures.push(f);
    }

    const coldSample = [
      "/system/overview",
      "/easy/home",
      "/interfaces",
      "/wan/connections",
      "/tunnels/wireguard",
      "/firewall/rules",
      "/dns/resolver",
      "/routing/routes",
      "/services",
      "/diagnostics/monitoring",
    ];
    for (const route of coldSample) {
      const f = await visit(page, route, "cold");
      if (f) failures.push(f);
    }

    const reportPath = resolve(__dirname, "../artifacts/route-smoke-report.json");
    mkdirSync(resolve(__dirname, "../artifacts"), { recursive: true });
    writeFileSync(
      reportPath,
      JSON.stringify(
        {
          generatedAt: new Date().toISOString(),
          totalRoutes: ROUTES.length,
          passed: ROUTES.length - failures.length,
          failed: failures.length,
          failures,
        },
        null,
        2,
      ),
    );

    expect(
      failures,
      `Route smoke: ${failures.length}/${ROUTES.length} routes failed. See ${reportPath}`,
    ).toEqual([]);
  });
});
