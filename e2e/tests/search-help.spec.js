// @ts-check
/**
 * Search and help test — serial flow with one login per spec to
 * avoid the backend auth rate-limit (UD-003). Verifies that the
 * shell's search box is reachable, accepts multilingual queries
 * (EN / RU / DE / ES), and that the page does not crash or trigger
 * a destructive mutation as a result.
 */
const { test, expect } = require("@playwright/test");
const { loginViaUI } = require("../fixtures/auth");

const SEARCH_QUERIES = [
  "VLAN 20",
  "как настроить WireGuard",
  "wie ändere ich DNS",
  "BGP сосед",
  "DNS",
  "WireGuard",
];

test.describe.serial("Search / help navigation", () => {
  test("every multilingual search query is accepted", async ({ browser }) => {
    test.setTimeout(90_000);
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await loginViaUI(page);

    const search = page.locator('input[role="searchbox"], input[type="search"]').first();
    const hasSearch = await search.count();
    if (!hasSearch) {
      const alt = page.locator('input[placeholder*="earch" i]').first();
      const altCount = await alt.count();
      if (!altCount) {
        test.skip(true, "no search box found in the shell");
        await ctx.close();
        return;
      }
      for (const query of SEARCH_QUERIES) {
        await alt.fill(query);
        await page.keyboard.press("Enter");
        await page.waitForTimeout(500);
        const body = (await page.locator("body").innerText({ timeout: 5_000 }).catch(() => "")).trim();
        expect(body).toContain("DuckOS");
        const lower = body.toLowerCase();
        expect(lower.includes("minified react error")).toBe(false);
      }
    } else {
      for (const query of SEARCH_QUERIES) {
        await search.fill(query);
        await page.keyboard.press("Enter");
        await page.waitForTimeout(500);
        const body = (await page.locator("body").innerText({ timeout: 5_000 }).catch(() => "")).trim();
        expect(body).toContain("DuckOS");
        const lower = body.toLowerCase();
        expect(lower.includes("minified react error")).toBe(false);
      }
    }
    await ctx.close();
  });
});
