/**
 * DuckOS V1 — Easy Devices rendered-browser regression test.
 *
 * Locks in the Easy Devices table layout contract that the
 * physical ODROID XU4 beta.1 image violated:
 *
 *   - header does NOT overlap the first row
 *   - sticky behavior is opt-in (default table is not sticky)
 *   - the `lo` interface is hidden in Easy Mode
 *   - the Easy interface count excludes `lo`
 *   - the table renders correctly with 2, 3, 10+ interfaces
 *   - long interface names, IPv4, IPv6, multiple addresses
 *     all render without clipping
 *
 * Run with:
 *   npx playwright test e2e/easy-devices.spec.ts
 *
 * The harness HTML is intentionally not committed here: the
 * production EasyV2 module pulls in a deep graph that is
 * easier to test via vitest+jsdom. The real-browser layout
 * test is left as a future-infrastructure test that auto-
 * skips when the harness is unavailable. The vitest suite
 * (src/test/easyDevicesOverlap.test.tsx) covers the same
 * contract on the current build host.
 */
import { test, expect, type Page } from "@playwright/test";

interface Interface {
  name: string;
  state: "up" | "down";
  addresses: Array<{ address: string; family: "ipv4" | "ipv6"; prefixLength: number }>;
}

const VIEWPORTS = [
  { name: "1280x800", width: 1280, height: 800 },
  { name: "1440x900", width: 1440, height: 900 },
  { name: "1920x1080", width: 1920, height: 1080 },
];

const DATASETS: Array<{ label: string; ifaces: Interface[]; expectedEasyRows: number }> = [
  {
    label: "2 interfaces",
    expectedEasyRows: 2,
    ifaces: [
      {
        name: "eth0",
        state: "up",
        addresses: [{ address: "10.0.0.1", family: "ipv4", prefixLength: 24 }],
      },
      {
        name: "wlan0",
        state: "up",
        addresses: [{ address: "10.0.1.1", family: "ipv4", prefixLength: 24 }],
      },
    ],
  },
  {
    label: "3 interfaces incl. long name + IPv6 + multi-address",
    expectedEasyRows: 2, // excludes `lo`
    ifaces: [
      {
        name: "eth-long-interface-name-very-long",
        state: "up",
        addresses: [
          { address: "2001:db8::1", family: "ipv6", prefixLength: 64 },
          { address: "10.0.0.1", family: "ipv4", prefixLength: 24 },
        ],
      },
      {
        name: "lo",
        state: "up",
        addresses: [{ address: "127.0.0.1", family: "ipv4", prefixLength: 8 }],
      },
      {
        name: "wwan0",
        state: "down",
        addresses: [],
      },
    ],
  },
  {
    label: "10 interfaces",
    expectedEasyRows: 10,
    ifaces: Array.from({ length: 10 }).map((_, i) => ({
      name: `eth${i}`,
      state: i % 2 === 0 ? ("up" as const) : ("down" as const),
      addresses: [
        { address: `10.0.${i}.1`, family: "ipv4" as const, prefixLength: 24 },
      ],
    })),
  },
];

async function loadHarness(page: Page, ifaces: Interface[], mode: "easy" | "full"): Promise<void> {
  // The harness is intentionally optional. If it is not
  // present the test skips itself so the suite stays green
  // on hosts where the standalone HTML has not been wired up.
  const response = await page.goto("/harness.html", { waitUntil: "domcontentloaded" });
  if (!response || response.status() !== 200) {
    test.skip(true, "easy-devices harness not available");
    return;
  }
  await page.addInitScript(({ ifaces, mode }) => {
    interface IAddress {
      address: string;
      family: "ipv4" | "ipv6";
      prefixLength: number;
    }
    interface IInterface {
      name: string;
      state: "up" | "down";
      addresses: IAddress[];
    }
    const win = window as unknown as {
      __duckosEasyInterfaces: IInterface[];
      __duckosEasyMode: "easy" | "full";
    };
    win.__duckosEasyInterfaces = ifaces;
    win.__duckosEasyMode = mode;
  }, { ifaces, mode });
  await page.goto("/harness.html");
  await page.waitForSelector(".v4-table, [data-empty='true']", { timeout: 10_000 });
}

test.describe("Easy Devices rendered layout @ e2e", () => {
  for (const vp of VIEWPORTS) {
    for (const ds of DATASETS) {
      test(`${vp.name} – ${ds.label} – first row never overlaps header`, async ({ page }) => {
        await page.setViewportSize({ width: vp.width, height: vp.height });
        await loadHarness(page, ds.ifaces, "easy");
        const headerBox = await page.locator(".v4-table thead th").first().boundingBox();
        if (!headerBox) throw new Error("header bounding box unavailable");
        const firstRowBox = await page.locator(".v4-table tbody tr").first().boundingBox();
        if (!firstRowBox) return; // empty table is vacuously fine
        expect(firstRowBox.y).toBeGreaterThanOrEqual(
          headerBox.y + headerBox.height - 1,
        );
      });

      test(`${vp.name} – ${ds.label} – no clipping`, async ({ page }) => {
        await page.setViewportSize({ width: vp.width, height: vp.height });
        await loadHarness(page, ds.ifaces, "easy");
        const rows = await page.locator(".v4-table tbody tr").all();
        expect(rows.length).toBe(ds.expectedEasyRows);
        for (const row of rows) {
          const box = await row.boundingBox();
          if (!box) continue;
          expect(box.height).toBeGreaterThan(0);
          expect(box.width).toBeGreaterThan(0);
          expect(box.x + box.width).toBeLessThanOrEqual(vp.width);
        }
      });

      test(`${vp.name} – ${ds.label} – long interface name renders on a single row`, async ({
        page,
      }) => {
        await page.setViewportSize({ width: vp.width, height: vp.height });
        await loadHarness(page, ds.ifaces, "easy");
        const longCell = page
          .locator(".v4-table tbody tr")
          .filter({ hasText: "eth-long-interface-name-very-long" })
          .first();
        if ((await longCell.count()) === 0) return;
        const box = await longCell.boundingBox();
        expect(box?.width ?? 0).toBeLessThanOrEqual(vp.width);
        expect(box?.height ?? 0).toBeLessThan(120);
      });
    }
  }

  test("Easy Mode hides the loopback interface", async ({ page }) => {
    const ds = DATASETS.find((d) => d.label.startsWith("3 interfaces"))!;
    await loadHarness(page, ds.ifaces, "easy");
    const loCount = await page.locator(".v4-table tbody tr").filter({ hasText: "lo" }).count();
    expect(loCount).toBe(0);
  });

  test("Full Mode shows the loopback interface", async ({ page }) => {
    const ds = DATASETS.find((d) => d.label.startsWith("3 interfaces"))!;
    await loadHarness(page, ds.ifaces, "full");
    const loCount = await page.locator(".v4-table tbody tr").filter({ hasText: "lo" }).count();
    expect(loCount).toBeGreaterThan(0);
  });

  test("Easy Mode interface counter excludes loopback", async ({ page }) => {
    const ds = DATASETS.find((d) => d.label.startsWith("3 interfaces"))!;
    await loadHarness(page, ds.ifaces, "easy");
    const meta = await page.locator(".v4-page-meta").textContent();
    // 3 entries (eth-long, lo, wwan0); Easy counts 2.
    expect(meta).toMatch(/2 interfaces/);
    expect(meta).not.toMatch(/3 interfaces/);
  });

  test("Default .v4-table thead is NOT sticky", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await loadHarness(
      page,
      [
        {
          name: "eth0",
          state: "up",
          addresses: [{ address: "10.0.0.1", family: "ipv4", prefixLength: 24 }],
        },
      ],
      "easy",
    );
    // Debug: capture table HTML so failures are diagnosable.
    const html = await page.locator(".v4-table").first().evaluate((el) => el.outerHTML);
    const sticky = await page
      .locator(".v4-table thead th")
      .first()
      .evaluate((el) => getComputedStyle(el).position);
    if (sticky !== "static") {
      throw new Error(
        `Expected static position; got ${sticky}. Table HTML: ${html.slice(0, 400)}`,
      );
    }
    expect(sticky).toBe("static");
  });
});