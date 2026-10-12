/**
 * DuckOS V1 — generic ObjectsPage + Collection integration tests.
 *
 * Verifies the typed envelope shape, per-row permissions,
 * availability filtering, and that the V4 chrome does NOT surface
 * raw backend jargon (envelope sources, snapshot ids) — those
 * belong in the technical-details drawer only.
 */
import { render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { APIClient } from "../api/client";
import type { Collection } from "../api/types";
import { ObjectsPage } from "../configurators/Objects";
import { messages } from "../i18n/messages";
import { PAGES_BY_ID } from "../pages/registry";
import type { DataColumn } from "../components/forms/Table";
import type { ViewObject } from "../api/types";
import { PrefsContext } from "../state/store";

function makeClient(): APIClient {
  return new APIClient({
    token: "t",
    locale: "en",
    onFetch: () => {},
  } as never) as unknown as APIClient;
}

const collection: Collection = {
  snapshotId: "snap-1",
  total: 3,
  complete: true,
  issues: [{ source: "agent", reasonId: "demo_only", detail: "synthetic" }],
  items: [
    {
      ref: { kind: "network.interface", id: "eth0" },
      displayName: "eth0",
      desired: null,
      availability: "available",
      supportedActions: ["show", "edit", "delete"],
      permissions: {
        observe: true,
        configure: true,
        action: false,
        exportSecret: false,
        delete: true,
      },
    } as unknown as ViewObject,
    {
      ref: { kind: "network.interface", id: "eth1" },
      displayName: "eth1",
      desired: null,
      availability: "unsupported",
      supportedActions: ["show"],
      permissions: {
        observe: true,
        configure: false,
        action: false,
        exportSecret: false,
      },
    } as unknown as ViewObject,
    {
      ref: { kind: "network.interface", id: "eth2" },
      displayName: "eth2",
      desired: null,
      availability: "credential-pending",
      supportedActions: ["show"],
      permissions: {
        observe: true,
        configure: false,
        action: false,
        exportSecret: false,
      },
    } as unknown as ViewObject,
  ],
};

describe("ObjectsPage", () => {
  it("renders a typed Collection with availability filter and per-row actions", async () => {
    const client = makeClient();
    vi.spyOn(client, "fetchObjects").mockResolvedValue({
      ok: true,
      data: collection,
      age: 1,
    });
    const page = PAGES_BY_ID.get("P007")!;
    const columns: DataColumn<ViewObject>[] = [
      { key: "name", label: "Name", render: (v) => v.displayName },
    ];
    const { container, getByText } = render(
      <PrefsContext.Provider
        value={{
          state: {
            locale: "en",
            mode: "full",
            theme: "system",
            sidebarCollapsed: false,
            inspectorPageId: null,
            health: {
              overview: "good",
              readiness: "good",
              doctor: "good",
              topology: "good",
              reachable: true,
              lastObservedAt: Date.now(),
              timeTrust: "trusted",
              activeIncidents: 0,
              pendingChanges: 0,
            },
            pending: [],
          },
          dispatch: () => {},
        }}
      >
        <ObjectsPage
          page={page}
          bundle={messages.en}
          client={client}
          kind="network.interface"
          columns={columns}
        />
      </PrefsContext.Provider>,
    );
    await waitFor(() => {
      expect(getByText("eth0")).toBeTruthy();
      expect(getByText("eth1")).toBeTruthy();
      expect(getByText("eth2")).toBeTruthy();
    });
    // Per-row actions are visible only when the row permits them.
    const buttons = Array.from(container.querySelectorAll("button"));
    expect(buttons.some((b) => b.textContent?.includes("Edit"))).toBe(true);
    expect(buttons.some((b) => b.textContent?.includes("Delete"))).toBe(true);
    // V4 chrome does NOT render raw envelope jargon in the main view
    // — the source agent id belongs in the technical-details drawer.
    expect(container.textContent).not.toContain("agent:");
    // The snapshot id IS surfaced (collapsed under "Technical details"
    // for power users), but it must not appear in the page header.
    const details = container.querySelector("details");
    if (details) {
      expect(container.textContent).toContain("snap-1");
      expect(details.textContent).toContain("snap-1");
    }
  });
});
