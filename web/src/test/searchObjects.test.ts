import { describe, expect, it } from "vitest";
import { searchObjects } from "../search/objects";
import type { Overview } from "../../../shared/api/types";

function makeOverview(over: Partial<Overview> = {}): Overview {
  return {
    schemaVersion: "1",
    observedAt: new Date().toISOString(),
    environment: "test",
    system: {
      hostname: "duck-dev-01",
      kernel: "Linux",
      os: "Debian",
      architecture: "x86_64",
      uptimeSeconds: 1234,
    },
    cpu: { logicalCores: 4, usagePercent: null, loadAverage: [0.1, 0.2, 0.3] },
    memory: { totalBytes: 1e9, availableBytes: 5e8, usedBytes: 5e8 },
    storage: [],
    interfaces: [
      {
        name: "eth0",
        index: 1,
        mtu: 1500,
        mac: "00:15:5d:04:07:07",
        state: "up",
        addresses: [
          { address: "172.18.4.149", family: "ipv4", prefixLength: 16 },
          { address: "fe80::1", family: "ipv6", prefixLength: 64 },
        ],
      },
      {
        name: "eth0.10",
        index: 2,
        mtu: 1500,
        mac: "00:15:5d:04:07:08",
        state: "up",
        addresses: [],
      },
    ],
    routes: [],
    dns: { nameservers: ["1.1.1.1"], searchDomains: [], source: "static" },
    services: [
      {
        name: "sshd",
        loadState: "loaded",
        activeState: "active",
        subState: "running",
      },
    ],
    nftables: { status: "available", tables: 1, chains: 2, rules: 3 },
    issues: [],
    ...over,
  } as Overview;
}

describe("searchObjects", () => {
  it("returns the interface row for an interface-name query (eth0)", () => {
    const overview = makeOverview();
    const hits = searchObjects("eth0", overview);
    expect(hits.length).toBeGreaterThan(0);
    // "eth0" matches both eth0 and eth0.10 — we just need to confirm
    // the exact match is in the result set.
    const eth0Hit = hits.find((h) => h.id === "eth0");
    expect(eth0Hit).toBeDefined();
    expect(eth0Hit?.kind).toBe("interface");
    expect(eth0Hit?.displayName).toBe("eth0");
    expect(eth0Hit?.route).toBe("/interfaces");
    expect(eth0Hit?.ref).toEqual({ kind: "network.interface", id: "eth0" });
  });

  it("finds an interface by IP prefix (172.18)", () => {
    const overview = makeOverview();
    const hits = searchObjects("172.18", overview);
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.id === "172.18.4.149")).toBe(true);
  });

  it("finds an interface by exact IPv4 address", () => {
    const overview = makeOverview();
    const hits = searchObjects("172.18.4.149", overview);
    expect(hits.length).toBeGreaterThan(0);
    const addressHit = hits.find((h) => h.kind === "address");
    expect(addressHit?.displayName).toBe("172.18.4.149");
    expect(addressHit?.route).toBe("/interfaces");
  });

  it("finds an interface by MAC address", () => {
    const overview = makeOverview();
    const hits = searchObjects("00:15:5d:04:07:07", overview);
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.displayName === "00:15:5d:04:07:07")).toBe(true);
  });

  it("surfaces VLAN hits when an interface name encodes one", () => {
    const overview = makeOverview();
    const hits = searchObjects("eth0.10", overview);
    expect(
      hits.some((h) => h.kind === "vlan" && h.displayName === "VLAN 10"),
    ).toBe(true);
  });

  it("surfaces the system hostname", () => {
    const overview = makeOverview();
    const hits = searchObjects("duck-dev-01", overview);
    expect(hits.some((h) => h.kind === "device")).toBe(true);
  });

  it("returns no hits for queries shorter than two characters", () => {
    const overview = makeOverview();
    expect(searchObjects("e", overview)).toEqual([]);
  });

  it("returns no hits when overview is missing", () => {
    expect(searchObjects("eth0", null)).toEqual([]);
  });

  it("returns no hits for unrelated queries", () => {
    const overview = makeOverview();
    expect(searchObjects("zzz_nope", overview)).toEqual([]);
  });
});
