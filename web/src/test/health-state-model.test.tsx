/**
 * DuckOS V1 — Centralized health-state model regression matrix.
 *
 * This test pins the 9-scenario matrix that the centralized
 * derivation in `web/src/state/health.ts` must satisfy. It also
 * asserts on the externally-visible semantics of Easy Home, Easy
 * Care, System Overview, and the AppShell topbar chip — these are
 * the four surfaces that previously fabricated positive health
 * claims from minimal per-endpoint evidence.
 *
 * The matrix:
 *   A. health OK + Doctor warning + recent observedAt
 *        → attention (verdict), warn (chip tone)
 *   B. health OK + Doctor empty + missing observedAt
 *        → incomplete
 *   C. health OK + Doctor empty + stale observedAt
 *        → stale/incomplete
 *   D. health OK + Doctor empty + recent observedAt
 *        → recent observation may show, global remains incomplete
 *   E. health request fails
 *        → API unreachable, never Healthy
 *   F. Doctor request fails + health OK + recent observedAt
 *        → Doctor unavailable / incomplete
 *   G. future observedAt (no skew allowance)
 *        → NOT fresh (stale)
 *   H. invalid observedAt
 *        → incomplete / unknown
 *   I. Doctor warning + missing observedAt
 *        → attention, finding remains visible
 *
 * Important: "Healthy" / "verified healthy" is intentionally NOT in
 * the verdict enum. Today no production endpoint provides verified
 * health evidence, so the global Healthy claim is structurally
 * unreachable. The chip must NEVER show "Healthy" for any of these
 * scenarios.
 */
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { APIClient, type FetchResult, type HealthPayload } from "../api/client";
import type { Overview } from "../../../shared/api/types";
import { EasyHomePage, EasyCarePage } from "../pages/EasyV2";
import { SystemOverviewV2 } from "../pages/SystemOverviewV2";
import { HealthChip } from "../shell/AppShell";
import { messages } from "../i18n/messages";
import type { Messages } from "../i18n/messages";
import type { PageMeta } from "../pages/registry";
import { deriveHealth, type HealthSources } from "../state/health";

const PROD_HEALTH = { status: "ok", schemaVersion: "1" } as HealthPayload;

const EASY_PAGE: PageMeta = {
  id: "P094",
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

const CARE_PAGE: PageMeta = {
  id: "P099",
  name: "Easy Care / Recovery",
  route: "/easy/care",
  section: "easy",
  icon: "care",
  question: "What can you do to keep the system healthy?",
  implemented: true,
  features: "care tile; recovery tile",
  packages: "easy",
  columns: "Action; recommendation",
  filters: "category",
  risk: "C",
  dependencies: "agent.backup; agent.upgrade",
  runtime: "recommendation per the last doctor run",
  diagnostics: "Doctor",
  easyHint: "Backups, updates, factory reset — with guard rails.",
  easyName: "Care / Recovery",
};

const SYSTEM_PAGE: PageMeta = {
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
    observedAt: new Date(Date.now() - 30 * 1000).toISOString(),
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
  constructor(
    private opts: {
      health?: FetchResult<HealthPayload>;
      overview?: FetchResult<Overview>;
      doctor?: FetchResult<unknown>;
    },
  ) {
    super({ token: "t", locale: "en", onFetch: () => {} } as never);
  }
  fetchHealth(): Promise<FetchResult<HealthPayload>> {
    return Promise.resolve<FetchResult<HealthPayload>>(
      this.opts.health ?? { ok: true, data: PROD_HEALTH, age: 0 },
    );
  }
  fetchOverview(): Promise<FetchResult<Overview>> {
    return Promise.resolve<FetchResult<Overview>>(
      this.opts.overview ?? { ok: true, data: mkOverview(), age: 0 },
    );
  }
  fetchDoctor(): Promise<FetchResult<unknown>> {
    return Promise.resolve<FetchResult<unknown>>(
      this.opts.doctor ?? { ok: true, data: { findings: [] }, age: 0 },
    );
  }
}

/**
 * Test fixture that lets a test hold a Doctor / health / overview
 * fetch genuinely pending and resolve it on demand. This is how
 * we test the LOADING state of the Easy Care pages WITHOUT racing
 * the microtask queue — a plain `Promise.resolve(...)` resolves
 * on the next microtask, which is too fast to distinguish from
 * "already on screen" when using `findByText`. A pending promise
 * keeps the page in its initial loading render until the test
 * explicitly resolves it.
 */
class ControlledClient extends APIClient {
  public doctorPending: Promise<FetchResult<unknown>> | null = null;
  public doctorResolve: ((v: FetchResult<unknown>) => void) | null = null;
  public doctorReject: ((e: unknown) => void) | null = null;
  constructor() {
    super({ token: "t", locale: "en", onFetch: () => {} } as never);
  }
  fetchHealth(): Promise<FetchResult<HealthPayload>> {
    return Promise.resolve<FetchResult<HealthPayload>>({
      ok: true,
      data: PROD_HEALTH,
      age: 0,
    });
  }
  fetchOverview(): Promise<FetchResult<Overview>> {
    return Promise.resolve<FetchResult<Overview>>({
      ok: true,
      data: mkOverview(),
      age: 0,
    });
  }
  fetchDoctor(): Promise<FetchResult<unknown>> {
    if (!this.doctorPending) {
      this.doctorPending = new Promise<FetchResult<unknown>>((res, rej) => {
        this.doctorResolve = res;
        this.doctorReject = rej;
      });
    }
    return this.doctorPending;
  }
  /** Resolve the pending Doctor promise with a value. */
  resolveDoctor(v: FetchResult<unknown>) {
    if (!this.doctorResolve) {
      throw new Error("resolveDoctor called without a pending request");
    }
    this.doctorResolve(v);
  }
  /** Reject the pending Doctor promise with a reason. */
  rejectDoctor(e: unknown) {
    if (!this.doctorReject) {
      throw new Error("rejectDoctor called without a pending request");
    }
    this.doctorReject(e);
  }
}

/* — Centralized derivation matrix — */

describe("Centralized health derivation matrix (9 scenarios A–I)", () => {
  const now = new Date("2025-01-01T00:00:00Z").getTime();

  function sources(opts: {
    health?: FetchResult<HealthPayload>;
    overview?: FetchResult<Overview>;
    doctor?: FetchResult<unknown>;
  }): HealthSources {
    return {
      health: opts.health ?? { ok: true, data: PROD_HEALTH, age: 0 },
      overview: opts.overview ?? {
        ok: true,
        data: mkOverview({
          observedAt: new Date(now - 30 * 1000).toISOString(),
        }),
        age: 0,
      },
      doctor: opts.doctor ?? { ok: true, data: { findings: [] }, age: 0 },
    };
  }

  it("A: health OK + Doctor warning + recent observedAt → attention (NEVER Healthy)", () => {
    const d = deriveHealth(
      sources({
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
      }),
      now,
    );
    expect(d.overall).toBe("attention");
    expect(d.api).toBe("reachable");
    expect(d.doctor).toBe("success");
    expect(d.observation).toBe("recent");
    expect(d.doctorFindings).not.toBeNull();
    expect(d.doctorFindings?.length).toBe(1);
  });

  it("B: health OK + Doctor empty + missing observedAt → incomplete", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({ observedAt: "" }),
          age: 0,
        },
      }),
      now,
    );
    expect(d.overall).toBe("incomplete");
    expect(d.observation).toBe("missing");
  });

  it("C: health OK + Doctor empty + stale observedAt → stale/incomplete (NEVER Healthy)", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({
            observedAt: new Date(now - 10 * 60 * 1000).toISOString(),
          }),
          age: 0,
        },
      }),
      now,
    );
    expect(["stale", "incomplete"]).toContain(d.overall);
    expect(d.observation).toBe("stale");
  });

  it("D: health OK + Doctor empty + recent observedAt → recent observation may show, global stays incomplete", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({
            observedAt: new Date(now - 30 * 1000).toISOString(),
          }),
          age: 0,
        },
      }),
      now,
    );
    expect(d.overall).toBe("incomplete");
    expect(d.observation).toBe("recent");
    // "Healthy" must not be reachable from this combination.
    expect(d.overall).not.toBe("good" as never);
  });

  it("E: health request fails → API unreachable (NEVER Healthy)", () => {
    const d = deriveHealth(
      sources({
        health: {
          ok: false,
          reason: "network",
          message: "down",
          status: 0,
        },
      }),
      now,
    );
    expect(d.overall).toBe("unreachable");
    expect(d.api).toBe("unreachable");
    expect(d.apiReachable).toBe(false);
  });

  it("F: Doctor request fails + health OK + recent observedAt → Doctor unavailable / incomplete (NEVER Healthy)", () => {
    const d = deriveHealth(
      sources({
        doctor: {
          ok: false,
          reason: "network",
          message: "Doctor down",
          status: 0,
        },
      }),
      now,
    );
    expect(d.overall).toBe("incomplete");
    expect(d.doctor).toBe("error");
    expect(d.doctorFindings).toBeNull();
  });

  it("G: future observedAt (no skew allowance) → NOT fresh (stale)", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({
            observedAt: new Date(now + 60 * 60 * 1000).toISOString(),
          }),
          age: 0,
        },
      }),
      now,
    );
    expect(d.observation).toBe("future");
    expect(d.overall).not.toBe("good" as never);
    // Either stale or incomplete — both honest. We assert "future".
    expect(["future", "stale"]).toContain(d.observation);
  });

  it("G2: future observedAt within skew tolerance (3s) is allowed as recent", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({
            observedAt: new Date(now + 3 * 1000).toISOString(),
          }),
          age: 0,
        },
      }),
      now,
    );
    expect(d.observation).toBe("recent");
  });

  it("H: invalid observedAt → incomplete/unknown", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({ observedAt: "not-a-date" as unknown as string }),
          age: 0,
        },
      }),
      now,
    );
    expect(d.observation).toBe("invalid");
    expect(d.overall).toBe("incomplete");
  });

  it("I: Doctor warning + missing observedAt → attention (finding remains visible)", () => {
    const d = deriveHealth(
      sources({
        overview: {
          ok: true,
          data: mkOverview({ observedAt: "" }),
          age: 0,
        },
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
      }),
      now,
    );
    expect(d.overall).toBe("attention");
    expect(d.doctorFindings?.length).toBe(1);
    expect(d.observation).toBe("missing");
  });
});

/* — Externally-visible surface semantics — */

describe("Surfaces consume the centralized derivation (no page-local optimistic logic)", () => {
  it("Easy Home does NOT say 'Everything is working' on Doctor warning + recent observation (A)", async () => {
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
    const { container, findAllByText } = render(
      <EasyHomePage
        page={EASY_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // Wait for the doctor fetch to settle. The page must surface
    // the attention verdict in both the headline and the System tile.
    expect(
      (await findAllByText("Some things need attention")).length,
    ).toBeGreaterThan(0);
    const text = container.textContent ?? "";
    expect(text).not.toContain("Everything is working");
  });

  it("Easy Home stays incomplete (B: missing observedAt) — NOT Healthy", async () => {
    const client = new StubClient({
      overview: { ok: true, data: mkOverview({ observedAt: "" }), age: 0 },
    });
    const { container, findAllByText } = render(
      <EasyHomePage
        page={EASY_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // Wait for async overview fetch to settle. The headline h2 and
    // the System tile both show the incomplete verdict.
    expect(
      (await findAllByText("Status check incomplete")).length,
    ).toBeGreaterThan(0);
    const text = container.textContent ?? "";
    expect(text).not.toContain("Everything is working");
    expect(text.toLowerCase()).toContain("incomplete");
  });

  it("Easy Care distinguishes Doctor success-empty from Doctor error", async () => {
    const clientOK = new StubClient({
      doctor: { ok: true, data: { findings: [] }, age: 0 },
    });
    const { findByText: f1, container: c1 } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={clientOK}
      />,
    );
    expect(await f1("No current Doctor findings")).toBeTruthy();
    expect(c1.textContent ?? "").not.toContain("All systems healthy");
    expect(c1.textContent ?? "").not.toContain("Doctor status unavailable");

    const clientDown = new StubClient({
      doctor: {
        ok: false,
        reason: "network",
        message: "down",
        status: 0,
      },
    });
    const { findByText: f2, container: c2 } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={clientDown}
      />,
    );
    expect(await f2("Doctor status unavailable")).toBeTruthy();
    expect(c2.textContent ?? "").not.toContain("All systems healthy");
    expect(c2.textContent ?? "").not.toContain("No current Doctor findings");
  });

  it("System Overview never claims Healthy/Observation fresh without recent valid observedAt", async () => {
    const client = new StubClient({
      overview: { ok: true, data: mkOverview({ observedAt: "" }), age: 0 },
    });
    const { container, findByText } = render(
      <SystemOverviewV2
        page={SYSTEM_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // Wait for async fetch to settle before asserting on the headline.
    expect(await findByText("Status incomplete")).toBeTruthy();
    const text = container.textContent ?? "";
    expect(text).not.toContain("Healthy");
    expect(text).not.toContain("Observation fresh");
  });

  it("AppShell topbar chip shows Status incomplete (not Healthy) on bare /api/v1/health OK with no other evidence", async () => {
    const client = new StubClient({});
    // We render HealthChip directly with the same shape AppShell uses.
    const { findByText } = render(
      <HealthChip
        locale="en"
        health={await client.fetchHealth()}
        doctor={await client.fetchDoctor()}
        overview={await client.fetchOverview()}
      />,
    );
    expect(await findByText("Status incomplete")).toBeTruthy();
  });

  it("AppShell topbar chip shows Needs attention on Doctor warning", async () => {
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
    const { findByText } = render(
      <HealthChip
        locale="en"
        health={await client.fetchHealth()}
        doctor={await client.fetchDoctor()}
        overview={await client.fetchOverview()}
      />,
    );
    expect(await findByText("Needs attention")).toBeTruthy();
  });

  it("AppShell topbar chip shows Unavailable when health request fails", async () => {
    const client = new StubClient({
      health: {
        ok: false,
        reason: "network",
        message: "down",
        status: 0,
      },
    });
    const { findByText } = render(
      <HealthChip
        locale="en"
        health={await client.fetchHealth()}
        doctor={await client.fetchDoctor()}
        overview={await client.fetchOverview()}
      />,
    );
    expect(await findByText("Unavailable")).toBeTruthy();
  });
});

/* — Final freeze micro-fix regression matrix — */

describe("Health freeze: no local positive derivations", () => {
  it("health OK + Doctor success + recent observedAt → global Status incomplete; System tile MUST NOT say 'Everything is working'; Internet tile MUST NOT claim Internet is working", async () => {
    // The "happy path" today: every per-endpoint signal looks fine.
    // No production API provides verified system health, so the
    // global verdict MUST remain "incomplete". The System tile and
    // the Internet tile MUST NOT promote that to a positive claim.
    const client = new StubClient({});
    const { container, findAllByText } = render(
      <EasyHomePage
        page={EASY_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // Wait for the async fetches to settle.
    expect(
      (await findAllByText("Status check incomplete")).length,
    ).toBeGreaterThan(0);
    const text = container.textContent ?? "";
    // Mandatory regression: the System tile MUST NOT say "Everything
    // is working" under any combination of healthy per-signal inputs.
    expect(text).not.toContain("Everything is working");
    // Mandatory regression: the Internet tile MUST NOT claim
    // "Internet is working" without an explicit production API
    // that proves Internet connectivity. Today none exists.
    expect(text).not.toMatch(/Internet is working/);
  });

  it("Easy Home System tile is never 'good' in any tested scenario", async () => {
    // Walk three input combos that used to drive a positive tile:
    //   (a) healthy per-signal bag
    //   (b) Doctor findings present
    //   (c) API unreachable
    // None of them may yield "Everything is working" on the
    // System tile.
    const scenarios: Array<{
      label: string;
      opts: {
        health?: FetchResult<HealthPayload>;
        overview?: FetchResult<Overview>;
        doctor?: FetchResult<unknown>;
      };
    }> = [
      { label: "happy", opts: {} },
      {
        label: "doctor-warning",
        opts: {
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
          } as FetchResult<unknown>,
        },
      },
      {
        label: "api-unreachable",
        opts: {
          health: {
            ok: false,
            reason: "network",
            message: "down",
            status: 0,
          } as FetchResult<HealthPayload>,
        },
      },
    ];
    for (const s of scenarios) {
      const client = new StubClient(s.opts);
      const { container } = render(
        <EasyHomePage
          page={EASY_PAGE}
          bundle={messages.en as Messages}
          navigate={() => {}}
          client={client}
        />,
      );
      // Allow microtasks to settle.
      await new Promise((r) => setTimeout(r, 30));
      const text = container.textContent ?? "";
      expect(text, `scenario=${s.label}`).not.toContain(
        "Everything is working",
      );
    }
  });

  it("Easy Home Internet tile is never 'good' in any tested scenario", async () => {
    const scenarios: Array<{
      label: string;
      opts: {
        health?: FetchResult<HealthPayload>;
        overview?: FetchResult<Overview>;
        doctor?: FetchResult<unknown>;
      };
    }> = [
      { label: "happy", opts: {} },
      {
        label: "doctor-warning",
        opts: {
          doctor: {
            ok: true,
            data: {
              findings: [
                {
                  component: "dns",
                  severity: "warn",
                  code: "resolver-down",
                  message: "resolver is unreachable",
                },
              ],
            },
            age: 0,
          } as FetchResult<unknown>,
        },
      },
      {
        label: "missing-observation",
        opts: {
          overview: {
            ok: true,
            data: mkOverview({ observedAt: "" }),
            age: 0,
          } as FetchResult<Overview>,
        },
      },
    ];
    for (const s of scenarios) {
      const client = new StubClient(s.opts);
      const { container } = render(
        <EasyHomePage
          page={EASY_PAGE}
          bundle={messages.en as Messages}
          navigate={() => {}}
          client={client}
        />,
      );
      await new Promise((r) => setTimeout(r, 30));
      const text = container.textContent ?? "";
      // The Internet tile MUST NOT claim connectivity was proven —
      // "Internet reachable" is acceptable (it states reachability,
      // not verified connectivity); any phrasing that asserts
      // Internet works without real evidence is rejected.
      expect(text, `scenario=${s.label}`).not.toMatch(/Internet is working/);
      expect(text, `scenario=${s.label}`).not.toMatch(/Connectivity verified/);
      expect(text, `scenario=${s.label}`).not.toMatch(/Internet works/);
    }
  });

  it("A: REAL LOADING — Doctor request genuinely pending → 'Checking Doctor…' visible, '0 FINDINGS' NOT visible", async () => {
    // Use ControlledClient so the Doctor promise stays pending
    // until WE resolve it. With a plain `Promise.resolve()` the
    // page would re-render into the empty state on the next
    // microtask and `findByText` would always see the resolved
    // text — that doesn't prove the loading render.
    const client = new ControlledClient();
    const { container } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // Yield once so React commits the initial render with the
    // pending state. We then inspect the DOM BEFORE resolving.
    await Promise.resolve();
    const loadingText = container.textContent ?? "";
    expect(loadingText).toContain("Checking Doctor…");
    // The numeric findings count line MUST NOT be present while
    // Doctor is loading. A "0" cannot legitimately represent a
    // Doctor response we never received.
    expect(loadingText).not.toContain("0 findings");
    expect(loadingText).not.toContain("No current Doctor findings");
    expect(loadingText).not.toContain("Doctor status unavailable");
    expect(loadingText).not.toContain("All systems healthy");
  });

  it("A2: REAL LOADING — after Doctor resolves with [] → loading disappears, '0 findings' becomes visible", async () => {
    const client = new ControlledClient();
    const { container, findByText } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    await Promise.resolve();
    // Confirm we ARE in the loading state.
    expect(container.textContent ?? "").toContain("Checking Doctor…");

    // Resolve Doctor with an explicit empty array.
    client.resolveDoctor({ ok: true, data: { findings: [] }, age: 0 });

    // Wait for the empty verdict to appear.
    expect(await findByText("No current Doctor findings")).toBeTruthy();
    const settled = container.textContent ?? "";
    // The count line ("0 findings") MUST now be reachable because
    // Doctor explicitly returned zero findings.
    expect(settled).toContain("0 findings");
    expect(settled).not.toContain("Checking Doctor…");
    expect(settled).not.toContain("Doctor status unavailable");
  });

  it("B: REAL ERROR — Doctor request rejects → 'Doctor status unavailable', '0 findings' NOT visible", async () => {
    const client = new ControlledClient();
    const { container, findByText } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    await Promise.resolve();
    // Confirm we ARE in the loading state first.
    expect(container.textContent ?? "").toContain("Checking Doctor…");

    // Reject Doctor. The FetchResult machinery in
    // web/src/api/client.ts catches and surfaces this as
    // `result.ok === false`; we use the same shape.
    client.resolveDoctor({
      ok: false,
      reason: "network",
      message: "down",
      status: 0,
    });

    expect(await findByText("Doctor status unavailable")).toBeTruthy();
    const settled = container.textContent ?? "";
    // Loading must have left, error verdict must be present.
    expect(settled).not.toContain("Checking Doctor…");
    // The numeric count line MUST NOT be reachable while Doctor
    // is in error state — same rule as loading.
    expect(settled).not.toContain("0 findings");
    expect(settled).not.toContain("No current Doctor findings");
    expect(settled).not.toContain("All systems healthy");
  });

  it("C: SUCCESS EMPTY — Doctor resolves with [] → '0 findings' visible + 'No current Doctor findings'", async () => {
    const client = new ControlledClient();
    const { container, findByText } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    await Promise.resolve();
    expect(container.textContent ?? "").toContain("Checking Doctor…");

    client.resolveDoctor({ ok: true, data: { findings: [] }, age: 0 });

    expect(await findByText("No current Doctor findings")).toBeTruthy();
    const settled = container.textContent ?? "";
    expect(settled).toContain("0 findings");
    expect(settled).not.toContain("Checking Doctor…");
    expect(settled).not.toContain("Doctor status unavailable");
  });

  it("D: SUCCESS WITH FINDINGS — Doctor resolves with one finding → correct count + finding rendered", async () => {
    const client = new ControlledClient();
    const { container, findByText } = render(
      <EasyCarePage
        page={CARE_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    await Promise.resolve();
    expect(container.textContent ?? "").toContain("Checking Doctor…");

    client.resolveDoctor({
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
    });

    // Finding code must be rendered (it appears inside the
    // findings card as "disk · disk-wear-high").
    expect(await findByText(/disk-wear-high/)).toBeTruthy();
    const settled = container.textContent ?? "";
    // Count must be exactly 1.
    expect(settled).toContain("1 findings");
    expect(settled).not.toContain("Checking Doctor…");
    expect(settled).not.toContain("Doctor status unavailable");
  });

  it("System Overview freshness is independent of Doctor findings", async () => {
    // Even when Doctor has findings, a recent observation timestamp
    // is still "Observation fresh". Findings affect the attention
    // verdict but NOT whether the timestamp is fresh.
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
    const { findAllByText } = render(
      <SystemOverviewV2
        page={SYSTEM_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // "Observation fresh" must appear regardless of Doctor findings.
    expect((await findAllByText("Observation fresh")).length).toBeGreaterThan(
      0,
    );
  });

  it("System Overview freshness is unchanged when Doctor request errors", async () => {
    // Recent observation + Doctor error → observation timestamp
    // is still recent. Doctor failure does NOT redefine freshness.
    const client = new StubClient({
      doctor: {
        ok: false,
        reason: "network",
        message: "down",
        status: 0,
      },
    });
    const { findAllByText } = render(
      <SystemOverviewV2
        page={SYSTEM_PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    expect((await findAllByText("Observation fresh")).length).toBeGreaterThan(
      0,
    );
  });
});

/* — Source-check guard — */

describe("Source-check: no optimistic/fabricated health patterns remain in production code", () => {
  it("no source file uses `health.data.components.some()` (the original Easy Care crash)", async () => {
    const fs = await import("node:fs/promises");
    const path = await import("node:path");
    const root = path.resolve(__dirname, "..");
    const sources = await fs.readdir(root, { withFileTypes: true });
    const targets: string[] = [];
    for (const entry of sources) {
      if (entry.isFile() && /\.(ts|tsx)$/.test(entry.name)) {
        targets.push(path.join(root, entry.name));
      }
    }
    let total = 0;
    for (const t of targets) {
      const txt = await fs.readFile(t, "utf-8");
      if (/\.components\.|health\.data\.components/.test(txt)) total += 1;
    }
    expect(total).toBe(0);
  });

  it("no production source file independently derives a positive System / Internet state", async () => {
    const fs = await import("node:fs/promises");
    const path = await import("node:path");
    const root = path.resolve(__dirname, "..");

    async function* walk(dir: string): AsyncGenerator<string> {
      const entries = await fs.readdir(dir, { withFileTypes: true });
      for (const e of entries) {
        const p = path.join(dir, e.name);
        // Skip the test directory — we only audit production
        // source files. Test files legitimately reference the
        // patterns we're guarding against.
        if (e.isDirectory()) {
          if (e.name === "test" || e.name === "__tests__") continue;
          yield* walk(p);
        } else if (/\.(ts|tsx)$/.test(e.name)) {
          yield p;
        }
      }
    }

    // Patterns that count as "local positive System/Internet
    // derivations":
    //   - `apiVerified` (the local bypass we removed)
    //   - `Internet is working` / `Everything is working` literals
    //     in pages, paired with explicit positive state computation
    //   - Doctor findings read AND combined with observation/API
    //     to drive a 'good' tone
    //   - Doctor "loading" treated as empty
    let apiVerifiedHits = 0;
    let positiveNetHits = 0;
    let doctorEmptyHits = 0;
    for await (const t of walk(root)) {
      const txt = await fs.readFile(t, "utf-8");
      if (/apiVerified\b/.test(txt)) apiVerifiedHits += 1;
      // Only count Easy Home / System Overview / Easy Care positive
      // derivation attempts. We exclude test files explicitly.
      if (
        /apiVerified\b/.test(txt) ||
        /\bderived\.overall === "good"\b/.test(txt) ||
        /\bsystemTileState\s*=\s*"good"/.test(txt) ||
        /\binternetTileState\s*=\s*"good"/.test(txt)
      ) {
        positiveNetHits += 1;
      }
      // Doctor loading treated as empty (findings is `?? []`
      // AND success branch equals empty branch).
      if (/derived\.doctorFindings\b\s*\?\?\s*\[\s*\]/.test(txt)) {
        doctorEmptyHits += 1;
      }
    }
    expect(apiVerifiedHits).toBe(0);
    expect(positiveNetHits).toBe(0);
    // Doctor findings must NEVER be coerced from `null` to `[]`
    // in production source. Doctor loading and Doctor error both
    // produce `null`; substituting `[]` would render "0 findings"
    // as if Doctor had reported zero findings, which is a
    // semantically false claim. The only legitimate place for
    // `derived.doctorFindings ?? []` is inside a derivation that
    // already checks `derived.doctor === "success"` — and we have
    // none of those outside the centralized module's own
    // helpers (which live in state/health.ts and are not flagged
    // here because we exclude test/ but not state/; the pattern
    // below tolerates exactly one hit: the placeholder array in
    // `doctorFindingsFromResult` which never reaches the UI
    // count line because callers gate on `derived.doctor`).
    expect(doctorEmptyHits).toBe(0);
  });
});
