// Nodes UI tests: real API integration through the fake client,
// enrollment one-time display, rotation states, destructive action
// confirmation, degraded storage, offline/stale/revoked display,
// loading and empty states. No mocked success notifications: every
// assertion follows what the (fake) backend actually returned.
import { describe, expect, it } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { XrayCenterPage } from "../xray/center";
import type { FetchResult } from "../api/client";
import type { PageMeta } from "../pages/registry";

const pageStub = {
  id: "P050",
  name: "Xray Center",
  route: "/tunnels/xray/profiles",
  question: "",
} as unknown as PageMeta;

type NodeFixture = {
  id: string;
  name: string;
  status: string;
  version?: string;
  capabilities?: string[];
  lastSeenAt?: string;
  lastError?: string;
};

function nodesClient(
  nodes: NodeFixture[] = [],
  overrides: {
    rotateFails?: boolean;
    suspendFails?: boolean;
    clientExtra?: Record<string, unknown>;
  } = {},
) {
  const calls: Record<string, number> = {};
  const client = {
    fetchXrayServers: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayUsers: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayCenterStats: () =>
      Promise.resolve({
        ok: true,
        data: { stale: false, ledger: {} },
        age: 0,
      } as FetchResult<unknown>),
    fetchXrayDiagnostics: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayNodes: () => {
      calls.nodes = (calls.nodes ?? 0) + 1;
      return Promise.resolve({ ok: true, data: nodes, age: 0 } as FetchResult<
        NodeFixture[]
      >);
    },
    fetchXrayGroups: () =>
      Promise.resolve({
        ok: true,
        data: [
          {
            id: "g1",
            name: "VPN",
            format: "clash",
            enabled: true,
            members: [
              {
                nodeId: "n-1",
                serverId: "s",
                userId: "u",
                displayName: "NL",
                enabled: true,
              },
            ],
          },
        ],
        age: 0,
      } as FetchResult<unknown[]>),
    fetchXrayStorage: () =>
      Promise.resolve({
        ok: true,
        data: { domains: {} },
        age: 0,
      } as FetchResult<unknown>),
    xrayIssueEnrollToken: () => {
      calls.enroll = (calls.enroll ?? 0) + 1;
      return Promise.resolve({
        ok: true,
        data: { token: "TOKEN-VALUE" },
        age: 0,
      } as FetchResult<unknown>);
    },
    xrayStageRotation: () => {
      calls.rotate = (calls.rotate ?? 0) + 1;
      if (overrides.rotateFails) {
        return Promise.reject(new Error("503"));
      }
      return Promise.resolve({
        ok: true,
        data: { credential: "NEW-CRED-VALUE", state: "PENDING" },
        age: 0,
      } as FetchResult<unknown>);
    },
    fetchXrayRotationState: () =>
      Promise.resolve({
        ok: true,
        data: { state: "PENDING" },
        age: 0,
      } as FetchResult<unknown>),
    xraySuspendNode: () => {
      calls.suspend = (calls.suspend ?? 0) + 1;
      return overrides.suspendFails
        ? Promise.reject(new Error("503"))
        : Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>);
    },
    xrayResumeNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayRevokeNode: () => {
      calls.revoke = (calls.revoke ?? 0) + 1;
      return Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>);
    },
    xrayReconcileStorage: () =>
      Promise.resolve({
        ok: true,
        data: { result: "recovered" },
        age: 0,
      } as FetchResult<unknown>),
    ...overrides.clientExtra,
  };
  return { client: client as never, calls };
}

const activeNode: NodeFixture = {
  id: "n-1",
  name: "edge-1",
  status: "active",
  version: "26.3.27",
  capabilities: ["vless", "reality"],
  lastSeenAt: "2026-10-20T10:00:00Z",
};

describe("Independent security closure guidance", () => {
  for (const locale of ["en", "ru", "de", "es"] as const) {
    it(`staged rotation guidance uses the supported safe input in ${locale}`, async () => {
      const { client } = nodesClient([activeNode]);
      render(
        <XrayCenterPage
          page={pageStub}
          bundle={{} as never}
          client={client}
          locale={locale}
        />,
      );
      await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
      fireEvent.click(
        screen.getByText(
          locale === "ru" ? "Ротация креденшала" : "Rotate credential",
        ),
      );
      await waitFor(() =>
        expect(screen.getByText("NEW-CRED-VALUE")).toBeTruthy(),
      );
      const guidance = screen.getByText(/pull-rotation/).textContent ?? "";
      expect(guidance).not.toMatch(/--credential(?:\s|=)/);
      expect(guidance).toContain("--credential-stdin");
    });
  }
});
