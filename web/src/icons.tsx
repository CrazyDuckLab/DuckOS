import type { ReactNode } from "react";

/**
 * DuckOS V1 — Icon library.
 *
 * One consistent stroke-based 24-grid SVG icon set. Each icon is
 * designed to read at 16–24 px in a sidebar and to remain recognisable
 * in tooltips, table cells, and inspector panels.
 *
 * Pages may declare `icon: "wan"` (etc.) in the registry; the SideRail
 * resolves the icon by name and falls back to `network` if a page
 * references an icon we haven't drawn yet.
 *
 * Family rules (must remain consistent across the set):
 *  - 24×24 viewBox
 *  - stroke-based, currentColor
 *  - 1.6 stroke width (renders cleanly at 16 px)
 *  - round line caps, round line joins
 *  - same visual mass: avoid huge filled blocks next to fine outlines
 *  - default fill = "none"; small `<circle r="…" fill="currentColor">`
 *    is allowed only for emphasis dots inside a glyph.
 *
 * Each icon below is named for the FUNCTION, not the parent category.
 * Neighbouring nav entries should not share the same glyph; the
 * `icons-distinctness.test.tsx` test enforces this.
 */
export type IconName =
  // — Overview / System —
  | "overview"
  | "topology"
  | "doctor"
  | "stethoscope"
  | "storage"
  | "monitor"
  | "alerts"
  | "events"
  | "logs"
  | "audit"
  | "explain"
  | "guide"
  | "tools"
  | "speed"
  | "lab"
  | "templates"
  | "history"
  | "updates"
  | "backup"
  | "migrate"
  | "users"
  | "api"
  | "security"

  // — Network (Layer 2 / Interfaces) —
  | "ethernet"
  | "bridge"
  | "lacp"
  | "loopShield"
  | "discovery"
  | "portSecurity"
  | "mirroring"
  | "macsec"
  | "acceleration"
  | "oam"

  // — WAN & Internet —
  | "wan"
  | "modem"
  | "multiwan"
  | "pathQuality"
  | "ddns"
  | "internet"

  // — LAN & Devices —
  | "vlan"
  | "ipam"
  | "devices"
  | "wifi"
  | "ipv6"

  // — Routing —
  | "routes"
  | "policyRouting"
  | "bgp"
  | "ospf"
  | "bfd"
  | "routeFilters"
  | "rpki"
  | "mpls"
  | "evpn"
  | "multicast"
  | "transition"
  | "vrf"
  | "redundancy"
  | "qos"

  // — Firewall / Security —
  | "accessPolicies"
  | "firewall"
  | "nat"
  | "firewallObjects"
  | "conntrack"
  | "firewallAnalysis"
  | "ids"

  // — VPN / Tunnels —
  | "wireguard"
  | "wireguardPeers"
  | "xray"
  | "xrayProfiles"
  | "mesh"

  // — DNS & DHCP —
  | "resolver"
  | "dns"
  | "policyDns"
  | "dhcpScopes"
  | "leases"

  // — Services —
  | "services"
  | "sites"
  | "certificates"
  | "pki"
  | "aaa"
  | "time"
  | "telemetry"
  | "upnp"
  | "portal"
  | "subscriptions"
  | "ha"
  | "ssh"
  | "cron"
  | "ups"
  | "hardware"
  | "license"

  // — Easy / overview-mode cards —
  | "home"
  | "system"
  | "router"
  | "care"
  | "publish"

  // — Generic fallbacks (kept for backward-compat with legacy code) —
  | "network"
  | "lan"
  | "activity"
  | "policy"
  | "tree"
  | "feed"
  | "site"
  | "audit"
  | "capture"
  | "vpn"
  | "tunnel"
  | "user"
  | "clock"
  | "pulse"
  | "timeline"
  | "template"
  | "candidate"
  | "cpu"
  | "power"
  | "alert"
  | "log"
  | "key"

  // — UI affordances —
  | "search"
  | "lock"
  | "logout"
  | "refresh"
  | "arrow"
  | "chevron"
  | "help"
  | "menu"
  | "close"
  | "check"
  | "warn"
  | "plus"
  | "minus"
  | "edit"
  | "trash"
  | "more"
  | "external"
  | "sun"
  | "moon"
  | "globe"
  | "info"
  | "monitor";

/* Common emphasised dots — fill=currentColor, no stroke.
 * Use sparingly so icons remain stroke-based. */
const dot = (cx: number, cy: number, r = 0.7) => (
  <circle
    key={`${cx}.${cy}`}
    cx={cx}
    cy={cy}
    r={r}
    fill="currentColor"
    stroke="none"
  />
);

/* SVG paths for every IconName. */
const paths: Record<IconName, ReactNode> = {
  // — Overview / System —
  /* Gauge / dial — System Overview */
  overview: (
    <>
      <path d="M5 16a8 8 0 1 1 14 0" />
      <path d="m12 11 4 4" />
      <circle cx="12" cy="16" r="0.8" fill="currentColor" stroke="none" />
    </>
  ),
  /* Topology — three-node graph */
  topology: (
    <>
      <circle cx="12" cy="5" r="2" />
      <circle cx="5" cy="19" r="2" />
      <circle cx="19" cy="19" r="2" />
      <path d="M12 7v4M12 11l-5 6M12 11l5 6" />
    </>
  ),
  /* Generic plus-in-circle (legacy fallback) — kept for compatibility */
  doctor: (
    <>
      <path d="M12 3v18M3 12h18" />
      <circle cx="12" cy="12" r="9" />
    </>
  ),
  /* Stethoscope — Doctor. Distinct from the generic cross */
  stethoscope: (
    <>
      <path d="M5 3v6a4 4 0 0 0 8 0V3" />
      <path d="M7 3v4M11 3v4" />
      <path d="M9 13v3a3 3 0 0 0 6 0v-2" />
      <circle cx="15" cy="13" r="2" />
    </>
  ),
  /* Hard drive — Storage */
  storage: (
    <>
      <path d="m5 4-2 9v6a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-6l-2-9z" />
      <path d="M3 13h18M7 17h.01M11 17h.01" />
    </>
  ),
  /* Monitor — bar-chart inside a screen. Was the old "monitor" generic. */
  monitor: (
    <>
      <rect x="3" y="4" width="18" height="14" rx="2" />
      <path d="M7 14l3-3 3 3 4-5" />
      <path d="M9 21h6M12 18v3" />
    </>
  ),
  /* Alerts — warning triangle */
  alerts: (
    <>
      <path d="m10.3 4-8 14a2 2 0 0 0 1.7 3h16a2 2 0 0 0 1.7-3l-8-14a2 2 0 0 0-3.4 0Z" />
      <path d="M12 9v4m0 4h.01" />
    </>
  ),
  /* Events — timeline with emphasised dots */
  events: (
    <>
      <path d="M3 6h12M3 12h18M3 18h8" />
      {dot(18, 6, 1)}
      {dot(21, 12, 1)}
      {dot(15, 18, 1)}
    </>
  ),
  /* Logs — document with text lines */
  logs: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M8 8h8M8 12h8M8 16h5" />
    </>
  ),
  /* Audit — magnifier with clock inside */
  audit: (
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="m21 21-5-5" />
      <path d="M11 8v3l2 1" />
    </>
  ),
  /* Explain — speech bubble with question mark */
  explain: (
    <>
      <path d="M4 5h12l4 4v10a2 2 0 0 1-2 2H4z" />
      <path d="M16 5v4h4" />
      <path d="M9 12a3 3 0 0 1 6 0c0 1.5-3 2-3 4" />
      <circle cx="12" cy="18" r="0.6" fill="currentColor" stroke="none" />
    </>
  ),
  /* Guide — book */
  guide: (
    <>
      <path d="M4 5h12a4 4 0 0 1 4 4v10H8a4 4 0 0 1-4-4z" />
      <path d="M4 5v10a4 4 0 0 0 4 4" />
    </>
  ),
  /* Tools — wrench */
  tools: (
    <>
      <path d="M14 4a4 4 0 0 1 4 6l-9 9-4-4 9-9z" />
      <path d="m5 19 4 4" />
    </>
  ),
  /* Speed — gauge (Speed Lab) */
  speed: (
    <>
      <path d="M12 14a2 2 0 1 0 2-2" />
      <path d="M4 19a14 14 0 0 1 16 0" />
      <path d="M7 16a9 9 0 0 1 10 0" />
    </>
  ),
  /* Lab — flask */
  lab: (
    <>
      <path d="M9 3h6" />
      <path d="M10 3v6L5 19a2 2 0 0 0 1.8 3h10.4A2 2 0 0 0 19 19l-5-10V3" />
      <path d="M7 14h10" />
    </>
  ),
  /* Templates — grid + layout */
  templates: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M3 9h18M9 4v16" />
    </>
  ),
  /* History — clock with rewind arrow */
  history: (
    <>
      <path d="M3 12a9 9 0 1 0 3-6.7" />
      <path d="M3 4v5h5" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  /* Updates — download arrow */
  updates: (
    <>
      <path d="M12 4v12" />
      <path d="m7 9 5-5 5 5" />
      <path d="M4 20h16" />
    </>
  ),
  /* Backup — cylinder */
  backup: (
    <>
      <ellipse cx="12" cy="6" rx="8" ry="2.5" />
      <path d="M4 6v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V6" />
      <path d="M4 11c2 1.6 5 2.5 8 2.5s6-.9 8-2.5" />
    </>
  ),
  /* Migrate — source arrow + destination arrow */
  migrate: (
    <>
      <path d="M3 7h10v10H3z" />
      <path d="m13 7 8 5-8 5" />
      <path d="M21 12H13" />
    </>
  ),
  /* Users — head + shoulders */
  users: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21a8 8 0 0 1 16 0" />
    </>
  ),
  /* API — code brackets in a card */
  api: (
    <>
      <rect x="4" y="4" width="16" height="16" rx="2" />
      <path d="M9 9h6M9 15h4M9 12h6" />
    </>
  ),
  /* Security — shield with check */
  security: (
    <>
      <path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z" />
      <path d="m9 12 2 2 4-4" />
    </>
  ),

  // — Network (Layer 2 / Interfaces) —
  /* Ethernet port — used for Interfaces (P007) so it differs from bridges / bonds */
  ethernet: (
    <>
      <rect x="3" y="8" width="18" height="8" rx="1.5" />
      <path d="M7 12h2M11 12h2M15 12h2" />
    </>
  ),
  /* Bridges — two segments joined by a cross-link */
  bridge: (
    <>
      <path d="M3 7v10M21 7v10" />
      <path d="M3 12h4M9 8v8M15 8v8M17 12h4" />
    </>
  ),
  /* Bonding / LACP — two links chained together */
  lacp: (
    <>
      <rect x="3" y="9" width="7" height="6" rx="1.2" />
      <rect x="14" y="9" width="7" height="6" rx="1.2" />
      <path d="M10 12h4" />
      {dot(12, 12, 1.2)}
    </>
  ),
  /* Loop protection — loop arrows crossed with a shield bar */
  loopShield: (
    <>
      <path d="M5 12a7 7 0 0 1 12-5" />
      <path d="M14 4l3 1-1 3" />
      <path d="M19 12a7 7 0 0 1-12 5" />
      <path d="M10 20l-3-1 1-3" />
      <path d="M4 12h16" />
    </>
  ),
  /* Discovery / LLDP — radar waves emanating outward */
  discovery: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 12a4 4 0 0 1 4-4M12 12a7 7 0 0 1 7-7" />
      <path d="m16 16 3-3 3 3" />
    </>
  ),
  /* Port Security — RJ45 port with a small shield */
  portSecurity: (
    <>
      <rect x="3" y="9" width="13" height="6" rx="1.2" />
      <path d="M6 12h2M9 12h2M12 12h2" />
      <path d="m17 13 2 2-2 2" />
      <path d="M19 11v8" />
    </>
  ),
  /* Mirroring / SPAN — one path split into two */
  mirroring: (
    <>
      <path d="M3 12h5" />
      <path d="M11 12h5" />
      <path d="m13 9 4 3-4 3" />
      <path d="M19 4v6M19 14v6" />
      <path d="M16 10l3-3 3 3" />
      <path d="M16 14l3 3 3-3" />
    </>
  ),
  /* MACsec — cable with padlock */
  macsec: (
    <>
      <rect x="3" y="10" width="13" height="6" rx="1.2" />
      <path d="M6 13h2M9 13h2M12 13h2" />
      <rect x="16" y="13" width="6" height="6" rx="1" />
      <path d="M17 13v-2a2 2 0 0 1 4 0v2" />
    </>
  ),
  /* Acceleration / SR-IOV — chip with VFs fanning out */
  acceleration: (
    <>
      <rect x="8" y="8" width="8" height="8" rx="1.5" />
      <rect x="10" y="10" width="4" height="4" rx="0.6" />
      <path d="M10 8V5m4 3V5M10 16v3m4-3v3M8 10H5m3 4H5m11-4h3m-3 4h3" />
    </>
  ),
  /* L2 Resilience / OAM — heartbeat across a link */
  oam: (
    <>
      <path d="M3 12h3l2-5 3 12 3-9 2 4h5" />
    </>
  ),

  // — WAN & Internet —
  /* WAN — globe with meridians */
  wan: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
    </>
  ),
  /* Modem — Mobile / USB WAN */
  modem: (
    <>
      <rect x="6" y="13" width="12" height="7" rx="1.4" />
      <path d="M9 13v-2M15 13v-2" />
      {dot(9, 16.5)}
      {dot(15, 16.5)}
      <path d="M11 5h2M12 5v6" />
    </>
  ),
  /* Multi-WAN — two uplinks converging into one trunk */
  multiwan: (
    <>
      {dot(5, 7, 1.6)}
      {dot(5, 17, 1.6)}
      {dot(19, 12, 1.6)}
      <path d="M6 7l12 5M6 17l12-5" />
    </>
  ),
  /* Path quality — waveform (SD-WAN) */
  pathQuality: (
    <>
      <path d="M3 12h2l2-6 3 12 3-9 2 5 2-2h4" />
    </>
  ),
  /* DDNS — globe + refresh arrow */
  ddns: (
    <>
      <circle cx="12" cy="12" r="8" />
      <path d="M4 12h16M12 4a14 14 0 0 1 0 16M12 4a14 14 0 0 0 0 16" />
      <path d="m18 4 3 2-2 2" />
      <path d="M21 6a8 8 0 0 1-1 3" />
    </>
  ),
  /* Internet — external uplink arrow (Easy-mode + standalone) */
  internet: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
      <path d="M12 8v6M9 11l3-3 3 3" />
    </>
  ),

  // — LAN & Devices —
  /* VLAN — segmented LAN/switch */
  vlan: (
    <>
      <rect x="3" y="9" width="18" height="6" rx="1.5" />
      <path d="M7 9V6M17 9V6M7 15v3M17 15v3M12 9V6" />
    </>
  ),
  /* IPAM — address plan: blocks stacked */
  ipam: (
    <>
      <rect x="3" y="4" width="18" height="4" rx="1" />
      <rect x="3" y="10" width="11" height="4" rx="1" />
      <rect x="3" y="16" width="6" height="4" rx="1" />
      <path d="M15 12h6M16 18h5" />
    </>
  ),
  /* Devices — laptop + phone */
  devices: (
    <>
      <rect x="3" y="6" width="11" height="9" rx="1.2" />
      <path d="M5 18h7" />
      <rect x="15" y="9" width="6" height="11" rx="1.2" />
      <path d="M17 18h2" />
    </>
  ),
  /* Wi-Fi — radio waves */
  wifi: (
    <>
      <path d="M5 12a10 10 0 0 1 14 0" />
      <path d="M8 15a6 6 0 0 1 8 0" />
      {dot(12, 19, 1.4)}
    </>
  ),
  /* IPv6 — colon-ipv6 marker with prefix */
  ipv6: (
    <>
      <rect x="3" y="9" width="18" height="6" rx="1.5" />
      <path d="M7 12h2M10 12h2M13 12h2" />
      <path d="M16 12h2" />
      <path d="M6 7h12M6 17h12" />
    </>
  ),

  // — Routing —
  /* Routes — generic routing signpost */
  routes: (
    <>
      <circle cx="5" cy="5" r="2" />
      <circle cx="19" cy="19" r="2" />
      <path d="M5 7v8a4 4 0 0 0 4 4h8M11 5h8v8M13 11l6-6" />
    </>
  ),
  /* Policy Routing — route plus a branching policy line */
  policyRouting: (
    <>
      <circle cx="5" cy="5" r="2" />
      <circle cx="19" cy="19" r="2" />
      <path d="M5 7v8a4 4 0 0 0 4 4h8M11 5h8v8" />
      <path d="M9 11h12M17 11l4-4M17 11l4 4" />
    </>
  ),
  /* BGP — two peers exchanging routes */
  bgp: (
    <>
      <circle cx="5" cy="12" r="2.2" />
      <circle cx="19" cy="12" r="2.2" />
      <path d="M7 12h10M11 9l-3 3 3 3M13 9l3 3-3 3" />
    </>
  ),
  /* OSPF — several nodes forming an area */
  ospf: (
    <>
      <path d="M12 4 4 9v6l8 5 8-5V9z" />
      <circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none" />
      <path d="M12 4v16M4 9l16 6M4 15l16-6" />
    </>
  ),
  /* BFD — peer link + heartbeat */
  bfd: (
    <>
      <circle cx="5" cy="12" r="1.8" />
      <circle cx="19" cy="12" r="1.8" />
      <path d="M7 12h2l2-5 2 10 2-7 2 4h2" />
    </>
  ),
  /* Route filters — funnel over a route */
  routeFilters: (
    <>
      <path d="M4 4h16l-6 8v6l-4 2v-8z" />
      <path d="M3 12h18" />
    </>
  ),
  /* RPKI — route + check shield */
  rpki: (
    <>
      <circle cx="5" cy="5" r="2" />
      <circle cx="19" cy="19" r="2" />
      <path d="M5 7v8a4 4 0 0 0 4 4h8M11 5h8v8" />
      <path d="m14 14 2 2 4-4" />
    </>
  ),
  /* MPLS — stacked labels */
  mpls: (
    <>
      <rect x="3" y="6" width="14" height="4" rx="1" />
      <rect x="6" y="10" width="14" height="4" rx="1" />
      <rect x="9" y="14" width="12" height="4" rx="1" />
      <path d="M17 8h4M20 12h1M21 16h0" />
    </>
  ),
  /* EVPN/VXLAN — overlay tunnel */
  evpn: (
    <>
      <rect x="3" y="9" width="14" height="6" rx="1.5" />
      <rect x="10" y="13" width="11" height="6" rx="1.5" />
      <path d="M5 12h2M8 12h2M12 16h2M15 16h2" />
    </>
  ),
  /* Multicast — one sender → many receivers */
  multicast: (
    <>
      <circle cx="12" cy="5" r="2" />
      <circle cx="5" cy="19" r="2" />
      <circle cx="12" cy="19" r="2" />
      <circle cx="19" cy="19" r="2" />
      <path d="M12 7v4M12 11l-5 6M12 11v6M12 11l5 6" />
    </>
  ),
  /* IPv6 Transition — bridge between two protocols */
  transition: (
    <>
      <path d="M3 12h6" />
      <path d="M15 12h6" />
      <path d="m9 9 3 3-3 3" />
      <path d="m15 9-3 3 3 3" />
      <rect x="6" y="9" width="3" height="6" rx="1" />
      <rect x="15" y="9" width="3" height="6" rx="1" />
    </>
  ),
  /* VRF — layered isolated routing tables */
  vrf: (
    <>
      <path d="M3 6h18M3 11h18M3 16h18M3 21h18" />
      <circle cx="6" cy="6" r="0.7" fill="currentColor" stroke="none" />
      <circle cx="9" cy="11" r="0.7" fill="currentColor" stroke="none" />
      <circle cx="14" cy="16" r="0.7" fill="currentColor" stroke="none" />
      <circle cx="17" cy="21" r="0.7" fill="currentColor" stroke="none" />
    </>
  ),
  /* Redundancy / VRRP — two routers synced */
  redundancy: (
    <>
      <rect x="3" y="8" width="8" height="8" rx="1.2" />
      <path d="M5 11h4M5 14h4" />
      <rect x="13" y="8" width="8" height="8" rx="1.2" />
      <path d="M15 11h4M15 14h4" />
      <path d="M11 12h2" />
      <path d="m12 9 2 2-2 2" />
      <path d="m12 13-2 2 2 2" />
    </>
  ),
  /* QoS — priority bars */
  qos: (
    <>
      <path d="M5 19V13M9 19V9M13 19V5M17 19V11M21 19V7" />
      <path d="M3 21h18" />
    </>
  ),

  // — Firewall / Security —
  /* Access Policies — list with lock at top */
  accessPolicies: (
    <>
      <rect x="6" y="3" width="12" height="10" rx="1.5" />
      <path d="M9 6h6M9 9h4" />
      <path d="M4 16h16M4 20h10" />
    </>
  ),
  /* Brick wall — Firewall. Distinct from the generic shield */
  firewall: (
    <>
      <rect x="3" y="5" width="6" height="4" />
      <rect x="9" y="5" width="6" height="4" />
      <rect x="15" y="5" width="6" height="4" />
      <rect x="5" y="9" width="6" height="4" />
      <rect x="11" y="9" width="6" height="4" />
      <rect x="17" y="9" width="2" height="4" />
      <rect x="3" y="13" width="6" height="4" />
      <rect x="9" y="13" width="6" height="4" />
      <rect x="15" y="13" width="6" height="4" />
      <rect x="5" y="17" width="6" height="4" />
      <rect x="11" y="17" width="6" height="4" />
      <rect x="17" y="17" width="6" height="4" />
    </>
  ),
  /* NAT — translation arrows through a box */
  nat: (
    <>
      <path d="M3 8h8v8H3z" />
      <path d="M11 12h6" />
      <path d="m14 9 3 3-3 3" />
      <path d="M16 5v2M16 17v2" />
    </>
  ),
  /* Firewall Objects — grouped/feeds (curly brackets) */
  firewallObjects: (
    <>
      <path d="M9 5H6a2 2 0 0 0-2 2v3a2 2 0 0 1-2 2 2 2 0 0 1 2 2v3a2 2 0 0 0 2 2h3" />
      <path d="M15 5h3a2 2 0 0 1 2 2v3a2 2 0 0 0 2 2 2 2 0 0 0-2 2v3a2 2 0 0 1-2 2h-3" />
    </>
  ),
  /* Connections / Conntrack — bidirectional arrows */
  conntrack: (
    <>
      <path d="M3 9h13" />
      <path d="m13 6 4 3-4 3" />
      <path d="M21 15H8" />
      <path d="m11 12-4 3 4 3" />
    </>
  ),
  /* Firewall Analysis — shield with pulse */
  firewallAnalysis: (
    <>
      <path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z" />
      <path d="M7 13h2l2-4 2 8 2-6 2 4" />
    </>
  ),
  /* IDS — defensive shield + detection radar dot */
  ids: (
    <>
      <path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z" />
      <circle cx="12" cy="11" r="2" />
      <path d="M9 16l3-3 3 3" />
    </>
  ),

  // — VPN / Tunnels —
  /* WireGuard — secure tunnel with key */
  wireguard: (
    <>
      <path d="M3 14a9 9 0 0 1 18 0" />
      <path d="M3 14v3M21 14v3" />
      <circle cx="12" cy="14" r="2" />
      <path d="M12 14v-3M11 11l1-1 1 1" />
    </>
  ),
  /* WireGuard Peers — two peer nodes connected */
  wireguardPeers: (
    <>
      <circle cx="5" cy="12" r="2.2" />
      <circle cx="19" cy="12" r="2.2" />
      <path d="M7 12h10" />
      <circle cx="12" cy="12" r="1.4" />
      <path d="M11 11l1-1 1 1" />
    </>
  ),
  /* Xray — directional arrow piercing a barrier */
  xray: (
    <>
      <path d="M3 8h13" />
      <path d="m13 5 4 3-4 3" />
      <path d="M3 16h7" />
      <path d="m7 13 3 3-3 3" />
      <path d="M14 14h7v6h-7" />
    </>
  ),
  /* Xray Profiles — user + tunnel */
  xrayProfiles: (
    <>
      <circle cx="9" cy="8" r="3" />
      <path d="M3 20a6 6 0 0 1 12 0" />
      <path d="M14 14a5 5 0 0 1 7 0" />
      <path d="M16 14v3" />
    </>
  ),
  /* Other Tunnels — generic mesh */
  mesh: (
    <>
      <circle cx="6" cy="6" r="2" />
      <circle cx="18" cy="6" r="2" />
      <circle cx="6" cy="18" r="2" />
      <circle cx="18" cy="18" r="2" />
      <path d="M8 6h8M8 18h8M6 8v8M18 8v8M8 8l8 8M16 8l-8 8" />
    </>
  ),

  // — DNS & DHCP —
  /* DNS Resolver — globe + magnifier */
  resolver: (
    <>
      <circle cx="11" cy="11" r="6" />
      <path d="M5 11h12M11 5a10 10 0 0 1 0 12M11 5a10 10 0 0 0 0 12" />
      <path d="m17 17 4 4" />
    </>
  ),
  /* DNS — globe with hierarchy */
  dns: (
    <>
      <circle cx="12" cy="12" r="9" />
      <ellipse cx="12" cy="12" rx="4" ry="9" />
      <path d="M3 12h18" />
    </>
  ),
  /* Policy DNS — globe + shield */
  policyDns: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18" />
      <path d="M12 8v4l3 2" />
      <path d="m9 17 3 3 3-3" />
    </>
  ),
  /* DHCP Scopes — address pool / range */
  dhcpScopes: (
    <>
      <rect x="3" y="9" width="18" height="6" rx="1.5" />
      <path d="M7 12h2M11 12h2M15 12h2" />
      <path d="M9 5h2M13 5h2M9 19h2M13 19h2" />
    </>
  ),
  /* DHCP Leases — list with address dot */
  leases: (
    <>
      <rect x="4" y="4" width="16" height="4" rx="1.2" />
      <rect x="4" y="10" width="16" height="4" rx="1.2" />
      <rect x="4" y="16" width="16" height="4" rx="1.2" />
      {dot(7, 6)}
      {dot(7, 12)}
      {dot(7, 18)}
    </>
  ),

  // — Services —
  /* Services — list rows in a frame */
  services: (
    <>
      <path d="M8 4H4v16h4M16 4h4v16h-4" />
      <path d="M10 8h4M10 12h4M10 16h4" />
    </>
  ),
  /* Sites — reverse proxy: host + backend */
  sites: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
    </>
  ),
  /* Certificates — document with seal */
  certificates: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M9 7h6M9 11h6" />
      <circle cx="12" cy="16" r="2.2" />
      <path d="m10.5 16 1.2 1.2 2.3-2.4" />
    </>
  ),
  /* PKI — CA tree hierarchy */
  pki: (
    <>
      <circle cx="12" cy="4" r="2" />
      <circle cx="5" cy="14" r="2" />
      <circle cx="19" cy="14" r="2" />
      <path d="M12 6v3M12 9 6 12M12 9l6 12M6 16v4M18 16v4" />
    </>
  ),
  /* AAA — auth + key */
  aaa: (
    <>
      <circle cx="9" cy="9" r="3" />
      <path d="M4 19a5 5 0 0 1 10 0" />
      <circle cx="17" cy="16" r="2" />
      <path d="M19 16h3M20 13v6" />
    </>
  ),
  /* Time / NTP — clock */
  time: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  /* Telemetry — broadcast / upload */
  telemetry: (
    <>
      <circle cx="12" cy="12" r="2" />
      <path d="M8 16a6 6 0 0 1 0-8M16 16a6 6 0 0 0 0-8" />
      <path d="M5 19a10 10 0 0 1 0-14M19 19a10 10 0 0 0 0-14" />
    </>
  ),
  /* UPnP — broadcast waves */
  upnp: (
    <>
      <circle cx="6" cy="18" r="2" />
      <path d="M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16" />
    </>
  ),
  /* Portal — captive sign-in card */
  portal: (
    <>
      <path d="M5 6h14a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2z" />
      <circle cx="12" cy="11" r="1.4" fill="currentColor" stroke="none" />
      <path d="M8 16a4 4 0 0 1 8 0" />
    </>
  ),
  /* Subscriptions — document + refresh */
  subscriptions: (
    <>
      <path d="M5 4h10l4 4v12H5z" />
      <path d="M15 4v4h4" />
      <path d="M9 14a3 3 0 0 1 6 0M15 14l2-2M15 14l-2-2" />
    </>
  ),
  /* Sites / HA — two systems synced */
  ha: (
    <>
      <rect x="3" y="8" width="8" height="8" rx="1.2" />
      <rect x="13" y="8" width="8" height="8" rx="1.2" />
      <path d="M11 12h2" />
      <path d="m9 11 2 1-2 1M15 11l-2 1 2 1" />
    </>
  ),
  /* SSH — terminal + key */
  ssh: (
    <>
      <rect x="3" y="5" width="18" height="14" rx="2" />
      <path d="M7 9l3 3-3 3M12 15h5" />
    </>
  ),
  /* Cron — clock + arrow */
  cron: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
      <path d="m18 18 3 3" />
      <path d="M19 19h2v2" />
    </>
  ),
  /* UPS — battery + bolt */
  ups: (
    <>
      <rect x="3" y="7" width="16" height="10" rx="1.5" />
      <path d="M19 10v4M22 10v4" />
      <path d="m9 9-2 6h2l-1 4" />
    </>
  ),
  /* Hardware — chip with pins */
  hardware: (
    <>
      <rect x="6" y="6" width="12" height="12" rx="1.5" />
      <rect x="9" y="9" width="6" height="6" rx="0.6" />
      <path d="M9 3v3m6-3v3M9 18v3m6-3v3M3 9h3m-3 6h3m12-6h3m-3 6h3" />
    </>
  ),
  /* License — key + badge */
  license: (
    <>
      <circle cx="8" cy="12" r="4" />
      <path d="M12 12h9M18 12v3M21 12v4" />
      <circle cx="8" cy="12" r="1" fill="currentColor" stroke="none" />
    </>
  ),

  // — Easy / overview-mode cards —
  /* Home */
  home: (
    <>
      <path d="M4 11 12 4l8 7" />
      <path d="M6 10v10h12V10" />
      <path d="M10 20v-6h4v6" />
    </>
  ),
  /* System — health/checkup on a device */
  system: (
    <>
      <rect x="4" y="4" width="16" height="14" rx="2" />
      <path d="M9 21h6M12 18v3" />
      <path d="m9 11 2 2 4-4" />
    </>
  ),
  /* Router — device with antennas */
  router: (
    <>
      <rect x="3" y="13" width="18" height="6" rx="1.4" />
      <path d="M7 16h.01M11 16h.01M15 16h.01" />
      <path d="M8 13V8M12 13V6M16 13V8" />
      <path d="M5 5l3 3M19 5l-3 3" />
    </>
  ),
  /* Care — heart/health icon used in Easy mode */
  care: (
    <>
      <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 7.8L12 13.3l-1-1a5.5 5.5 0 1 0-7.8-7.8 5.5 5.5 0 0 0 0 7.8L11 19.8l1-1 7.8-7.8a5.5 5.5 0 0 0 1-6.4z" />
    </>
  ),
  /* Publish — share-out arrow (Easy Publish page) */
  publish: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M9 12h6M12 9v6" />
    </>
  ),

  // — UI affordances —
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m16 16 5 5" />
    </>
  ),
  lock: (
    <>
      <rect x="5" y="10" width="14" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3" />
    </>
  ),
  logout: (
    <>
      <path d="M10 4H5v16h5M9 12h12m-4-4 4 4-4 4" />
    </>
  ),
  refresh: (
    <>
      <path d="M20 10a8 8 0 0 0-14-5L3 8m0-5v5h5M4 14a8 8 0 0 0 14 5l3-3m0 5v-5h-5" />
    </>
  ),
  arrow: <path d="M5 12h14m-5-5 5 5-5 5" />,
  chevron: <path d="m9 5 7 7-7 7" />,
  help: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 1.5-2.5 2-2.5 4" />
      {dot(12, 17, 0.6)}
    </>
  ),
  menu: (
    <>
      <path d="M4 7h16M4 12h16M4 17h16" />
    </>
  ),
  close: (
    <>
      <path d="m6 6 12 12M18 6 6 18" />
    </>
  ),
  check: (
    <>
      <path d="m5 12 5 5 9-11" />
    </>
  ),
  warn: (
    <>
      <path d="m10.3 4-8 14a2 2 0 0 0 1.7 3h16a2 2 0 0 0 1.7-3l-8-14a2 2 0 0 0-3.4 0Z" />
      <path d="M12 10v4m0 3h.01" />
    </>
  ),
  plus: (
    <>
      <path d="M12 5v14M5 12h14" />
    </>
  ),
  minus: (
    <>
      <path d="M5 12h14" />
    </>
  ),
  edit: (
    <>
      <path d="M4 20h4l11-11-4-4L4 16z" />
      <path d="m14 5 5 5" />
    </>
  ),
  trash: (
    <>
      <path d="M5 7h14M9 7V4h6v3M7 7l1 13a2 2 0 0 0 2 2h4a2 2 0 0 0 2-2l1-13" />
    </>
  ),
  more: (
    <>
      {dot(6, 12, 1.4)}
      {dot(12, 12, 1.4)}
      {dot(18, 12, 1.4)}
    </>
  ),
  external: (
    <>
      <path d="M14 5h5v5" />
      <path d="m19 5-9 9" />
      <path d="M19 13v6H5V5h6" />
    </>
  ),
  /* Sun — theme toggle (light → dark) */
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4l1.4-1.4M17 7l1.4-1.4" />
    </>
  ),
  /* Moon — theme toggle (dark → light) */
  moon: (
    <>
      <path d="M20 14a8 8 0 1 1-9-10 7 7 0 0 0 9 10z" />
    </>
  ),
  /* Globe — locale */
  globe: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
    </>
  ),
  info: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 8h.01M12 12v5" />
    </>
  ),

  // — Legacy fallback icons —
  // These names are kept in IconName for backward compatibility with any
  // caller that still references them. New code should use the canonical
  // name listed at the top of each section.
  network: (
    <>
      <rect x="8" y="3" width="8" height="6" rx="1.5" />
      <path d="M12 9v5M5 17v-3h14v3" />
      <rect x="2" y="17" width="6" height="4" rx="1" />
      <rect x="16" y="17" width="6" height="4" rx="1" />
    </>
  ),
  lan: (
    <>
      <rect x="3" y="9" width="18" height="6" rx="1.5" />
      <path d="M7 9V6M17 9V6M7 15v3M17 15v3M12 9V6" />
    </>
  ),
  activity: (
    <>
      <path d="M3 12h4l3-8 4 16 3-8h4" />
    </>
  ),
  policy: (
    <>
      <path d="M4 6h16M4 12h16M4 18h10" />
      <circle cx="18" cy="18" r="2" />
    </>
  ),
  tree: (
    <>
      <path d="M12 4v6M12 14v6M6 10h12M6 20h12" />
      {dot(6, 10, 1.5)}
      {dot(18, 10, 1.5)}
      {dot(6, 20, 1.5)}
      {dot(18, 20, 1.5)}
    </>
  ),
  feed: (
    <>
      <circle cx="6" cy="18" r="2" />
      <path d="M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16" />
    </>
  ),
  site: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />
    </>
  ),
  capture: (
    <>
      <rect x="3" y="6" width="13" height="12" rx="2" />
      <path d="m22 8-6 4 6 4z" />
    </>
  ),
  vpn: (
    <>
      <path d="m12 3 7 3v5c0 5-7 9-7 9s-7-4-7-9V6z" />
      <circle cx="12" cy="10" r="2" />
      <path d="M12 12v3" />
    </>
  ),
  tunnel: (
    <>
      <path d="M3 14a9 9 0 0 1 18 0" />
      <path d="M7 14a5 5 0 0 1 10 0" />
      <path d="M11 14a1 1 0 0 1 2 0" />
      {dot(12, 14, 0.8)}
    </>
  ),
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21a8 8 0 0 1 16 0" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  pulse: (
    <>
      <path d="M3 12h3l3-8 4 16 3-8h5" />
    </>
  ),
  timeline: (
    <>
      <path d="M3 6h12M3 12h18M3 18h8" />
    </>
  ),
  template: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M3 9h18M9 4v16" />
    </>
  ),
  candidate: (
    <>
      <path d="M12 4v12" />
      <path d="m7 9 5-5 5 5" />
      <path d="M4 20h16" />
    </>
  ),
  cpu: (
    <>
      <rect x="6" y="6" width="12" height="12" rx="2" />
      <rect x="9" y="9" width="6" height="6" rx="1" />
      <path d="M9 3v3m6-3v3M9 18v3m6-3v3M3 9h3m-3 6h3m12-6h3m-3 6h3" />
    </>
  ),
  power: (
    <>
      <path d="M12 4v8" />
      <path d="M6 7a8 8 0 1 0 12 0" />
    </>
  ),
  alert: (
    <>
      <path d="m10.3 4-8 14a2 2 0 0 0 1.7 3h16a2 2 0 0 0 1.7-3l-8-14a2 2 0 0 0-3.4 0Z" />
      <path d="M12 9v4m0 4h.01" />
    </>
  ),
  log: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M8 8h8M8 12h8M8 16h5" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="12" r="4" />
      <path d="M12 12h9M18 12v3M21 12v4" />
    </>
  ),
};

/* Legacy aliases — keep the old registry names mapping to the new icons
 * so any caller that still references the previous name resolves correctly.
 * New callers should use the canonical name. */
const sectionAliases: Partial<Record<IconName, IconName>> = {
  monitor: "monitor",
  lan: "vlan",
  candidate: "updates",
  policy: "policyRouting",
  activity: "oam",
  tree: "dhcpScopes",
  capture: "mirroring",
  power: "ups",
  cpu: "hardware",
};

function resolve(name: string): IconName {
  const base = (name as IconName) in paths ? (name as IconName) : "network";
  if (!((base as IconName) in paths)) {
    const aliased = sectionAliases[base];
    if (aliased) return aliased as IconName;
  }
  return (sectionAliases[base] ?? base) as IconName;
}

export function Icon({
  name,
  className = "",
  size,
}: {
  name: string;
  className?: string;
  size?: number;
}) {
  const resolved = resolve(name);
  return (
    <svg
      className={`icon ${className}`}
      aria-hidden="true"
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {paths[resolved]}
    </svg>
  );
}

// Re-export the themed mark from src/brand/.
export { DuckMark, DuckWordmark } from "./brand/DuckMark";

/* Dev-only: serialize each icon's SVG markup to a JSON object keyed
 * by IconName. Consumed by `vite.config.ts`'s dev middleware during
 * iconography visual capture. The wrapper SVG includes the standard
 * 24×24 viewBox so consumers can size it freely via CSS width/height. */
import { renderToStaticMarkup } from "react-dom/server";

export const __iconPathsForCapture: Record<IconName, string> =
  Object.fromEntries(
    (Object.entries(paths) as [IconName, ReactNode][]).map(([k, v]) => [
      k,
      renderToStaticMarkup(
        <svg
          viewBox="0 0 24 24"
          width="24"
          height="24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {v}
        </svg>,
      ),
    ]),
  ) as Record<IconName, string>;
