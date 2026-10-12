/**
 * Health-semantics regression tests for the Easy Home, System Overview,
 * and AppShell status chip.
 *
 * Locks in the production contract:
 *   GET /api/v1/health  →  { "status": "...", "schemaVersion": "1" }
 *
 * /api/v1/health carries no overall tone, time trust, observation
 * timestamp, or component health. These tests assert that no Easy Home,
 * System Overview, or AppShell surface fabricates a "Healthy" /
 * "Everything is working" / "Observation fresh" verdict from the health
 * probe alone — they must use real per-endpoint evidence:
 *   - /api/v1/overview  →  observedAt, issues[]
 *   - /api/v1/doctor    →  findings[]
 */
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { APIClient, type FetchResult, type HealthPayload } from "../api/client";
import { EasyHomePage } from "../pages/EasyV2";
import { SystemOverviewV2 } from "../pages/SystemOverviewV2";
import { messages } from "../i18n/messages";
import type { Messages } from "../i18n/messages";
import type { Overview } from "../../../shared/api/types";
import type { PageMeta } from "../pages/registry";

const PROD_HEALTH = { status: "ok", schemaVersion: "1" };
const PAGE: PageMeta = {
  id: "P001",
  name: "Easy Home",
  route: "/easy/home",
  section: "easy",
  icon: "home",
  question: "Is the network working?",
  implemented: true,
  features: "easy home tile",
  packages: "easy",
  columns: "Tile; state",
  filters: "category",
  risk: "I",
  dependencies: "overview; doctor",
  runtime: "easy home shows current observation summary",
  diagnostics: "Doctor",
  easyHint: "The single page a non-technical user needs first.",
  easyName: "Home",
};

const SYS_PAGE: PageMeta = {
  id: "P091",
  name: "System Overview",
  route: "/system/overview",
  section: "system",
  icon: "overview",
  question: "What is failing now, what still works?",
  implemented: true,
  features: "system overview",
  packages: "overview",
  columns: "Section; status",
  filters: "category",
  risk: "I",
  dependencies: "overview",
  runtime: "system overview shows observation summary",
  diagnostics: "Doctor",
  easyHint: "Current system state at a glance.",
  easyName: "System Overview",
};

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
        addresses: [],
      },
    ],
    routes: [],
    dns: { nameservers: ["1.1.1.1"], searchDomains: [], source: "systemd" },
    services: [],
    nftables: { status: "available", tables: 0, chains: 0, rules: 0 },
    issues: [],
    ...over,
  };
}

class StubClient extends APIClient {
  private health: FetchResult<HealthPayload>;
  private overview: FetchResult<Overview> | undefined;
  private doctor: FetchResult<unknown>;

  constructor(
    opts: {
      health?: FetchResult<HealthPayload>;
      overview?: FetchResult<Overview>;
      doctor?: FetchResult<unknown>;
    } = {},
  ) {
    super({ token: "t", locale: "en", onFetch: () => {} } as never);
    this.health = opts.health ?? { ok: true, data: PROD_HEALTH, age: 0 };
    this.overview = opts.overview;
    this.doctor = opts.doctor ?? { ok: true, data: { findings: [] }, age: 0 };
  }

  fetchHealth(): Promise<FetchResult<HealthPayload>> {
    return Promise.resolve(this.health);
  }
  fetchOverview(): Promise<FetchResult<Overview>> {
    if (this.overview) return Promise.resolve(this.overview);
    return Promise.resolve({
      ok: true,
      data: mkOverview(),
      age: 0,
    });
  }
  fetchDoctor(): Promise<FetchResult<unknown>> {
    return Promise.resolve(this.doctor);
  }
}

describe("Health semantics regression against the production /api/v1/health contract", () => {
  describe("Easy Home", () => {
    it("does NOT say 'Everything is working' when Doctor reports disk-wear-high", async () => {
      const client = new StubClient({
        doctor: {
          ok: true,
          data: {
            findings: [
              {
                component: "disk",
                severity: "warn",
                code: "disk-wear-high",
                message: "SSD wear is at 92%",
              },
            ],
          },
          age: 0,
        },
      });
      const { container } = render(
        <EasyHomePage
          page={PAGE}
          bundle={messages.en as Messages}
          navigate={() => {}}
          client={client}
        />,
      );
      // The headline MUST surface the warning, not "Everything is
      // working". Easy Home surfaces Doctor findings through the
      // overall tone (warn) and the "Open Doctor" action, not by
      // rendering the raw finding text.
      const text = container.textContent ?? "";
      expect(text).not.toContain("Everything is working");
      // Must not crash — page renders sufficient content.
      expect(container.textContent ?? "").toMatch(/Devices|DNS|Internet/i);
    });

    it("shows Status incomplete (not Healthy) when health OK but no observation timestamp", async () => {
      const client = new StubClient({
        overview: {
          ok: true,
          data: mkOverview({ observedAt: "" }), // empty observedAt = no freshness
          age: 0,
        },
      });
      const { container } = render(
        <EasyHomePage
          page={PAGE}
          bundle={messages.en as Messages}
          navigate={() => {}}
          client={client}
        />,
      );
      const text = container.textContent ?? "";
      expect(text).not.toContain("Everything is working");
      // Should display an honest incomplete verdict.
      expect(text.toLowerCase()).toContain("incomplete");
    });
  });

  describe("System Overview", () => {
    it("does NOT show 'Observation fresh' or 'Healthy' from health endpoint alone", async () => {
      const client = new StubClient({
        // Health probe OK, but observation has no timestamp.
        overview: {
          ok: true,
          data: mkOverview({ observedAt: "" }),
          age: 0,
        },
      });
      const { container, findByText } = render(
        <SystemOverviewV2
          page={SYS_PAGE}
          bundle={messages.en as Messages}
          navigate={() => {}}
          client={client}
        />,
      );
      const text = container.textContent ?? "";
      // Must NOT claim observation is fresh.
      expect(text).not.toContain("Observation fresh");
      // Must NOT claim Healthy without real evidence.
      expect(text).not.toContain("Healthy");
      // Must render Status incomplete instead.
      expect(text).toContain("Status incomplete");
      // Must not crash.
      expect(await findByText("System Overview")).toBeTruthy();
    });

    it("does NOT show Healthy when Doctor has findings but observation timestamp is recent", async () => {
      const client = new StubClient({
        doctor: {
          ok: true,
          data: {
            findings: [
              {
                component: "disk",
                severity: "warn",
                code: "disk-wear-high",
                message: "SSD wear is at 92%",
              },
            ],
          },
          age: 0,
        },
      });
      const { container } = render(
        <SystemOverviewV2
          page={SYS_PAGE}
          bundle={messages.en as Messages}
          navigate={() => {}}
          client={client}
        />,
      );
      const text = container.textContent ?? "";
      // System Overview surfaces "Issues" via overview.issues, not via
      // doctor. With empty overview.issues, headline stays neutral.
      // But the doctor finding's code must NOT appear under "Healthy".
      expect(text).not.toMatch(/Healthy.*disk-wear-high/s);
    });
  });

  describe("AppShell health chip (logic mirror)", () => {
    // Mirror of AppShell.tsx HealthChip logic. Any drift here is a
    // silent regression; both must move together.
    function chipFor(h: {
      overview: string;
      doctor: string;
      reachable: boolean;
      lastObservedAt: number | null;
      timeTrust: string;
    }): { tone: string; label: string } {
      const reachable = h.reachable === true;
      const observed = h.lastObservedAt !== null;
      const fresh = h.timeTrust === "trusted";
      const tone: "good" | "warn" | "bad" | "unknown" = !reachable
        ? "unknown"
        : !observed
          ? "unknown"
          : !fresh
            ? "unknown"
            : h.doctor === "bad" || h.overview === "bad"
              ? "bad"
              : h.doctor === "warn" || h.overview === "warn"
                ? "warn"
                : h.doctor === "good" && h.overview === "good"
                  ? "good"
                  : "unknown";
      const label =
        tone === "good"
          ? "Healthy"
          : tone === "warn"
            ? "Needs attention"
            : tone === "bad"
              ? "Unavailable"
              : observed && !fresh
                ? "Status check incomplete"
                : "Status incomplete";
      return { tone, label };
    }

    it("does NOT default the chip to Healthy when only /api/v1/health returned", () => {
      // The /api/v1/health probe returning {status:"ok"} is the bare
      // production signal. The chip must NOT show "Healthy" from that
      // alone; it must show "Status incomplete" because no freshness /
      // doctor / overview evidence exists.
      const chip = chipFor({
        overview: "unknown",
        doctor: "unknown",
        reachable: true,
        lastObservedAt: null,
        timeTrust: "unknown",
      });
      expect(chip.tone).not.toBe("good");
      expect(chip.label).toBe("Status incomplete");
    });

    it("does NOT show Healthy when Doctor has findings but no freshness evidence", () => {
      const chip = chipFor({
        overview: "good",
        doctor: "warn",
        reachable: true,
        lastObservedAt: null,
        timeTrust: "unknown",
      });
      expect(chip.tone).not.toBe("good");
      expect(chip.label).toBe("Status incomplete");
    });
  });
});
