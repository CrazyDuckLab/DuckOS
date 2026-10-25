// Nodes UI security closure regressions: the staged-rotation guidance
// must recommend ONLY the supported protected-stdin contract, must
// never render literal credential flags or secret values, and the
// Stage → Pull → Confirm → Recover flow semantics must stay intact in
// all four locales.
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

function guidanceClient(
  nodes: NodeFixture[],
  overrides: { rotateFails?: boolean } = {},
) {
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
    fetchXrayNodes: () =>
      Promise.resolve({ ok: true, data: nodes, age: 0 } as FetchResult<
        NodeFixture[]
      >),
    fetchXrayGroups: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayStorage: () =>
      Promise.resolve({
        ok: true,
        data: { domains: {} },
        age: 0,
      } as FetchResult<unknown>),
    xrayIssueEnrollToken: () =>
      Promise.resolve({
        ok: true,
        data: { token: "TOKEN-VALUE" },
        age: 0,
      } as FetchResult<unknown>),
    xrayStageRotation: () => {
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
    xraySuspendNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayResumeNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayRevokeNode: () =>
      Promise.resolve({ ok: true, age: 0 } as FetchResult<unknown>),
    xrayReconcileStorage: () =>
      Promise.resolve({
        ok: true,
        data: { result: "recovered" },
        age: 0,
      } as FetchResult<unknown>),
  };
  return client as never;
}

const activeNode: NodeFixture = {
  id: "n-1",
  name: "edge-1",
  status: "active",
  version: "26.3.27",
  capabilities: ["vless", "reality"],
  lastSeenAt: "2026-10-20T10:00:00Z",
};

const locales = ["en", "ru", "de", "es"] as const;
// Button labels: ru is localized; de/es intentionally inherit the EN
// label (only the guidance text is overridden per-locale), matching
// the unchanged independent audit test.
const rotateLabels: Record<(typeof locales)[number], string> = {
  en: "Rotate credential",
  ru: "Ротация креденшала",
  de: "Rotate credential",
  es: "Rotate credential",
};

describe("Rotation guidance uses the protected stdin contract", () => {
  for (const locale of locales) {
    it(`safe stdin guidance is rendered in ${locale}`, async () => {
      const client = guidanceClient([activeNode]);
      render(
        <XrayCenterPage
          page={pageStub}
          bundle={{} as never}
          client={client}
          locale={locale}
        />,
      );
      await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
      fireEvent.click(screen.getByText(rotateLabels[locale]));
      await waitFor(() =>
        expect(screen.getByText("NEW-CRED-VALUE")).toBeTruthy(),
      );
      const guidance = screen.getByText(/pull-rotation/).textContent ?? "";
      // The supported protected-stdin contract is present…
      expect(guidance).toContain("--credential-stdin");
      // …and no literal credential flag form is recommended.
      expect(guidance).not.toMatch(/--credential(?:\s|=)/);
      expect(guidance).not.toMatch(/-credential(?:\s|=)/);
      // The command names the real operation and keeps the id placeholder.
      expect(guidance).toContain("duck xray nodes pull-rotation");
      expect(guidance).toContain("--id <id>");
      // No secret value leaks into the generated command text.
      expect(guidance).not.toContain("NEW-CRED-VALUE");
      expect(guidance).not.toContain("TOKEN-VALUE");
      // No unsafe shortcuts are recommended.
      expect(guidance).not.toMatch(/echo\s+['"]?[A-Za-z0-9_-]*SECRET/i);
      // Staging alone is not presented as a completed rotation.
      expect(guidance).toMatch(/PENDING/);
    });
  }

  it("staging alone does not claim completion in any locale", async () => {
    for (const locale of locales) {
      const client = guidanceClient([activeNode]);
      const { unmount } = render(
        <XrayCenterPage
          page={pageStub}
          bundle={{} as never}
          client={client}
          locale={locale}
        />,
      );
      await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
      fireEvent.click(screen.getByText(rotateLabels[locale]));
      await waitFor(() =>
        expect(screen.getByText("NEW-CRED-VALUE")).toBeTruthy(),
      );
      // PENDING semantics stay explicit: the old credential is still valid.
      expect(screen.getAllByText(/PENDING/).length).toBeGreaterThan(0);
      unmount();
    }
  });

  it("rotation failure keeps the state untouched message", async () => {
    const client = guidanceClient([activeNode], { rotateFails: true });
    render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        client={client}
        locale="en"
      />,
    );
    await waitFor(() => expect(screen.getByText("edge-1")).toBeTruthy());
    fireEvent.click(screen.getByText("Rotate credential"));
    await waitFor(() =>
      expect(screen.getByText(/Rotation failed/)).toBeTruthy(),
    );
    expect(screen.queryByText(/pull-rotation/)).toBeNull();
  });
});
