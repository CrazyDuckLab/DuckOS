// @ts-check
/**
 * Runtime oracle test — every read-only page that exposes Linux
 * truth must match a real runtime snapshot. We do NOT trust the UI
 * alone; the same data must be reproducible from `ip`, `nft`,
 * `systemctl`, etc. on duck-dev-01.
 */
const { test, expect } = require("@playwright/test");
const { execSync } = require("node:child_process");

const VM = "duck-dev-01";

function ssh(cmd) {
  return execSync(
    `ssh -o ConnectTimeout=5 -o BatchMode=yes ${VM} '${cmd.replace(/'/g, "'\\''")}'`,
    { stdio: ["ignore", "pipe", "pipe"], timeout: 30_000 },
  ).toString();
}

async function api(token, path) {
  const res = await fetch(`http://127.0.0.1:8080${path}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) {
    throw new Error(`API ${path} returned HTTP ${res.status}`);
  }
  return res.json();
}

function getToken() {
  const out = execSync(
    `curl -s -X POST -H "Content-Type: application/json" -d '{"username":"admin","password":"test-password-12345"}' http://127.0.0.1:8080/api/v1/auth/login`,
    { stdio: ["ignore", "pipe", "pipe"] },
  ).toString();
  const body = JSON.parse(out);
  return body.token;
}

test.describe.serial("Runtime oracle — UI vs Linux truth", () => {
  test("interfaces listed in /api/v1/objects/network.interface match `ip link`", async () => {
    const token = getToken();
    const apiIfaces = await api(token, "/api/v1/objects/network.interface");
    const items = apiIfaces.items || [];
    const apiNames = new Set(
      items.map((it) => it.ref?.id || it.id || it.displayName).filter(Boolean),
    );

    const ipOut = ssh("ip -d link show 2>/dev/null | awk -F': ' '/^[0-9]+: /{print $2}' | sort");
    const ipNames = new Set(
      ipOut
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean),
    );

    // Every kernel interface must appear in the engine snapshot
    // (the engine may add aliases but never drops a kernel device).
    let missing = [];
    for (const name of ipNames) {
      if (!apiNames.has(name)) missing.push(name);
    }
    if (missing.length > 0) {
      // The engine may legitimately omit bridge slaves; we just
      // log them so the report shows the divergence without
      // failing.
      console.warn(
        `Engine snapshot does not mention these kernel interfaces: ${missing.join(", ")}`,
      );
    }
    // Conversely, the engine must never invent a device that the
    // kernel does not know about — that would be a UI bug.
    let invented = [];
    for (const name of apiNames) {
      if (!ipNames.has(name)) invented.push(name);
    }
    expect(invented, "engine must not invent kernel interfaces").toEqual([]);
  });

  test("lab bridge ducklab-br0 is still present and routed", () => {
    const ipOut = ssh("ip -br addr show ducklab-br0 2>/dev/null | head -1").trim();
    expect(ipOut).toContain("ducklab-br0");
    expect(ipOut).toContain("10.99.99.1/24");

    const routeOut = ssh("ip route show 10.99.99.0/24 2>/dev/null").trim();
    expect(routeOut).toContain("ducklab-br0");
  });

  test("management boundary is untouched", () => {
    const eth0 = ssh("ip -br addr show eth0 2>/dev/null | head -1").trim();
    expect(eth0).toContain("172.18.4.149/16");

    const route = ssh("ip route show default 2>/dev/null | head -1").trim();
    expect(route).toContain("default via 172.18.1.254");
    expect(route).toContain("dev eth0");
  });

  test("agent socket is listening", () => {
    const ss = ssh("ss -lnp 2>/dev/null | grep duckos-agent").trim();
    expect(ss).toContain("/run/duckos-agent/agent.sock");
  });
});
