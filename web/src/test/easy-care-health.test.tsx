/**
 * DuckOS V1 — Easy Care production-health regression.
 *
 * This test pins the production `/api/v1/health` contract, which is
 * intentionally minimal:
 *
 *   { "status": "ok", "schemaVersion": "1" }
 *
 * The previous frontend type asserted per-component health, timeTrust,
 * findings, etc. Easy Care called `.some()` on `health.data.components`
 * which is `undefined` in production, producing:
 *
 *   TypeError: Cannot read properties of undefined (reading 'some')
 *
 * These tests lock in the fix: Easy Care must render against the
 * real minimal contract and against a failed health probe without
 * crashing, without an error boundary, and without inventing fake
 * component health.
 */
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { APIClient, type FetchResult, type HealthPayload } from "../api/client";
import { EasyCarePage } from "../pages/EasyV2";
import { messages } from "../i18n/messages";
import type { Messages } from "../i18n/messages";
import type { PageMeta } from "../pages/registry";

const PROD_HEALTH_PAYLOAD: HealthPayload = {
  status: "ok",
  schemaVersion: "1",
};

const PAGE: PageMeta = {
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

class StubClient extends APIClient {
  private health: FetchResult<HealthPayload>;
  private doctor: FetchResult<unknown>;

  constructor(
    opts: {
      health?: FetchResult<HealthPayload>;
      doctor?: FetchResult<unknown>;
    } = {},
  ) {
    super({
      token: "t",
      locale: "en",
      onFetch: () => {},
    } as never);
    this.health = opts.health ?? {
      ok: true,
      data: PROD_HEALTH_PAYLOAD,
      age: 0,
    };
    this.doctor = opts.doctor ?? {
      ok: true,
      data: { findings: [] },
      age: 0,
    };
  }

  fetchHealth(): Promise<FetchResult<HealthPayload>> {
    return Promise.resolve(this.health);
  }
  fetchDoctor(): Promise<FetchResult<unknown>> {
    return Promise.resolve(this.doctor);
  }
}

describe("Easy Care against the production /api/v1/health contract", () => {
  it("renders without crashing when /api/v1/health returns the minimal production payload", async () => {
    const client = new StubClient({
      health: {
        ok: true,
        data: PROD_HEALTH_PAYLOAD,
        age: 0,
      },
    });
    const navigate = () => {};
    const { container, findByText } = render(
      <EasyCarePage
        page={PAGE}
        bundle={messages.en as Messages}
        navigate={navigate}
        client={client}
      />,
    );
    // The page must not fabricate fake component health. The only
    // signal it surfaces is API reachability, derived from
    // health.status === "ok".
    expect(await findByText("API reachable")).toBeTruthy();
    const text = container.textContent ?? "";
    expect(text).toContain("Care & recovery");
    expect(text).not.toContain("undefined");
    expect(text).not.toContain("NaN");
    expect(text).not.toContain("{");
  });

  it("renders the API-unreachable state when /api/v1/health is not ok", async () => {
    const client = new StubClient({
      health: {
        ok: true,
        data: { status: "degraded", schemaVersion: "1" } as HealthPayload,
        age: 0,
      },
    });
    const { findByText } = render(
      <EasyCarePage
        page={PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    expect(await findByText("API unreachable")).toBeTruthy();
  });

  it("renders the API-unreachable state when the health request fails", async () => {
    const client = new StubClient({
      health: {
        ok: false,
        reason: "network",
        message: "API did not respond",
        status: 0,
      },
    });
    const { findByText } = render(
      <EasyCarePage
        page={PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    expect(await findByText("API unreachable")).toBeTruthy();
  });

  it("renders real Doctor findings when present, not fake component health", async () => {
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
    const { container, findByText } = render(
      <EasyCarePage
        page={PAGE}
        bundle={messages.en as Messages}
        navigate={() => {}}
        client={client}
      />,
    );
    // Wait for the async fetch + re-render before asserting on text.
    expect(await findByText("SSD wear is at 92%")).toBeTruthy();
    const text = container.textContent ?? "";
    // The card combines component + code in one heading.
    expect(text).toContain("disk-wear-high");
    expect(text).toContain("SSD wear is at 92%");
  });
});
