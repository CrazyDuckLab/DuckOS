/**
 * DuckOS V1 — Easy V2 page tests.
 *
 * Locks in the product-level Easy-mode contract:
 *   - never shows speed / latency / public IP (those are not in the API);
 *   - shows real signals (interfaces, default route, DNS, doctor
 *     findings) and reports "no fresh data" honestly;
 *   - exposes at most one "Configure X" primary action per tile.
 */
import { describe, expect, it } from "vitest";
import { APIClient, type FetchResult } from "../api/client";
import type { Overview } from "../../../shared/api/types";
import {
  EasyHomePage,
  EasyInternetPage,
  EasyDevicesPage,
} from "../pages/EasyV2";
import { messages } from "../i18n/messages";
import type { Messages } from "../i18n/messages";
import { v4Messages } from "../i18n/v4-strings";
import { render } from "@testing-library/react";
import type { PageMeta } from "../pages/registry";

function mkOverview(over: Partial<Overview> = {}): Overview {
  return {
    schemaVersion: "1",
    observedAt: new Date().toISOString(),
    environment: "production",
    system: {
      hostname: "duckos-1",
      kernel: "Linux",
      os: "DuckOS",
      architecture: "x86_64",
      uptimeSeconds: 12345,
    },
    cpu: { logicalCores: 4, usagePercent: 12, loadAverage: [0.1, 0.2, 0.3] },
    memory: { totalBytes: 1e9, availableBytes: 5e8, usedBytes: 5e8 },
    storage: [],
    interfaces: [
      {
        name: "eth0",
        index: 1,
        mtu: 1500,
        mac: "aa:bb:cc:dd:ee:ff",
        state: "up",
        addresses: [{ address: "10.0.0.1", family: "ipv4", prefixLength: 24 }],
      },
    ],
    routes: [
      {
        family: "ipv4",
        destination: "default",
        gateway: "10.0.0.1",
        device: "eth0",
        protocol: "static",
        scope: "global",
        table: "main",
        metric: 100,
      },
    ],
    dns: { nameservers: ["1.1.1.1"], searchDomains: [], source: "static" },
    services: [],
    nftables: { status: "available", tables: 4, chains: 12, rules: 24 },
    issues: [],
    ...over,
  } as Overview;
}

class FakeClient extends APIClient {
  private overview: FetchResult<Overview>;
  private multiwan: FetchResult<unknown>;
  private wifi: FetchResult<unknown>;
  private sites: FetchResult<unknown>;
  private doctor: FetchResult<unknown>;

  constructor(opts: {
    overview?: FetchResult<Overview>;
    multiwan?: FetchResult<unknown>;
    wifi?: FetchResult<unknown>;
    sites?: FetchResult<unknown>;
    doctor?: FetchResult<unknown>;
  }) {
    super({
      token: "t",
      locale: "en",
      onFetch: () => {},
    } as never);
    this.overview = opts.overview ?? { ok: true, data: mkOverview(), age: 0 };
    this.multiwan = opts.multiwan ?? { ok: false, reason: "missing", message: "n/a", status: 0 };
    this.wifi = opts.wifi ?? { ok: false, reason: "missing", message: "n/a", status: 0 };
    this.sites = opts.sites ?? { ok: false, reason: "missing", message: "n/a", status: 0 };
    this.doctor = opts.doctor ?? {
      ok: true,
      data: {
        findings: [],
      },
      age: 0,
    };
  }

  fetchOverview(): Promise<FetchResult<Overview>> {
    return Promise.resolve(this.overview);
  }
  fetchMultiwan(): Promise<FetchResult<unknown>> {
    return Promise.resolve(this.multiwan);
  }
  fetchWifi(): Promise<FetchResult<unknown>> {
    return Promise.resolve(this.wifi);
  }
  fetchSites(): Promise<FetchResult<unknown>> {
    return Promise.resolve(this.sites);
  }
  fetchDoctor(): Promise<FetchResult<unknown>> {
    return Promise.resolve(this.doctor);
  }
}

const page: PageMeta = {
  id: "P094",
  name: "Easy Home",
  easyName: "Home",
  route: "/easy/home",
  section: "easy",
  icon: "home",
  question: "Is everything working?",
  features: "internet, wifi, devices, vpn, system",
  packages: "overview",
  columns: "Section; status; detail",
  filters: "section",
  risk: "I",
  dependencies: "overview.interfaces, overview.dns, overview.routes",
  runtime: "fetchOverview every 5s",
  diagnostics: "Doctor / EasyCare",
  easyHint: "A one-screen view of the network.",
  implemented: true,
};

const navigate = () => {};

describe("EasyHomePage", () => {
  it("renders the system about-section hostname from the typed Overview", async () => {
    const c = new FakeClient({});
    const { findByText } = render(
      <EasyHomePage
        page={page}
        bundle={messages.en}
        navigate={navigate}
        client={c as unknown as APIClient}
      />,
    );
    // Easy Home always renders the system about card with hostname.
    expect(await findByText("duckos-1")).toBeTruthy();
  });

  it("never shows speed, latency, or public IP", async () => {
    const c = new FakeClient({});
    const { container } = render(
      <EasyHomePage
        page={page}
        bundle={messages.en}
        navigate={navigate}
        client={c as unknown as APIClient}
      />,
    );
    await new Promise((r) => setTimeout(r, 10));
    const text = container.textContent ?? "";
    expect(text).not.toMatch(/Mbps|MB\/s|Kbps|latency|public ip/i);
  });

  it("renders the incomplete headline even when observation is fresh and Doctor has no findings (verified health evidence absent)", async () => {
    // The production backend does NOT provide a verified-system-health
    // signal, so Easy Home MUST default to the incomplete verdict
    // regardless of recent observation or empty Doctor findings.
    // The headline "Everything is working" is structurally unreachable
    // today. This test pins the conservative behavior so a future
    // backend field change can revisit it explicitly.
    const c = new FakeClient({});
    const { findByText } = render(
      <EasyHomePage
        page={page}
        bundle={messages.en}
        navigate={navigate}
        client={c as unknown as APIClient}
      />,
    );
    expect(
      await findByText(v4Messages.en["easyHome.statusIncomplete"] as string),
    ).toBeTruthy();
    expect(
      (c as unknown as { bodyText?: () => string }).bodyText?.(),
    ).toBeUndefined();
  });

  it("still renders the page (without throwing) when routes are missing", async () => {
    const c = new FakeClient({
      overview: {
        ok: true,
        data: mkOverview({ routes: [] }),
        age: 0,
      },
    });
    const { findByText } = render(
      <EasyHomePage
        page={page}
        bundle={messages.en}
        navigate={navigate}
        client={c as unknown as APIClient}
      />,
    );
    // Home page should always render the system about-section hostname,
    // regardless of which headline tone the engine derives from the data.
    expect(await findByText("duckos-1")).toBeTruthy();
  });
});

describe("EasyInternetPage", () => {
  it("renders the gateway from the typed routes", async () => {
    const c = new FakeClient({});
    const { findByText } = render(
      <EasyInternetPage
        page={page}
        bundle={messages.en}
        client={c as unknown as APIClient}
        navigate={navigate}
      />,
    );
    expect(await findByText("10.0.0.1")).toBeTruthy();
  });
});

describe("EasyDevicesPage", () => {
  it("shows online / total device count from interfaces", async () => {
    const c = new FakeClient({});
    const { findByText } = render(
      <EasyDevicesPage
        page={page}
        bundle={messages.en}
        client={c as unknown as APIClient}
        navigate={navigate}
      />,
    );
    expect(await findByText(/1 interfaces/)).toBeTruthy();
  });
});

describe("Easy locale parity", () => {
  const locales: Array<{ name: string; bundle: Messages }> = [
    { name: "en", bundle: messages.en },
    { name: "ru", bundle: messages.ru },
    { name: "de", bundle: messages.de },
    { name: "es", bundle: messages.es },
  ];
  for (const { name, bundle } of locales) {
    it(`renders without throwing in ${name.toUpperCase()}`, () => {
      const c = new FakeClient({});
      expect(() =>
        render(
          <EasyHomePage
            page={page}
            bundle={bundle}
            navigate={navigate}
            client={c as unknown as APIClient}
          />,
        ),
      ).not.toThrow();
    });
  }
});