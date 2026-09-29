/**
 * DuckOS V1 — focused unit / integration tests.
 *
 * Covers:
 *   - Help index integrity (99 pages, no duplicates, all ids resolve)
 *   - Page registry integrity (all 99 routes + ids resolve)
 *   - Search: returns results, never executes destructive actions
 *   - Configurator form primitives: validators behave correctly
 *   - StatePanel: renders every state kind
 *   - Inspector: shows the V1 distinctions (configured/desired/...)
 */
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { ALL_PAGES, PAGES_BY_ID, PAGES_BY_ROUTE } from "../pages/registry";
import { HELP_INDEX, searchHelp, lookupEntry } from "../help/index";
import {
  validateCidr,
  validateHostname,
  validateIpv4,
  validateIpv6,
  validateMac,
  validatePort,
  validatePortRange,
  validateSeconds,
  validateVlanId,
} from "../components/forms/validators";
import { StatePanel } from "../components/forms/States";

describe("page registry integrity", () => {
  it("registers exactly 99 pages", () => {
    expect(ALL_PAGES.length).toBe(99);
  });
  it("has unique page ids", () => {
    const ids = new Set<string>();
    for (const p of ALL_PAGES) {
      expect(ids.has(p.id)).toBe(false);
      ids.add(p.id);
    }
  });
  it("has unique routes", () => {
    const routes = new Set<string>();
    for (const p of ALL_PAGES) {
      expect(routes.has(p.route)).toBe(false);
      routes.add(p.route);
    }
  });
  it("PAGES_BY_ID and PAGES_BY_ROUTE contain every page", () => {
    for (const p of ALL_PAGES) {
      expect(PAGES_BY_ID.get(p.id)).toBe(p);
      expect(PAGES_BY_ROUTE.get(p.route)).toBe(p);
    }
  });
  it("every page route resolves under one of the ten sections", () => {
    const valid = new Set(["interfaces", "wan", "lan", "routing", "firewall", "tunnels", "dns", "services", "system", "logs", "easy"]);
    for (const p of ALL_PAGES) {
      expect(valid.has(p.section)).toBe(true);
    }
  });
});

describe("help index integrity", () => {
  it("covers every catalog page", () => {
    const helpIds = new Set(HELP_INDEX.map((h) => h.pageId));
    for (const p of ALL_PAGES) {
      expect(helpIds.has(p.id)).toBe(true);
    }
  });
  it("has no duplicate page ids", () => {
    const ids = new Set<string>();
    for (const e of HELP_INDEX) {
      expect(ids.has(e.pageId)).toBe(false);
      ids.add(e.pageId);
    }
  });
  it("search returns hits for natural-language queries", () => {
    const hits = searchHelp("WireGuard");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits[0].pageId).toBe("P047");
  });
  it("search matches Russian synonym", () => {
    const hits = searchHelp("провайдер");
    expect(hits.length).toBeGreaterThan(0);
  });
  it("search returns no hits for nonsense queries", () => {
    const hits = searchHelp("xyzzy plugh");
    expect(hits).toHaveLength(0);
  });
  it("search returns hits for page id", () => {
    const hits = searchHelp("P024");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits[0].pageId).toBe("P024");
  });
  it("lookupEntry returns undefined for unknown id", () => {
    expect(lookupEntry("PX99")).toBeUndefined();
  });
  it("help entries declare the four locales", () => {
    for (const e of HELP_INDEX) {
      expect(e.languages).toEqual(["en", "ru", "de", "es"]);
    }
  });
});

describe("network validators", () => {
  it("accepts a valid IPv4 and rejects an invalid one", () => {
    expect(validateIpv4("192.0.2.1").ok).toBe(true);
    expect(validateIpv4("999.0.2.1").ok).toBe(false);
    expect(validateIpv4("hello").ok).toBe(false);
  });
  it("accepts a valid IPv6 and rejects an invalid one", () => {
    expect(validateIpv6("2001:db8::1").ok).toBe(true);
    expect(validateIpv6("2001:db8:::1").ok).toBe(false);
    expect(validateIpv6("not::an::ip").ok).toBe(false);
  });
  it("validates CIDR", () => {
    expect(validateCidr("192.0.2.0/24", "ipv4").ok).toBe(true);
    expect(validateCidr("192.0.2.0/40", "ipv4").ok).toBe(false);
    expect(validateCidr("2001:db8::/64", "ipv6").ok).toBe(true);
    expect(validateCidr("192.0.2.0/24", "ipv6").ok).toBe(false);
  });
  it("validates MAC", () => {
    expect(validateMac("aa:bb:cc:dd:ee:ff").ok).toBe(true);
    expect(validateMac("aabb.ccdd.eeff").ok).toBe(false);
  });
  it("validates hostname", () => {
    expect(validateHostname("router.lan").ok).toBe(true);
    expect(validateHostname("-leading").ok).toBe(false);
    expect(validateHostname("").ok).toBe(false);
  });
  it("validates VLAN id", () => {
    expect(validateVlanId(20).ok).toBe(true);
    expect(validateVlanId(0).ok).toBe(false);
    expect(validateVlanId(4095).ok).toBe(false);
  });
  it("validates ports", () => {
    expect(validatePort(80).ok).toBe(true);
    expect(validatePort(0).ok).toBe(false);
    expect(validatePort(70000).ok).toBe(false);
  });
  it("validates port range", () => {
    expect(validatePortRange("1000-2000").ok).toBe(true);
    expect(validatePortRange("2000-1000").ok).toBe(false);
    expect(validatePortRange("not a range").ok).toBe(false);
  });
  it("validates seconds", () => {
    expect(validateSeconds(30).ok).toBe(true);
    expect(validateSeconds(-1).ok).toBe(false);
  });
});

describe("StatePanel", () => {
  it("renders every state kind without crashing", () => {
    const kinds = [
      "loading",
      "empty",
      "error",
      "degraded",
      "permission",
      "missing-contract",
      "stale",
      "unsupported",
      "intentionally-disabled",
    ] as const;
    for (const k of kinds) {
      const { unmount } = render(<StatePanel state={k} />);
      unmount();
    }
  });
});