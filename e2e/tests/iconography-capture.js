// @ts-check
/**
 * Iconography capture — visual proof for the V1 icon system.
 *
 * Renders the real DuckOS UI at multiple viewports, dark + light, and
 * captures:
 *
 *   1. Full sidebar dark
 *   2. Full sidebar light
 *   3. Easy nav dark
 *   4. Easy nav light
 *   5. Interfaces expanded dark
 *   6. Routing expanded dark
 *   7. Firewall/Security expanded dark
 *   8. VPN/Tunnels expanded dark
 *   9. System/Diagnostics expanded dark
 *  10. Icon spec sheet (every canonical icon, labeled)
 *  11. Size comparison sheet (16 / 20 / 24 / 32 px)
 *
 * All screenshots are written to /tmp/icon-shots/.
 */
const { chromium } = require("playwright-core");
const { mkdirSync, writeFileSync } = require("node:fs");
const { resolve } = require("node:path");

const OUT = "/tmp/icon-shots";
mkdirSync(OUT, { recursive: true });

const DEV = "http://127.0.0.1:5173";

// Inject a real session into sessionStorage so the app skips login and
// renders the full UI. We mint the token via the mock backend at 8090.
const SESSION_KEY = "duckos.v1.session.v1";

async function mintSession() {
  const r = await fetch("http://127.0.0.1:8090/api/v1/auth/login", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      username: "admin",
      password: "test-password-12345",
    }),
  });
  const j = await r.json();
  return JSON.stringify({
    token: j.token,
    user: j.user,
    expiresAt: Date.now() + (j.ttl ?? 3600) * 1000,
  });
}

async function gotoFull(page, viewport, theme, session) {
  await page.setViewportSize(viewport);
  await page.addInitScript(
    ({ key, value, theme }) => {
      try {
        sessionStorage.setItem(key, value);
        localStorage.setItem(
          "duckos.v1.prefs.v1",
          JSON.stringify({ theme }),
        );
        document.documentElement.setAttribute("data-theme", theme);
      } catch {}
    },
    { key: SESSION_KEY, value: session, theme },
  );
  await page.goto(DEV + "/system/overview", {
    waitUntil: "networkidle",
    timeout: 30_000,
  });
  await page.waitForTimeout(800);
}

async function main() {
  const browser = await chromium.launch({
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
  });

  const session = await mintSession();

  try {
    // ===== Full sidebar — dark =====
    {
      const page = await browser.newPage();
      await gotoFull(page, { width: 1440, height: 900 }, "dark");
      // Click each section to expand it for the screenshot.
      await page.screenshot({
        path: `${OUT}/01-sidebar-dark.png`,
        fullPage: false,
        clip: { x: 0, y: 0, width: 280, height: 900 },
      });
      await page.close();
    }

    // ===== Full sidebar — light =====
    {
      const page = await browser.newPage();
      await gotoFull(page, { width: 1440, height: 900 }, "light");
      await page.screenshot({
        path: `${OUT}/02-sidebar-light.png`,
        fullPage: false,
        clip: { x: 0, y: 0, width: 280, height: 900 },
      });
      await page.close();
    }

    // ===== Easy nav — dark =====
    {
      const page = await browser.newPage();
      await gotoFull(page, { width: 1440, height: 900 }, "dark");
      await page.evaluate(() => {
        history.pushState({}, "", "/easy/home");
        window.dispatchEvent(new PopStateEvent("popstate"));
      });
      await page.waitForTimeout(500);
      await page.screenshot({
        path: `${OUT}/03-easy-nav-dark.png`,
        clip: { x: 0, y: 0, width: 280, height: 900 },
      });
      await page.close();
    }

    // ===== Easy nav — light =====
    {
      const page = await browser.newPage();
      await gotoFull(page, { width: 1440, height: 900 }, "light");
      await page.evaluate(() => {
        history.pushState({}, "", "/easy/home");
        window.dispatchEvent(new PopStateEvent("popstate"));
      });
      await page.waitForTimeout(500);
      await page.screenshot({
        path: `${OUT}/04-easy-nav-light.png`,
        clip: { x: 0, y: 0, width: 280, height: 900 },
      });
      await page.close();
    }

    // ===== Each section expanded — dark =====
    const sections = [
      ["interfaces", "interfaces"],
      ["wan", "wan"],
      ["lan", "lan"],
      ["routing", "routing"],
      ["firewall", "firewall"],
      ["tunnels", "tunnels"],
      ["dns", "dns-dhcp"],
      ["services", "services"],
      ["system", "system"],
      ["logs", "diagnostics"],
    ];
    let idx = 5;
    for (const [route, label] of sections) {
      const page = await browser.newPage();
      await gotoFull(page, { width: 1440, height: 900 }, "dark");
      await page.evaluate((r) => {
        history.pushState({}, "", "/" + r);
        window.dispatchEvent(new PopStateEvent("popstate"));
      }, route);
      await page.waitForTimeout(700);
      await page.screenshot({
        path: `${OUT}/${String(idx).padStart(2, "0")}-section-${label}-dark.png`,
        clip: { x: 0, y: 0, width: 280, height: 900 },
      });
      await page.close();
      idx++;
    }

    // ===== Icon spec sheet — every canonical icon with its DuckOS label =====
    {
      const html = renderIconSheet();
      const page = await browser.newPage();
      await page.setViewportSize({ width: 1440, height: 1800 });
      await page.goto(DEV + "/__icon-paths.json", { waitUntil: "load" }).catch(() => {});
      await page.setContent(html, { waitUntil: "load" });
      await page.evaluate((theme) => {
        document.documentElement.setAttribute("data-theme", theme);
      }, "dark");
      await page.waitForTimeout(800);
      await page.screenshot({
        path: `${OUT}/15-icon-spec-sheet.png`,
        fullPage: true,
      });
      await page.close();
    }

    // ===== Size comparison sheet =====
    {
      const html = renderSizeSheet();
      const page = await browser.newPage();
      await page.setViewportSize({ width: 1100, height: 900 });
      await page.goto(DEV + "/__icon-paths.json", { waitUntil: "load" }).catch(() => {});
      await page.setContent(html, { waitUntil: "load" });
      await page.evaluate((theme) => {
        document.documentElement.setAttribute("data-theme", theme);
      }, "dark");
      await page.waitForTimeout(800);
      await page.screenshot({
        path: `${OUT}/16-size-comparison.png`,
        fullPage: true,
      });
      await page.close();
    }

    // ===== Wide-viewport sidebar (1920x1080) — dark =====
    {
      const page = await browser.newPage();
      await gotoFull(page, { width: 1920, height: 1080 }, "dark");
      await page.screenshot({
        path: `${OUT}/17-sidebar-dark-1920.png`,
        clip: { x: 0, y: 0, width: 320, height: 1080 },
      });
      await page.close();
    }

    // ===== Light spec sheet =====
    {
      const html = renderIconSheet();
      const page = await browser.newPage();
      await page.setViewportSize({ width: 1440, height: 1800 });
      await page.goto(DEV + "/__icon-paths.json", { waitUntil: "load" }).catch(() => {});
      await page.setContent(html, { waitUntil: "load" });
      await page.evaluate((theme) => {
        document.documentElement.setAttribute("data-theme", theme);
      }, "light");
      await page.waitForTimeout(800);
      await page.screenshot({
        path: `${OUT}/18-icon-spec-sheet-light.png`,
        fullPage: true,
      });
      await page.close();
    }

    const report = {
      generatedAt: new Date().toISOString(),
      screenshots: require("node:fs").readdirSync(OUT).sort(),
    };
    writeFileSync(`${OUT}/capture-report.json`, JSON.stringify(report, null, 2));
    console.log("Captured", report.screenshots.length, "screenshots to", OUT);
  } finally {
    await browser.close();
  }
}

// ---- helpers ----

/**
 * Inline a small page that draws the entire current IconName set as a
 * spec sheet grouped by the same sections as the runtime navigation.
 */
function renderIconSheet() {
  // Each group: section name + (icon name, label) pairs.
  const groups = [
    {
      title: "OVERVIEW",
      items: [
        ["overview", "System Overview"],
        ["topology", "Topology"],
        ["stethoscope", "Doctor"],
        ["storage", "Storage"],
      ],
    },
    {
      title: "NETWORK",
      items: [
        ["ethernet", "Interfaces"],
        ["bridge", "Bridges"],
        ["lacp", "Bonding / LACP"],
        ["loopShield", "Loop Protection"],
        ["discovery", "Discovery / LLDP"],
        ["portSecurity", "Port Security"],
        ["mirroring", "Mirroring / SPAN"],
        ["macsec", "MACsec"],
        ["acceleration", "Acceleration / SR-IOV"],
        ["oam", "L2 Resilience / OAM"],
      ],
    },
    {
      title: "WAN & INTERNET",
      items: [
        ["wan", "WAN Connections"],
        ["modem", "Mobile / USB WAN"],
        ["multiwan", "Multi-WAN"],
        ["pathQuality", "Path Quality / SD-WAN"],
        ["ddns", "DDNS"],
      ],
    },
    {
      title: "LAN & DEVICES",
      items: [
        ["vlan", "Networks / LAN / VLAN"],
        ["ipam", "Address Plan / IPAM"],
        ["devices", "Clients / Devices"],
        ["wifi", "Wi-Fi"],
        ["ipv6", "IPv6 Addressing / PD"],
      ],
    },
    {
      title: "ROUTING",
      items: [
        ["routes", "Routes / RIB / FIB"],
        ["policyRouting", "Policy Routing"],
        ["bgp", "BGP"],
        ["ospf", "OSPF"],
        ["bfd", "BFD"],
        ["routeFilters", "Route Filters"],
        ["rpki", "RPKI / BMP"],
        ["mpls", "MPLS"],
        ["evpn", "VXLAN / EVPN"],
        ["multicast", "Multicast"],
        ["transition", "IPv6 Transition"],
        ["vrf", "VRF"],
        ["redundancy", "Redundancy / VRRP"],
        ["qos", "QoS"],
      ],
    },
    {
      title: "SECURITY",
      items: [
        ["accessPolicies", "Access Policies"],
        ["firewall", "Firewall Rules"],
        ["nat", "NAT / Published Services"],
        ["firewallObjects", "Firewall Objects"],
        ["conntrack", "Connections / Conntrack"],
        ["firewallAnalysis", "Firewall Analysis"],
      ],
    },
    {
      title: "VPN & TUNNELS",
      items: [
        ["wireguard", "WireGuard Servers"],
        ["wireguardPeers", "WireGuard Peers"],
        ["xray", "Xray Servers"],
        ["xrayProfiles", "Xray Profiles"],
        ["mesh", "Other Tunnels / NetBird"],
      ],
    },
    {
      title: "DNS & DHCP",
      items: [
        ["resolver", "DNS Resolver"],
        ["dns", "DNS Zones"],
        ["policyDns", "Policy DNS / RPZ"],
        ["dhcpScopes", "DHCP Scopes"],
        ["leases", "Leases / Reservations"],
      ],
    },
    {
      title: "SERVICES",
      items: [
        ["services", "Services / Listeners"],
        ["sites", "Sites / Reverse Proxy"],
        ["certificates", "Certificates / ACME"],
        ["pki", "Internal PKI / SSH CA"],
        ["aaa", "AAA / RADIUS / 802.1X"],
        ["time", "Time / NTP"],
        ["telemetry", "Telemetry / Integrations"],
        ["upnp", "UPnP / NAT-PMP"],
        ["portal", "Portal"],
        ["subscriptions", "Subscriptions"],
        ["ha", "Sites / HA"],
        ["ssh", "Provisioning / ZTP"],
        ["cron", "Time and Clock"],
        ["ups", "Power and UPS"],
        ["hardware", "Hardware and Sensors"],
        ["license", "License"],
      ],
    },
    {
      title: "SYSTEM / DIAGNOSTICS",
      items: [
        ["users", "Users and Roles"],
        ["security", "Privacy"],
        ["updates", "Updates and Upgrades"],
        ["backup", "Backup and Restore"],
        ["history", "Configuration History"],
        ["migrate", "Migrations"],
        ["audit", "Audit"],
        ["logs", "Logs / Live Tail"],
        ["events", "Events / Webhooks"],
        ["explain", "Explain Why"],
        ["alerts", "Incidents"],
        ["monitor", "Monitoring"],
        ["speed", "Speed Lab"],
        ["tools", "Network Tools"],
        ["lab", "Lab"],
        ["guide", "Migration Help"],
        ["api", "API Surface"],
      ],
    },
    {
      title: "EASY MODE",
      items: [
        ["home", "Home"],
        ["internet", "Internet"],
        ["wifi", "Networks / Wi-Fi"],
        ["devices", "Devices / Access"],
        ["vpn", "Publish a service"],
        ["system", "Care / Recovery"],
        ["router", "Router (card)"],
        ["stethoscope", "Doctor (card)"],
      ],
    },
    {
      title: "UI AFFORDANCES",
      items: [
        ["search", "Search"],
        ["sun", "Light theme"],
        ["moon", "Dark theme"],
        ["globe", "Locale"],
        ["help", "Help"],
        ["lock", "Lock"],
        ["logout", "Sign out"],
        ["refresh", "Refresh"],
        ["warn", "Warn"],
        ["info", "Info"],
        ["alert", "Alerts"],
      ],
    },
  ];

  const groupsHtml = groups
    .map((g) => {
      const cells = g.items
        .map(([name, label]) => {
          // Each cell is a real <Icon> from the running app via Vite.
          // We can't import TSX here, so we inline the SVG paths by
          // calling the resolved icon. Instead we draw using <img>
          // tags that reference the live dev URL and a stable path.
          return `
            <div class="cell">
              <div class="ic" data-icon="${name}"></div>
              <div class="lbl">${label}</div>
              <div class="nm">${name}</div>
            </div>`;
        })
        .join("");
      return `
        <section class="group">
          <h2>${g.title}</h2>
          <div class="row">${cells}</div>
        </section>`;
    })
    .join("");

  return `<!doctype html>
<html><head><meta charset="utf-8"><title>DuckOS Icon Set</title>
<style>
  :root {
    color-scheme: light dark;
    --bg: #0f1620;
    --bg-card: #1a2332;
    --fg: #d8e1ec;
    --muted: #8a99ad;
    --accent: #4fd1c5;
    --border: #2a3a52;
  }
  html[data-theme="light"] {
    --bg: #f6f8fb;
    --bg-card: #ffffff;
    --fg: #1a2332;
    --muted: #5a6878;
    --accent: #2c9b91;
    --border: #d6dde7;
  }
  body { margin: 0; padding: 32px; background: var(--bg); color: var(--fg); font: 14px -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; }
  header h1 { font-size: 26px; margin: 0 0 4px; }
  header p { margin: 0; color: var(--muted); }
  section.group { background: var(--bg-card); border: 1px solid var(--border); border-radius: 12px; padding: 18px 22px; margin-bottom: 18px; }
  section.group h2 { font-size: 12px; letter-spacing: 0.08em; margin: 0 0 14px; color: var(--muted); }
  .row { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
  .cell { background: var(--bg); border: 1px solid var(--border); border-radius: 8px; padding: 12px 10px; text-align: center; }
  .cell .ic { display: flex; align-items: center; justify-content: center; height: 36px; }
  .cell .ic svg { width: 24px; height: 24px; stroke: var(--fg); fill: none; stroke-width: 1.6; }
  .cell .lbl { font-size: 12px; margin-top: 6px; }
  .cell .nm { font-size: 10px; color: var(--muted); margin-top: 2px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
</style>
</head>
<body>
<header><h1>DuckOS — V1 Icon Set</h1><p>Real SVG icons from the running app. 24 px grid · 1.6 stroke · round caps · single currentColor family.</p></header>
${groupsHtml}
<script>
// After the document is parsed, reach into the dev server to fetch each
// icon's SVG via a known stable URL pattern. The dev server returns the
// running app's icon paths through the React tree; we instead fetch a
// tiny dedicated route that renders each requested IconName into the DOM.
//
// To keep this self-contained without spinning up a second build, we
// render the SVG markup directly here using the same paths the icons
// library ships — fetched from the dev server via JSON.
const ICON_JSON_URL = '/__icon-paths.json';
async function loadPaths() {
  const r = await fetch(ICON_JSON_URL);
  if (!r.ok) throw new Error('icon paths fetch failed');
  return r.json();
}
loadPaths().then((paths) => {
  document.querySelectorAll('[data-icon]').forEach((el) => {
    const name = el.dataset.icon;
    const path = paths[name];
    if (!path) return;
    el.innerHTML = '<svg viewBox="0 0 24 24">' + path + '</svg>';
  });
}).catch(() => {/* leave blank if fetch fails */});
</script>
</body></html>`;
}

function renderSizeSheet() {
  const sizes = [16, 20, 24, 32];
  const icons = [
    "ethernet",
    "bridge",
    "lacp",
    "loopShield",
    "discovery",
    "portSecurity",
    "mirroring",
    "macsec",
    "acceleration",
    "oam",
    "wan",
    "modem",
    "multiwan",
    "pathQuality",
    "ddns",
    "vlan",
    "ipam",
    "devices",
    "wifi",
    "ipv6",
    "routes",
    "policyRouting",
    "bgp",
    "ospf",
    "vrf",
    "qos",
    "firewall",
    "nat",
    "wireguard",
    "xray",
    "resolver",
    "dns",
    "services",
    "stethoscope",
    "topology",
  ];

  const headerCells = sizes
    .map((s) => `<th>${s}×${s}</th>`)
    .join("");
  const body = icons
    .map((name) => {
      const cells = sizes
        .map((s) => {
          return `<td><div class="ic" data-icon="${name}" data-size="${s}"></div></td>`;
        })
        .join("");
      return `<tr><th class="nm">${name}</th>${cells}</tr>`;
    })
    .join("");

  return `<!doctype html>
<html><head><meta charset="utf-8"><title>DuckOS Icon Size Comparison</title>
<style>
  :root { color-scheme: light dark; --bg:#0f1620; --fg:#d8e1ec; --border:#2a3a52; --muted:#8a99ad; }
  html[data-theme="light"] { --bg:#f6f8fb; --fg:#1a2332; --border:#d6dde7; --muted:#5a6878; }
  body { margin: 0; padding: 32px; background: var(--bg); color: var(--fg); font: 13px -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; }
  table { border-collapse: collapse; }
  th, td { padding: 10px 14px; border-bottom: 1px solid var(--border); text-align: center; vertical-align: middle; }
  th.nm { text-align: left; font-family: ui-monospace, monospace; font-size: 11px; color: var(--muted); }
  th { font-size: 11px; color: var(--muted); font-weight: 500; }
  .ic { display: flex; align-items: center; justify-content: center; }
  .ic svg { stroke: var(--fg); stroke-width: 1.6; fill: none; }
</style>
</head>
<body>
<h1>DuckOS Icon Size Comparison</h1>
<p>16 / 20 / 24 / 32 px — primary sidebar sizes.</p>
<table>
  <thead><tr><th></th>${headerCells}</tr></thead>
  <tbody>${body}</tbody>
</table>
<script>
fetch('/__icon-paths.json').then((r) => r.json()).then((paths) => {
  document.querySelectorAll('[data-icon]').forEach((el) => {
    const name = el.dataset.icon;
    const size = parseInt(el.dataset.size, 10);
    const p = paths[name];
    if (!p) return;
    el.innerHTML = '<svg viewBox="0 0 24 24" width="' + size + '" height="' + size + '">' + p + '</svg>';
  });
});
</script>
</body></html>`;
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});