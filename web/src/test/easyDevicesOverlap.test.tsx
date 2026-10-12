/**
 * DuckOS V1 — Easy Devices overlap regression test (jsdom + CSS source).
 *
 * Locks in the layout contract that the physical ODROID XU4
 * beta.1 image violated. The bug was that the default
 * `.v4-table thead th` rule had `position: sticky; top: 56px`
 * combined with `overflow: hidden` on the table itself. On
 * small viewports that combination caused the first
 * `<tbody> <tr>` to render inside / overlapping the header.
 *
 * Two layered assertions:
 *
 *   1. CSS source guard: `.v4-table thead th` MUST NOT set
 *      `position: sticky` or `top: 56px`. The sticky behaviour
 *      is opt-in via `.v4-table-sticky`.
 *
 *   2. jsdom invariant guard: rendered TH elements report
 *      `position: static` in computed style; rendered DOM has
 *      thead before tbody; rows render for 2/3/10 interfaces;
 *      `lo` is hidden in Easy Mode; the Easy counter excludes
 *      `lo`.
 *
 * jsdom does not compute layout, but the structural and
 * computed-style invariants catch the regression at the
 * source level. The actual rendered-browser regression
 * spec lives at e2e/easy-devices.spec.ts.
 */
import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { render, waitFor } from "@testing-library/react";
import type { APIClient, FetchResult } from "../api/client";
import type { Overview } from "../../../shared/api/types";
import { EasyDevicesPage } from "../pages/EasyV2";
import { messages } from "../i18n/messages";
import type { Messages } from "../i18n/messages";

type SimpleIface = {
  name: string;
  state: "up" | "down";
  addresses: Array<{
    address: string;
    family: "ipv4" | "ipv6";
    prefixLength: number;
  }>;
};

function fakeOverview(ifaces: SimpleIface[]): Overview {
  return {
    schemaVersion: "1",
    observedAt: new Date().toISOString(),
    environment: "production",
    system: {
      hostname: "duckos-1",
      kernel: "Linux",
      os: "DuckOS",
      architecture: "x86_64",
      uptimeSeconds: 0,
    },
    cpu: { logicalCores: 1, usagePercent: 0, loadAverage: [0, 0, 0] },
    memory: { totalBytes: 0, availableBytes: 0, usedBytes: 0 },
    storage: [],
    interfaces: ifaces as unknown as Overview["interfaces"],
    routes: [],
    dns: { nameservers: [], searchDomains: [], source: "static" },
    services: [],
    nftables: { status: "available", tables: 0, chains: 0, rules: 0 },
    issues: [],
  };
}

function fakeClient(ov: Overview): APIClient {
  return {
    token: () => "t",
    fetchOverview: () =>
      Promise.resolve({ ok: true, data: ov, age: 0 } as FetchResult<Overview>),
  } as unknown as APIClient;
}

const page = { id: "P094-easy-devices", name: "Devices" } as never;
const bundle = messages.en as Messages;
const navigate = () => undefined;

const CSS_PATH = resolve(__dirname, "../styles/v4-shell.css");

describe("EasyDevicesPage overlap-fix invariants", () => {
  it("CSS source: default .v4-table thead th does NOT set position: sticky", () => {
    const css = readFileSync(CSS_PATH, "utf8");
    // Strip comments so the regression-fence text we wrote in
    // the comment does not match the assertion.
    const stripped = css.replace(/\/\*[\s\S]*?\*\//g, "");
    const defaultIdx = stripped.indexOf(".v4-table thead th");
    expect(defaultIdx).toBeGreaterThanOrEqual(0);
    const slice = stripped.slice(
      defaultIdx,
      stripped.indexOf("}", defaultIdx) + 1,
    );
    expect(slice).not.toMatch(/position:\s*sticky/);
    expect(slice).not.toMatch(/top:\s*56px/);
    // Opt-in rule must exist for tables that genuinely scroll,
    // and it must use the SAME `top: 56px` so it sticks below
    // the topbar.
    expect(stripped).toMatch(
      /\.v4-table-sticky\s+thead\s+th\s*\{[^}]*position:\s*sticky/s,
    );
  });

  it("CSS source: no non-opt-in rule sets position: sticky on .v4-table thead", () => {
    const css = readFileSync(CSS_PATH, "utf8");
    const stripped = css.replace(/\/\*[\s\S]*?\*\//g, "");
    // Find every `.v4-table ... thead th { ... }` rule and
    // ensure none of them set `position: sticky` unless they
    // are inside the opt-in `.v4-table-sticky` selector.
    const re = /([^{}]*)\{[^{}]*\}/g;
    let offending = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(stripped)) !== null) {
      const selector = m[1];
      const body = m[0];
      if (
        body.includes("position: sticky") &&
        /\.v4-table[^-].*thead\s+th/.test(selector) &&
        !selector.includes("v4-table-sticky")
      ) {
        offending += 1;
      }
    }
    expect(offending).toBe(0);
  });

  it("Rendered TH elements have position: static (computed)", async () => {
    const c = fakeClient(
      fakeOverview([
        {
          name: "eth0",
          state: "up",
          addresses: [
            { address: "10.0.0.1", family: "ipv4", prefixLength: 24 },
          ],
        },
      ]),
    );
    const { container } = render(
      <EasyDevicesPage
        page={page}
        bundle={bundle}
        navigate={navigate}
        client={c}
      />,
    );
    await waitFor(() => {
      const ths = container.querySelectorAll(".v4-table thead th");
      expect(ths.length).toBeGreaterThan(0);
    });
    const ths = container.querySelectorAll(".v4-table thead th");
    for (const th of Array.from(ths)) {
      const pos = getComputedStyle(th as Element).position;
      expect(pos).toBe("static");
    }
  });

  it("Rendered table has thead followed by tbody (DOM order)", async () => {
    const c = fakeClient(
      fakeOverview([
        {
          name: "eth0",
          state: "up",
          addresses: [
            { address: "10.0.0.1", family: "ipv4", prefixLength: 24 },
          ],
        },
        {
          name: "wlan0",
          state: "up",
          addresses: [
            { address: "10.0.1.1", family: "ipv4", prefixLength: 24 },
          ],
        },
      ]),
    );
    const { container } = render(
      <EasyDevicesPage
        page={page}
        bundle={bundle}
        navigate={navigate}
        client={c}
      />,
    );
    await waitFor(() => {
      expect(container.querySelector(".v4-table tbody")).not.toBeNull();
    });
    const thead = container.querySelector(".v4-table thead");
    const tbody = container.querySelector(".v4-table tbody");
    expect(thead).not.toBeNull();
    expect(tbody).not.toBeNull();
    expect(
      thead && tbody
        ? (thead.compareDocumentPosition(tbody) as number) &
            Node.DOCUMENT_POSITION_FOLLOWING
        : 0,
    ).toBeTruthy();
  });

  it("Easy Mode hides `lo` and renders long name + IPv6 + multi-address", async () => {
    const c = fakeClient(
      fakeOverview([
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
          addresses: [
            { address: "127.0.0.1", family: "ipv4", prefixLength: 8 },
          ],
        },
        {
          name: "wwan0",
          state: "down",
          addresses: [],
        },
      ]),
    );
    const { container } = render(
      <EasyDevicesPage
        page={page}
        bundle={bundle}
        navigate={navigate}
        client={c}
      />,
    );
    await waitFor(() => {
      const rows = container.querySelectorAll(".v4-table tbody tr");
      expect(rows.length).toBe(2);
    });
    const rows = Array.from(container.querySelectorAll(".v4-table tbody tr"));
    expect(rows.length).toBe(2);
    for (const row of rows) {
      const name = row.querySelector("td")?.textContent?.trim();
      expect(name).not.toBe("lo");
    }
    const names = rows.map((r) => r.querySelector("td")?.textContent?.trim());
    expect(names).toContain("eth-long-interface-name-very-long");
    expect(names).toContain("wwan0");
  });

  it("Easy Mode interface counter excludes loopback", async () => {
    const c = fakeClient(
      fakeOverview([
        {
          name: "eth0",
          state: "up",
          addresses: [
            { address: "10.0.0.1", family: "ipv4", prefixLength: 24 },
          ],
        },
        {
          name: "lo",
          state: "up",
          addresses: [
            { address: "127.0.0.1", family: "ipv4", prefixLength: 8 },
          ],
        },
      ]),
    );
    const { container } = render(
      <EasyDevicesPage
        page={page}
        bundle={bundle}
        navigate={navigate}
        client={c}
      />,
    );
    await waitFor(() => {
      const meta = container.querySelector(".v4-page-meta")?.textContent ?? "";
      expect(meta).toMatch(/1 interfaces/);
    });
  });

  it("10 interfaces render with no row truncation", async () => {
    const ifaces = Array.from({ length: 10 }).map((_, i) => ({
      name: `eth${i}`,
      state: (i % 2 === 0 ? "up" : "down") as "up" | "down",
      addresses: [
        { address: `10.0.${i}.1`, family: "ipv4" as const, prefixLength: 24 },
      ],
    }));
    const c = fakeClient(fakeOverview(ifaces));
    const { container } = render(
      <EasyDevicesPage
        page={page}
        bundle={bundle}
        navigate={navigate}
        client={c}
      />,
    );
    await waitFor(() => {
      const rows = container.querySelectorAll(".v4-table tbody tr");
      expect(rows.length).toBe(10);
    });
  });
});
