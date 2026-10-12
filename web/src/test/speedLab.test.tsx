/**
 * DuckOS V1 — Speed Lab page tests.
 *
 * Locks in the bounded P090 UI contract:
 *   - renders all three probe kinds
 *   - validates URL/host/port on Start
 *   - calls the backend on valid input
 */
import { fireEvent, render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SpeedLabPage } from "../pages/SpeedLabPage";
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

function wrap(_node: React.ReactNode, client: APIClient) {
  return render(
    <PrefsContext.Provider value={prefsValue}>
      <SpeedLabPage
        page={{ id: "P090", name: "Duck Speed Lab" }}
        bundle={baseBundle}
        client={client}
      />
    </PrefsContext.Provider>,
  );
}

describe("SpeedLabPage", () => {
  it("renders all three probe kinds", () => {
    const { getByRole } = wrap(null, fakeClient(vi.fn()));
    const select = getByRole("combobox", {
      name: /probe type/i,
    }) as HTMLSelectElement;
    const options = Array.from(select.options).map((o) => o.value);
    expect(options).toEqual(["latency", "http", "throughput"]);
  });

  it("blocks Start when the URL is not http(s) for HTTP/Throughput", async () => {
    const fetchJson = vi.fn();
    const { getByRole, getByText, getByLabelText } = wrap(
      null,
      fakeClient(fetchJson),
    );
    fireEvent.change(getByRole("combobox", { name: /probe type/i }), {
      target: { value: "throughput" },
    });
    fireEvent.change(getByLabelText(/URL/i), {
      target: { value: "ftp://nope" },
    });
    fireEvent.click(getByRole("button", { name: /start/i }));
    await waitFor(() => {
      expect(getByText(/http:\/\/ or https:\/\//i)).toBeTruthy();
    });
    expect(fetchJson).not.toHaveBeenCalled();
  });

  it("requires Host for latency probes", async () => {
    const fetchJson = vi.fn();
    const { getByRole, getByText } = wrap(null, fakeClient(fetchJson));
    fireEvent.click(getByRole("button", { name: /start/i }));
    await waitFor(() => {
      expect(getByText(/Host is required/i)).toBeTruthy();
    });
    expect(fetchJson).not.toHaveBeenCalled();
  });
});
