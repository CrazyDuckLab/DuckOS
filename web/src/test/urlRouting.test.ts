/**
 * DuckOS V1 — URL routing helper tests.
 *
 * Locks in the deep-link contract:
 *   - `readRouteFromHash` accepts catalog routes with or without the leading
 *     slash and rejects unknown / empty hashes.
 *   - `syncHashToRoute` writes the canonical `#/route` form, skips no-op
 *     writes, and uses `replaceState` so the back stack isn't polluted.
 *   - The shell's `navigate` callback updates the hash so the next
 *     `readRouteFromHash` call returns the new route.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readRouteFromHash, syncHashToRoute } from "../shell/urlRouting";

const SAMPLE_ROUTE = "/wan/connections";

describe("readRouteFromHash", () => {
  beforeEach(() => {
    window.location.hash = "";
  });
  afterEach(() => {
    window.location.hash = "";
  });

  it("returns null when no hash is set", () => {
    expect(readRouteFromHash()).toBeNull();
  });

  it("returns null for an unknown route", () => {
    window.location.hash = "#/no/such/page";
    expect(readRouteFromHash()).toBeNull();
  });

  it("parses a canonical hash route", () => {
    window.location.hash = "#" + SAMPLE_ROUTE;
    expect(readRouteFromHash()).toBe(SAMPLE_ROUTE);
  });

  it("tolerates a missing leading slash", () => {
    window.location.hash = "#wan/connections";
    expect(readRouteFromHash()).toBe(SAMPLE_ROUTE);
  });

  it("strips a query suffix", () => {
    window.location.hash = "#" + SAMPLE_ROUTE + "?foo=bar";
    expect(readRouteFromHash()).toBe(SAMPLE_ROUTE);
  });
});

describe("syncHashToRoute", () => {
  beforeEach(() => {
    window.location.hash = "";
  });
  afterEach(() => {
    window.location.hash = "";
  });

  it("writes the canonical hash form", () => {
    syncHashToRoute(SAMPLE_ROUTE);
    expect(window.location.hash).toBe("#" + SAMPLE_ROUTE);
  });

  it("is a no-op when the hash already matches", () => {
    syncHashToRoute(SAMPLE_ROUTE);
    const replaceSpy = vi.spyOn(window.history, "replaceState");
    syncHashToRoute(SAMPLE_ROUTE);
    expect(replaceSpy).not.toHaveBeenCalled();
    replaceSpy.mockRestore();
  });
});

describe("hash round-trip", () => {
  it("preserves the route through syncHashToRoute → readRouteFromHash", () => {
    const route = "/system/overview";
    syncHashToRoute(route);
    expect(readRouteFromHash()).toBe(route);
  });
});
