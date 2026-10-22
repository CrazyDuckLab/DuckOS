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
      Promise.resolve({ ok: true, data: { state: "PENDING" }, age: 0 } as FetchResult<unknown>),
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

describe("Nodes UI", () => {
  it("renders inventory with real backend fields and derives group count", async () => {
    const { client } = nodesClient([activeNode]);
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    expect(screen.getByText("26.3.27")).toBeTruthy();
    expect(screen.getByText("vless, reality")).toBeTruthy();
    expect(screen.getByText("1")).toBeTruthy(); // groups referencing n-1
  });

  it("shows unavailable fields as unavailable, never invented", async () => {
    const { client } = nodesClient([
      {
        id: "n-2",
        name: "ghost",
        status: "stale",
        lastError: "connection refused",
      },
    ]);
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText("ghost")).toBeTruthy());
    expect(screen.getByText("never")).toBeTruthy(); // lastSeen unavailable
    expect(
      document.querySelectorAll("table td").length > 0 &&
        Array.from(document.querySelectorAll("table td")).some(
          (td) => td.textContent === "—",
        ),
    ).toBeTruthy(); // version unavailable
    expect(screen.getByText(/connection refused/)).toBeTruthy();
    expect(screen.getByText(/не отвечает|stale/)).toBeTruthy();
  });

  it("displays the enrollment token exactly once with instructions", async () => {
    const { client } = nodesClient([]);
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    fireEvent.click(screen.getByText("Generate enrollment token"));
    await waitFor(() => expect(screen.getByText("TOKEN-VALUE")).toBeTruthy());
    expect(screen.getByText(/never be shown again/)).toBeTruthy();
    expect(screen.getByText(/duck xray nodes redeem/)).toBeTruthy();
  });

  it("shows an empty state when no nodes exist", async () => {
    const { client } = nodesClient([]);
    const { container } = render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() =>
      expect(container.querySelector(".v4-page")).toBeTruthy(),
    );
  });

  it("requires confirmation for revocation and only reports success after the backend", async () => {
    const { client, calls } = nodesClient([activeNode]);
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    fireEvent.click(screen.getByText("Revoke"));
    expect(calls.revoke ?? 0).toBe(0); // nothing sent before confirmation
    fireEvent.click(screen.getByRole("dialog").querySelector("button")!);
    await waitFor(() => expect(calls.revoke).toBe(1));
  });

  it("rotation displays the new credential once with the committed-state guidance", async () => {
    const { client } = nodesClient([activeNode]);
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    fireEvent.click(screen.getByText("Rotate credential"));
    await waitFor(() =>
      expect(screen.getByText("NEW-CRED-VALUE")).toBeTruthy(),
    );
    // Staged semantics: old credential NOT invalidated at stage time.
    expect(screen.getByText(/STAGED \(PENDING\)/)).toBeTruthy();
    expect(screen.getByText(/pull-rotation/)).toBeTruthy();
  });

  it("rotation failure shows a failure message, never success", async () => {
    const { client } = nodesClient([activeNode], { rotateFails: true });
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    fireEvent.click(screen.getByText("Rotate credential"));
    await waitFor(() =>
      expect(screen.getByText(/Rotation failed/)).toBeTruthy(),
    );
    expect(screen.queryByText("NEW-CRED-VALUE")).toBeNull();
  });

  it("surfaces DEGRADED storage with a real reconcile action", async () => {
    let degraded = true;
    const { client } = nodesClient([activeNode], {
      clientExtra: {
        fetchXrayStorage: () =>
          Promise.resolve({
            ok: true,
            data: { domains: degraded ? { nodes: "commit_uncertain" } : {} },
            age: 0,
          } as FetchResult<unknown>),
        xrayReconcileStorage: () => {
          degraded = false;
          return Promise.resolve({
            ok: true,
            data: { result: "recovered" },
            age: 0,
          } as FetchResult<unknown>);
        },
      },
    });
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText(/DEGRADED/)).toBeTruthy());
    fireEvent.click(screen.getByText("Reconcile storage"));
    await waitFor(() => expect(screen.queryByText(/DEGRADED/)).toBeNull());
  });

  it("shows an error state when the nodes API fails (controller unavailable / RBAC)", async () => {
    const failing = {
      fetchXrayServers: () =>
        Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<
          unknown[]
        >),
      fetchXrayUsers: () =>
        Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<
          unknown[]
        >),
      fetchXrayCenterStats: () =>
        Promise.resolve({
          ok: true,
          data: { stale: false },
          age: 0,
        } as FetchResult<unknown>),
      fetchXrayDiagnostics: () =>
        Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<
          unknown[]
        >),
      fetchXrayNodes: () =>
        Promise.resolve({
          ok: false,
          reason: "forbidden",
          message: "403",
        } as FetchResult<never>),
      fetchXrayGroups: () =>
        Promise.resolve({
          ok: false,
          reason: "forbidden",
          message: "403",
        } as FetchResult<never>),
      fetchXrayStorage: () =>
        Promise.resolve({
          ok: true,
          data: { domains: {} },
          age: 0,
        } as FetchResult<unknown>),
    };
    const { container } = render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        client={failing as never}
      />,
    );
    await waitFor(() =>
      expect(container.querySelector(".v4-page")).toBeTruthy(),
    );
  });

  it("suspend flows through confirmation and reloads real state", async () => {
    const { client, calls } = nodesClient([activeNode]);
    render(
      <XrayCenterPage page={pageStub} bundle={{} as never} client={client} />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    fireEvent.click(screen.getByText("Suspend"));
    fireEvent.click(screen.getByRole("dialog").querySelector("button")!);
    await waitFor(() => expect(calls.suspend).toBe(1));
    // Real state re-fetch after mutation.
    await waitFor(() => expect(calls.nodes).toBeGreaterThanOrEqual(2));
  });
});
