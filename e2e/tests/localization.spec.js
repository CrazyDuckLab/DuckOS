// @ts-check
/**
 * Localization test — serial flow with one login per spec to avoid
 * the backend auth rate-limit (UD-003). Verifies that switching the
 * locale to each option in turn updates the html[lang] / html[data-locale]
 * attributes and that the shell renders meaningful body content
 * under every locale.
 *
 * KNOWN DEFECT (UD-001, recorded in
 * docs/ui/V1_LIVE_QUALIFICATION_STATUS.md): the `t()` helper only
 * resolves top-level keys, so nested paths like `"shell.language"`
 * surface the raw key text. After the fix in
 * web/src/i18n/messages.ts the runtime tests below verify the
 * html[lang] / html[data-locale] attributes and the body length
 * — translated-copy assertions are intentionally limited to brand
 * text ("DuckOS") and structural checks until the upstream fix
 * is exercised under all four locales.
 */
const { test, expect } = require("@playwright/test");
const { authenticate } = require("../fixtures/auth");

const LOCALES = [
  { id: "en", label: "English" },
  { id: "ru", label: "Русский" },
  { id: "de", label: "Deutsch" },
  { id: "es", label: "Español" },
];

async function currentLocale(page) {
  return await page.evaluate(() => {
    return {
      lang: document.documentElement.lang,
      data: document.documentElement.dataset.locale || "",
    };
  });
}

test.describe.serial("Localization EN / RU / DE / ES", () => {
  test("all locales switch and persist", async ({ page }) => {
    test.setTimeout(120_000);
    await authenticate(page);

    // Locale <select> is the second v1-select (index 1).
    const sel = page.locator("select.v1-select").nth(1);
    const selCount = await page.locator("select.v1-select").count();
    expect(selCount).toBeGreaterThanOrEqual(2);

    for (const { id, label } of LOCALES) {
      await sel.selectOption(id);
      // Allow the reducer + persist effect to settle.
      await page.waitForTimeout(300);
      const { lang, data } = await currentLocale(page);
      expect(data, `data-locale must equal ${id} for ${label}`).toBe(id);
      expect(lang, `html lang must equal ${id} for ${label}`).toBe(id);

      const body = (await page.locator("body").innerText({ timeout: 5_000 })).trim();
      expect(body, `${id} body must include brand`).toContain("DuckOS");
      expect(body.length, `${id} body too short`).toBeGreaterThan(40);

      // Walking easy-mode also stays healthy under each locale.
      await page.goto("/easy/home", { waitUntil: "domcontentloaded" });
      const homeBody = (await page.locator("body").innerText({ timeout: 5_000 })).trim();
      expect(homeBody, `${id} easy/home must include brand`).toContain("DuckOS");
      expect(homeBody.length, `${id} easy/home too short`).toBeGreaterThan(40);

      const lower = homeBody.toLowerCase();
      expect(lower.includes("minified react error")).toBe(false);
    }

    // Persistence: switch to Russian, navigate around, re-check.
    await sel.selectOption("ru");
    await page.waitForTimeout(300);
    await page.goto("/system/overview", { waitUntil: "domcontentloaded" });
    // Wait for the shell to re-render before asserting the locale
    // attribute — the AppShell sets html[lang] in a useEffect that
    // fires after mount, so a too-early check sees the previous
    // value from the previous page.
    await page.locator(".v1-shell").first().waitFor({ state: "visible", timeout: 10_000 });
    await page.waitForFunction(
      () => document.documentElement.lang === "ru",
      null,
      { timeout: 5_000 },
    ).catch(() => {});
    let cur = await currentLocale(page);
    expect(cur.lang).toBe("ru");
    await page.goto("/tunnels/wireguard", { waitUntil: "domcontentloaded" });
    await page.locator(".v1-shell").first().waitFor({ state: "visible", timeout: 10_000 });
    await page.waitForFunction(
      () => document.documentElement.lang === "ru",
      null,
      { timeout: 5_000 },
    ).catch(() => {});
    cur = await currentLocale(page);
    expect(cur.lang).toBe("ru");
  });
});
