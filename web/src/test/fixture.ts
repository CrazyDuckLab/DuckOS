import type { Overview } from "../../../shared/api/types";

// Synthetic TEST-only observations. Never imported by runtime application code.
export function overviewFixture(): Overview {
  return {
    schemaVersion: "1",
    observedAt: new Date().toISOString(),
    environment: "test",
    system: {
      hostname: "synthetic-test-router",
      kernel: "6.test",
      os: "Debian GNU/Linux 13 (trixie)",
      architecture: "amd64",
      uptimeSeconds: 90061,
    },
    cpu: { logicalCores: 4, usagePercent: 12.5, loadAverage: [0.4, 0.3, 0.2] },
    memory: {
      totalBytes: 8 * 1024 ** 3,
      availableBytes: 6 * 1024 ** 3,
      usedBytes: 2 * 1024 ** 3,
    },
    storage: [
      {
        mountpoint: "/",
        filesystem: "ext4",
        totalBytes: 64 * 1024 ** 3,
        availableBytes: 48 * 1024 ** 3,
        usedBytes: 16 * 1024 ** 3,
      },
    ],
    interfaces: [
      {
        name: "test-lan",
        index: 2,
        mtu: 1500,
        mac: "02:00:00:00:00:01",
        state: "up",
        addresses: [
          { address: "192.0.2.1", family: "ipv4", prefixLength: 24 },
          { address: "2001:db8::1", family: "ipv6", prefixLength: 64 },
        ],
      },
      {
        name: "test-idle",
        index: 3,
        mtu: 1500,
        mac: "02:00:00:00:00:02",
        state: "down",
        addresses: [],
      },
    ],
    routes: [
      {
        family: "ipv4",
        destination: "0.0.0.0/0",
        gateway: "192.0.2.254",
        device: "test-lan",
        protocol: "static",
        scope: "global",
        table: "main",
        metric: 100,
      },
      {
        family: "ipv6",
        destination: "2001:db8::/64",
        gateway: "",
        device: "test-lan",
        protocol: "kernel",
        scope: "link",
        table: "main",
        metric: 256,
      },
    ],
    dns: {
      nameservers: ["192.0.2.53", "2001:db8::53"],
      searchDomains: ["example.test"],
      source: "/etc/resolv.conf",
    },
    services: [
      {
        name: "duckos-agent.service",
        loadState: "loaded",
        activeState: "active",
        subState: "running",
      },
      {
        name: "synthetic.service",
        loadState: "loaded",
        activeState: "failed",
        subState: "failed",
      },
    ],
    nftables: { status: "available", tables: 2, chains: 4, rules: 17 },
    issues: [],
  };
}
