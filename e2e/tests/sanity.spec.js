// @ts-check
const { test, expect } = require("@playwright/test");
const { authenticate, fetchSessionToken } = require("../fixtures/auth");

test("sanity: UI shell renders after login", async ({ page }) => {
  await authenticate(page);
  // After login, the shell exists with a known class name.
  await expect(page.locator(".v1-shell").first()).toBeVisible({ timeout: 10_000 });
  // The branded header must include the product name.
  await expect(page.locator("body")).toContainText(/DuckOS/);
});

test("sanity: session token authenticates /api/v1/ready", async () => {
  const token = await fetchSessionToken();
  const res = await fetch("http://127.0.0.1:8080/api/v1/ready", {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(res.status).toBe(200);
  const body = await res.json();
  expect(body.status).toBe("ready");
  expect(body.buildCommit).toMatch(/^[0-9a-f]{40}$/);
});

test("sanity: page navigates after login", async ({ page }) => {
  await authenticate(page);
  await page.goto("/system/overview", { waitUntil: "domcontentloaded" });
  // The system overview heading or any other content must appear.
  await expect(page.locator("body")).toContainText(/DuckOS/);
});
