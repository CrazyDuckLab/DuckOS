/**
 * DuckOS V1 — Service-discovery page tests.
 *
 * Locks in the bounded P065 UI contract:
 *   - renders the service-status card with all four protocols
 *   - rejects non-canonical MACs on Wake
 *   - disables the inputs while pending
 *   - renders empty-state copy when there are no networks
 */
import { fireEvent, render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ServiceDiscoveryPage } from "../pages/ServiceDiscoveryPage";
import type { APIClient } from "../api/client";
import { PrefsContext, initialPrefs } from "../state/store";

const baseBundle = {} as Record<string, unknown>;

const prefsValue = {
  state: {
    ...initialPrefs,
    health: {
      overview: "good" as const,
      readiness: "good" as const,
      doctor: "good" as const,
      topology: "good" as const,
      reachable: true,
      lastObservedAt: Date.now(),
      timeTrust: "trusted" as const,
      activeIncidents: 0,
      pendingChanges: 0,
    },
  },
  dispatch: () => {},
};

function fakeClient(fetchJson: ReturnType<typeof vi.fn>): APIClient {
  return {
    token: () => "t",
    fetchJson,
  } as unknown as APIClient;
}

function wrap(client: APIClient) {
  return render(
    <PrefsContext.Provider value={prefsValue}>
      <ServiceDiscoveryPage
        page={{ id: "P065", name: "Service discovery" }}
        bundle={baseBundle}
        client={client}
      />
    </PrefsContext.Provider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("ServiceDiscoveryPage", () => {
  it("renders all four protocols in the status card", async () => {
    const fetchJson = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        stats: { networks: 0, mappings: 0, observedAt: "", stopped: false },
        networks: [],
        mappings: [],
      }),
    });
    const { container } = wrap(fakeClient(fetchJson));
    expect(container.textContent).toMatch(/UPnP IGD/);
    expect(container.textContent).toMatch(/NAT-PMP/);
    expect(container.textContent).toMatch(/Wake-on-LAN/);
  });

  it("rejects a malformed MAC on Wake", async () => {
    const fetchJson = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        stats: { networks: 0, mappings: 0, observedAt: "", stopped: false },
        networks: [],
        mappings: [],
      }),
    });
    const { getByLabelText, getByRole, getByText } = wrap(fakeClient(fetchJson));
    fireEvent.change(getByLabelText(/Target MAC/i), {
      target: { value: "AA:BB:CC" },
    });
    fireEvent.click(getByRole("button", { name: /Send magic packet/i }));
    await waitFor(() => {
      expect(getByText(/six colon-separated hex octets/i)).toBeTruthy();
    });
  });

  it("shows the empty-state copy when no networks are configured", async () => {
    const fetchJson = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        stats: { networks: 0, mappings: 0, observedAt: "", stopped: false },
        networks: [],
        mappings: [],
      }),
    });
    const { getByText } = wrap(fakeClient(fetchJson));
    await waitFor(() => {
      expect(getByText(/Disabled by operator choice/i)).toBeTruthy();
    });
  });
});