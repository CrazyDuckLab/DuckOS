// @ts-check
/**
 * Reboot persistence test — after a managed restart of the API and
 * agent, the UI must come back, the session must still work, and
 * the configured interface inventory must match the runtime state.
 *
 * This is a one-shot VM-side test:
 *   1. snapshot interfaces before restart
 *   2. restart duck-api + duck-agent
 *   3. wait for API to come back
 *   4. login again
 *   5. compare before/after inventory
 *
 * We never restart the host, never touch eth0, never touch the
 * management boundary, and the test VM is disposable.
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

test.describe.serial("Reboot persistence (managed restart)", () => {
  test("API + agent restart preserves runtime state", async ({ browser }) => {
    test.setTimeout(180_000);
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    // Capture the runtime interface inventory before restart.
    const before = ssh("ip -d link show 2>/dev/null | awk -F': ' '/^[0-9]+: /{print $2}' | sort");
    console.log("Before restart, interfaces:\n" + before);
    expect(before.length, "should have captured some interfaces").toBeGreaterThan(5);

    // Restart the API and agent (NOT the host).
    ssh("sudo systemctl restart duck-api.service duck-agent.service");
    // Wait for the API to come back on 127.0.0.1:8080.
    let ready = false;
    for (let i = 0; i < 30; i++) {
      ssh("sleep 1");
      try {
        const code = ssh(
          "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/v1/ready",
        ).trim();
        if (code === "200" || code === "401") {
          ready = true;
          break;
        }
      } catch {
        // keep polling
      }
    }
    expect(ready, "API must come back online within 30 s").toBeTruthy();

    // The lab interfaces must still be there.
    const after = ssh("ip -d link show 2>/dev/null | awk -F': ' '/^[0-9]+: /{print $2}' | sort");
    expect(after, "interface inventory must survive a managed restart").toBe(before);

    // Management boundary is still intact.
    const mgmt = ssh("ip -br addr show eth0 2>/dev/null | head -1").trim();
    expect(mgmt).toContain("172.18.4.149/16");
    const route = ssh("ip route show default 2>/dev/null | head -1").trim();
    expect(route).toContain("172.18.1.254");

    // The UI must log in again and show the dashboard.
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await page.locator("#username").waitFor({ state: "visible", timeout: 15_000 });
    await page.locator("#username").fill("admin");
    await page.locator("#password").fill("test-password-12345");
    await page.locator('button[type="submit"]').click();
    await page
      .locator(".v1-shell")
      .first()
      .waitFor({ state: "visible", timeout: 30_000 });
    const body = (await page.locator("body").innerText({ timeout: 5_000 })).trim();
    expect(body).toContain("DuckOS");

    await ctx.close();
  });
});
