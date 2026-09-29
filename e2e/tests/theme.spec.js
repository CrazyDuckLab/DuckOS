// @ts-check
/**
 * Theme test — serial flow with one login per spec to avoid the
 * backend auth rate-limit (UD-003). Verifies that switching the
 * theme picker to each option in turn results in a valid
 * data-theme attribute.
 */
const { test, expect } = require("@playwright/test");
const { authenticate } = require("../fixtures/auth");

const THEMES = ["system", "light", "dark"];

test.describe.serial("Theme switching (serial, single login)", () => {
  test("all themes produce a valid data-theme attribute", async ({ page }) => {
    test.setTimeout(60_000);
    await authenticate(page);
    const selects = page.locator("select.v1-select");
    const count = await selects.count();
    expect(count, "expected at least two v1-selects").toBeGreaterThanOrEqual(2);

    for (const theme of THEMES) {
      await selects.nth(0).selectOption(theme);
      // Allow the reducer + theme effect to settle.
      await page.waitForTimeout(300);
      const actual = await page.evaluate(() => {
        const v =
          document.documentElement.dataset.theme ||
          document.body.dataset.theme ||
          "";
        return v;
      });
      expect(
        ["light", "dark", "system"],
        `theme="${theme}" must produce a valid data-theme (got "${actual}")`,
      ).toContain(actual);
    }
  });
});
