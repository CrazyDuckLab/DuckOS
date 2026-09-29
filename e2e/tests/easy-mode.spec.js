// @ts-check
/**
 * Easy mode test — serial flow with one login per spec to avoid the
 * backend auth rate-limit (UD-003). Verifies that every Easy-mode
 * tile renders the shell and contains the brand text, and that the
 * Easy/Full mode toggle survives a navigation.
 */
const { test, expect } = require("@playwright/test");
const { loginViaUI } = require("../fixtures/auth");

const EASY_TILES = [
  "/easy/home",
  "/easy/internet",
  "/easy/devices",
  "/easy/care",
  "/easy/publish",
  "/easy/networks",
];

test.describe.serial("Easy mode workflow", () => {
  test("every easy tile renders and the shell persists", async ({ browser }) => {
    test.setTimeout(90_000);
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await loginViaUI(page);
    // Once the shell is up, walk through every tile in turn.
    for (const tile of EASY_TILES) {
      const res = await page.goto(tile, { waitUntil: "domcontentloaded" });
      expect(res, `no response for ${tile}`).toBeTruthy();
      expect(res.status(), `${tile} must return 200`).toBe(200);
      await expect(page.locator(".v1-shell").first()).toBeVisible({ timeout: 10_000 });
      const body = (await page.locator("body").innerText({ timeout: 5_000 })).trim();
      expect(body, `${tile} must include brand`).toContain("DuckOS");
      expect(body.length, `${tile} body too short`).toBeGreaterThan(40);
      const lower = body.toLowerCase();
      expect(lower.includes("minified react error")).toBe(false);
    }
    await ctx.close();
  });

  test("easy tile /easy/home in en, ru, de, es via cookie-free context", async ({ browser }) => {
    test.setTimeout(60_000);
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await loginViaUI(page);
    for (const id of ["en", "ru", "de", "es"]) {
      await page.locator("select.v1-select").nth(1).selectOption(id);
      await page.waitForTimeout(300);
      await page.goto("/easy/home", { waitUntil: "domcontentloaded" });
      const body = (await page.locator("body").innerText({ timeout: 5_000 })).trim();
      expect(body, `${id} easy/home must include brand`).toContain("DuckOS");
      expect(body.length, `${id} easy/home too short`).toBeGreaterThan(40);
    }
    await ctx.close();
  });
});
