/**
 * DuckOS V1 — V4 strings.
 *
 * A small, type-checked, locale-resolving string module for V4-only
 * UI. Lives outside the main typed bundle so it can grow fast and so
 * the existing Messages contract stays stable.
 *
 * Every key in this file MUST exist in every locale below. Missing
 * keys throw at module-evaluation time during development.
 */
import type { Locale } from "./messages";

// Use `any[]` for the function value type so individual callers (which
// declare more specific signatures like `(n: number) => string`) remain
// assignable without contravariance errors.
type V4Bundle = Readonly<Record<string, string | ((...args: any[]) => string)>>;
type V4Messages = Readonly<Record<Locale, V4Bundle>>;

interface V4 {
  /* Page meta */
  "pageMeta.title": string;
  "pageMeta.designOnly": string;
  "pageMeta.readOnly": string;
  "pageMeta.configurable": string;
  "pageMeta.irreversible": string;

  /* Topbar */
  "topbar.searchOpen": string;
  "topbar.openSearchHint": string;
  "topbar.openInspector": string;
  "topbar.themeToggle": string;
  "topbar.localeSelect": string;
  "topbar.endSession": string;
  "topbar.modeEasy": string;
  "topbar.modeFull": string;

  /* Shell */
  "shell.skipToMain": string;
  "shell.primaryNav": string;

  /* Rail groups */
  "rail.network": string;
  "rail.routing": string;
  "rail.security": string;
  "rail.services": string;
  "rail.system": string;
  "rail.logs": string;

  /* Easy rail */
  "easy.home": string;
  "easy.internet": string;
  "easy.wifi": string;
  "easy.devices": string;
  "easy.vpn": string;
  "easy.system": string;

  /* Easy Home */
  "easyHome.title": string;
  "easyHome.subtitle": string;
  "easyHome.headlineLabel": string;
  "easyHome.statusHealthy": string;
  "easyHome.statusAttention": string;
  "easyHome.statusIncomplete": string;
  "easyHome.statusUnavailable": string;
  "easyHome.statusVerified": string;
  "easyHome.statusUnverified": string;
  "easyHome.statusInternetObserved": string;
  "easyHome.statusInternetVerified": string;
  "easyHome.statusInternetIncomplete": string;
  "easyHome.statusInternetUnavailable": string;
  "easyHome.internetCard": string;
  "easyHome.internetSub": string;
  "easyHome.internetConfigure": string;
  "easyHome.wifiCard": string;
  "easyHome.wifiSub": string;
  "easyHome.wifiConfigure": string;
  "easyHome.wifiDetails": string;
  "easyHome.devicesCard": string;
  "easyHome.devicesSub": string;
  "easyHome.devicesOpen": string;
  "easyHome.vpnCard": string;
  "easyHome.vpnSub": string;
  "easyHome.vpnConfigure": string;
  "easyHome.systemCard": string;
  "easyHome.systemSub": string;
  "easyHome.systemOpen": string;
  "easyHome.aboutTitle": string;
  "easyHome.aboutHostname": string;
  "easyHome.aboutUptime": string;
  "easyHome.aboutEnvironment": string;
  "easyHome.aboutOs": string;
  "easyHome.aboutKernel": string;
  "easyHome.aboutArchitecture": string;
  "easyHome.aboutObserved": string;
  "easyHome.details": string;
  "easyHome.unknownValue": string;

  /* Easy Care */
  "easyCare.title": string;
  "easyCare.subtitle": string;
  "easyCare.apiReachable": string;
  "easyCare.apiUnreachable": string;
  "easyCare.findingsCount": string;
  "easyCare.doctorEmptyTitle": string;
  "easyCare.doctorEmptyBody": string;
  "easyCare.doctorEmptyAction": string;
  "easyCare.doctorUnavailableTitle": string;
  "easyCare.doctorUnavailableBody": string;
  "easyCare.doctorUnavailableAction": string;
  "easyCare.findingOpenAction": string;
  "easyCare.doctorLoadingTitle": string;
  "easyCare.doctorLoadingBody": string;

  /* System Overview */
  "sys.title": string;
  "sys.subtitle": string;
  "sys.headline": string;
  "sys.headlineDescription": string;
  "sys.sectionHeadline": string;
  "sys.sectionHeadlineSub": string;
  "sys.cards": string;
  "sys.cardCpu": string;
  "sys.cardMemory": string;
  "sys.cardStorage": string;
  "sys.cardInterfaces": string;
  "sys.cardRoutes": string;
  "sys.cardUptime": string;
  "sys.cardDns": string;
  "sys.cardClock": string;
  "sys.identity": string;
  "sys.identitySub": string;
  "sys.storage": string;
  "sys.storageSub": string;
  "sys.cores": string;
  "sys.percentUsed": string;
  "sys.coresFormat": (n: number) => string;
  "sys.percentFormat": (pct: number) => string;
  "sys.uptimeDaysHours": (d: number, h: number) => string;
  "sys.uptimeHours": (h: number) => string;
  "sys.uptimeMinutes": (m: number) => string;
  "sys.recentFindings": string;
  "sys.noCurrentFindings": string;
  "sys.dataStale": string;
  "sys.recentIncidents": string;
  "sys.noIncidents": string;
  "sys.loading": string;
  "sys.error": string;
  "sys.contractMissing": string;

  /* Page-specific copy */
  "pageWan.findUplink": string;
  "pageWan.defaultGateway": string;
  "pageWan.via": string;
  "pageWan.noMatch": string;
  "pageWan.noUplinks": string;
  "pageWan.failoverOrder": string;
  "pageWan.colInterface": string;
  "pageWan.colUplink": string;
  "pageWan.colState": string;
  "pageWan.colRole": string;
  "pageWan.colHealth": string;
  "pageWan.colIPv4": string;
  "pageWan.colIPv6": string;
  "pageWan.colMTU": string;

  /* ActionBar */
  "actionbar.idleAllGood": string;
  "actionbar.idleQuiet": string;
  "actionbar.idleStale": string;
  "actionbar.pending": (n: number) => string;
  "actionbar.apply": string;
  "actionbar.discard": string;
  "actionbar.applying": string;
  "actionbar.confirmRequired": (s: number) => string;
  "actionbar.confirm": string;
  "actionbar.rollback": string;
  "actionbar.failed": string;
  "actionbar.failedRecovery": string;

  /* Health */
  "health.healthy": string;
  "health.needsAttention": string;
  "health.statusIncomplete": string;
  "health.observationUntrusted": string;
  "health.unavailable": string;
  "health.observationFresh": string;
  "health.observationStale": string;
  "health.observationNone": string;

  /* Search overlay */
  "search.headingPages": string;
  "search.headingObjects": string;
  "search.headingSections": string;
  "search.headingActions": string;
  "search.placeholder": string;
  "search.openHint": string;
  "search.empty": string;
  "search.emptyHint": string;
  "search.tooShort": string;
  "search.tooShortHint": string;
  "search.resultsHint": (n: number) => string;

  /* Empty states per domain */
  "empty.wireguardPeers.title": string;
  "empty.wireguardPeers.body": string;
  "empty.wireguardPeers.action": string;
  "empty.firewallRules.title": string;
  "empty.firewallRules.body": string;
  "empty.firewallRules.action": string;
  "empty.dhcpLeases.title": string;
  "empty.dhcpLeases.body": string;
  "empty.dhcpLeases.action": string;
  "empty.xray.title": string;
  "empty.xray.body": string;
  "empty.xray.action": string;
  "empty.sites.title": string;
  "empty.sites.body": string;
  "empty.sites.action": string;
  "empty.vpn.title": string;
  "empty.vpn.body": string;
  "empty.interfaces.title": string;
  "empty.interfaces.body": string;
  "empty.routes.title": string;
  "empty.routes.body": string;
  "empty.dns.title": string;
  "empty.dns.body": string;
  "empty.services.title": string;
  "empty.services.body": string;
  "empty.generic.title": string;
  "empty.generic.body": string;

  /* Inspector */
  "inspector.tabs.what": string;
  "inspector.tabs.why": string;
  "inspector.tabs.risk": string;
  "inspector.tabs.example": string;
  "inspector.openCTA": string;
  "inspector.technicalDetails": string;
  "inspector.noObjectSelected": string;
  "inspector.kind": string;
  "inspector.snapshot": string;
  "inspector.objectId": string;

  /* Per-domain Inspector copy. Kind matches SupportedObjectKind. */
  "inspector.domain.network.interface.what": string;
  "inspector.domain.network.interface.why": string;
  "inspector.domain.network.interface.risk": string;
  "inspector.domain.network.interface.example": string;
  "inspector.domain.network.address.what": string;
  "inspector.domain.network.address.why": string;
  "inspector.domain.network.address.risk": string;
  "inspector.domain.network.address.example": string;
  "inspector.domain.network.firewall.rule.what": string;
  "inspector.domain.network.firewall.rule.why": string;
  "inspector.domain.network.firewall.rule.risk": string;
  "inspector.domain.network.firewall.rule.example": string;
  "inspector.domain.network.firewall.set.what": string;
  "inspector.domain.network.firewall.set.why": string;
  "inspector.domain.network.firewall.set.risk": string;
  "inspector.domain.network.firewall.set.example": string;
  "inspector.domain.network.lease.what": string;
  "inspector.domain.network.lease.why": string;
  "inspector.domain.network.lease.risk": string;
  "inspector.domain.network.lease.example": string;
  "inspector.domain.wireguard.server.what": string;
  "inspector.domain.wireguard.server.why": string;
  "inspector.domain.wireguard.server.risk": string;
  "inspector.domain.wireguard.server.example": string;
  "inspector.domain.wireguard.peer.what": string;
  "inspector.domain.wireguard.peer.why": string;
  "inspector.domain.wireguard.peer.risk": string;
  "inspector.domain.wireguard.peer.example": string;
  "inspector.domain.xray.server.what": string;
  "inspector.domain.xray.server.why": string;
  "inspector.domain.xray.server.risk": string;
  "inspector.domain.xray.server.example": string;
  "inspector.domain.xray.user.what": string;
  "inspector.domain.xray.user.why": string;
  "inspector.domain.xray.user.risk": string;
  "inspector.domain.xray.user.example": string;
  "inspector.domain.dns.zone.what": string;
  "inspector.domain.dns.zone.why": string;
  "inspector.domain.dns.zone.risk": string;
  "inspector.domain.dns.zone.example": string;
  "inspector.domain.dhcp.subnet.what": string;
  "inspector.domain.dhcp.subnet.why": string;
  "inspector.domain.dhcp.subnet.risk": string;
  "inspector.domain.dhcp.subnet.example": string;
  "inspector.domain.route.map.what": string;
  "inspector.domain.route.map.why": string;
  "inspector.domain.route.map.risk": string;
  "inspector.domain.route.map.example": string;
  "inspector.domain.bgp.peer.what": string;
  "inspector.domain.bgp.peer.why": string;
  "inspector.domain.bgp.peer.risk": string;
  "inspector.domain.bgp.peer.example": string;
  "inspector.domain.ospf.area.what": string;
  "inspector.domain.ospf.area.why": string;
  "inspector.domain.ospf.area.risk": string;
  "inspector.domain.ospf.area.example": string;
  "inspector.domain.qos.class.what": string;
  "inspector.domain.qos.class.why": string;
  "inspector.domain.qos.class.risk": string;
  "inspector.domain.qos.class.example": string;
  "inspector.domain.page.network": string;
  "inspector.domain.page.wan": string;
  "inspector.domain.page.lan": string;
  "inspector.domain.page.routes": string;
  "inspector.domain.page.firewall": string;
  "inspector.domain.page.dns": string;
  "inspector.domain.page.dhcp": string;
  "inspector.domain.page.services": string;

  /* Legacy chrome strings — used by the configurator internals (filter,
     availability, actions, columns, technical details). Page chrome uses
     the v4 namespace above; this namespace exists for the lower-level
     rows, buttons and toggles still rendered by Table/FilterBar. */
  "chrome.filter": string;
  "chrome.typeToFilter": string;
  "chrome.allAvailability": string;
  "chrome.availableOnly": string;
  "chrome.nonAvailable": string;
  "chrome.availability": string;
  "chrome.create": string;
  "chrome.edit": string;
  "chrome.delete": string;
  "chrome.actions": string;
  "chrome.ofTotal": (n: number, total: number) => string;
  "chrome.technicalDetails": string;
  "chrome.collectionSnapshot": string;
  "chrome.emptyKind": string;
  "chrome.total": string;
  "chrome.active": string;
  "chrome.issues": string;

  /* V4 page chrome */
  "v4.allTechnicalColumns": string;
  "v4.hideTechnicalColumns": string;
  "v4.emptyFor": string;
  "v4.viewFull": string;
  "v4.refresh": string;
  "v4.copyAction": string;
  "v4.copied": string;
  "v4.since": string;
  "v4.units.bytes": (n: number) => string;
  "v4.units.percent": (n: number) => string;
}

const en: V4Bundle = {
  /* Page meta */
  "pageMeta.title": "Page",
  "pageMeta.designOnly": "Design-only surface",
  "pageMeta.readOnly": "Read-only",
  "pageMeta.configurable": "Configurable",
  "pageMeta.irreversible": "Irreversible",

  /* Topbar */
  "topbar.searchOpen": "Search",
  "topbar.openSearchHint": "Search pages and help",
  "topbar.openInspector": "Page help",
  "topbar.themeToggle": "Toggle theme",
  "topbar.localeSelect": "Language",
  "topbar.endSession": "End session",
  "topbar.modeEasy": "Easy",
  "topbar.modeFull": "Full",

  /* Shell */
  "shell.skipToMain": "Skip to main content",
  "shell.primaryNav": "Primary navigation",

  /* Rail groups */
  "rail.network": "Network",
  "rail.routing": "Routing",
  "rail.security": "Security",
  "rail.services": "Services",
  "rail.system": "System",
  "rail.logs": "Logs & Diagnostics",

  /* Easy rail */
  "easy.home": "Home",
  "easy.internet": "Internet",
  "easy.wifi": "Wi-Fi",
  "easy.devices": "Devices",
  "easy.vpn": "VPN",
  "easy.system": "System",

  /* Easy Home */
  "easyHome.title": "Is everything working?",
  "easyHome.subtitle":
    "A one-screen view of the network. If something is wrong, you'll see it here first.",
  "easyHome.headlineLabel": "Overall",
  "easyHome.statusHealthy": "Everything is working",
  "easyHome.statusAttention": "Some things need attention",
  "easyHome.statusIncomplete": "Status check incomplete",
  "easyHome.statusUnavailable": "Status unavailable",
  "easyHome.statusVerified": "verified",
  "easyHome.statusUnverified": "verification pending",
  "easyHome.statusInternetObserved": "Internet reachable",
  "easyHome.statusInternetVerified": "Internet reachable",
  "easyHome.statusInternetIncomplete": "Connectivity not verified",
  "easyHome.statusInternetUnavailable": "Connectivity check unavailable",
  "easyHome.internetCard": "Internet",
  "easyHome.internetSub": "How this device reaches the rest of the world.",
  "easyHome.internetConfigure": "Configure Internet",
  "easyHome.wifiCard": "Wi-Fi",
  "easyHome.wifiSub": "Wireless networks this device can reach.",
  "easyHome.wifiConfigure": "Configure Wi-Fi",
  "easyHome.wifiDetails": "Details",
  "easyHome.devicesCard": "Devices",
  "easyHome.devicesSub": "Devices currently visible on your network.",
  "easyHome.devicesOpen": "View devices",
  "easyHome.vpnCard": "VPN",
  "easyHome.vpnSub": "WireGuard and Xray services configured on this device.",
  "easyHome.vpnConfigure": "Open VPN",
  "easyHome.systemCard": "System",
  "easyHome.systemSub": "How this device is doing overall.",
  "easyHome.systemOpen": "Open Doctor",
  "easyHome.aboutTitle": "About this device",
  "easyHome.aboutHostname": "Hostname",
  "easyHome.aboutUptime": "Uptime",
  "easyHome.aboutEnvironment": "Environment",
  "easyHome.aboutOs": "Operating system",
  "easyHome.aboutKernel": "Kernel",
  "easyHome.aboutArchitecture": "Architecture",
  "easyHome.aboutObserved": "Observed",
  "easyHome.details": "Details",
  "easyHome.unknownValue": "—",

  /* Easy Care */
  "easyCare.title": "Care & recovery",
  "easyCare.subtitle":
    "What Doctor sees, and the buttons you can press to recover when something is wrong.",
  "easyCare.apiReachable": "API reachable",
  "easyCare.apiUnreachable": "API unreachable",
  "easyCare.findingsCount": "findings",
  "easyCare.doctorEmptyTitle": "No current Doctor findings",
  "easyCare.doctorEmptyBody":
    "Doctor returned an empty report. Verified system health requires additional evidence we don't have yet.",
  "easyCare.doctorEmptyAction": "Open Doctor in Full mode",
  "easyCare.doctorUnavailableTitle": "Doctor status unavailable",
  "easyCare.doctorUnavailableBody":
    "We couldn't reach Doctor. This is different from \"no findings\" — we can't confirm anything until Doctor responds.",
  "easyCare.doctorUnavailableAction": "Retry",
  "easyCare.findingOpenAction": "Open Doctor",
  "easyCare.doctorLoadingTitle": "Checking Doctor…",
  "easyCare.doctorLoadingBody":
    "Doctor hasn't responded yet. We're waiting for its first report — this is different from \"no findings\".",

  /* System Overview */
  "sys.title": "System Overview",
  "sys.subtitle":
    "What is failing now, what still works, and is anything waiting to apply.",
  "sys.headline": "Overall",
  "sys.headlineDescription":
    "One top-level answer for the device, drawn from observation and the Doctor.",
  "sys.sectionHeadline": "Resources at a glance",
  "sys.sectionHeadlineSub":
    "Live signals from the system, refreshed on demand.",
  "sys.cards": "Resource cards",
  "sys.cardCpu": "CPU",
  "sys.cardMemory": "Memory",
  "sys.cardStorage": "Storage",
  "sys.cardInterfaces": "Interfaces",
  "sys.cardRoutes": "Routes",
  "sys.cardUptime": "Uptime",
  "sys.cardDns": "DNS",
  "sys.cardClock": "Clock",
  "sys.identity": "Identity",
  "sys.identitySub": "How this device presents itself.",
  "sys.storage": "Storage",
  "sys.storageSub": "Filesystems mounted on this device.",
  "sys.cores": "logical cores",
  "sys.percentUsed": "in use",
  "sys.coresFormat": (n: number) => `${n} ${n === 1 ? "core" : "cores"}`,
  "sys.percentFormat": (pct: number) => `${pct}% in use`,
  "sys.uptimeDaysHours": (d: number, h: number) =>
    `${d}d ${h}h since the last boot`,
  "sys.uptimeHours": (h: number) => `${h}h since the last boot`,
  "sys.uptimeMinutes": (m: number) => `${m}m since the last boot`,
  "sys.recentFindings": "Recent findings",
  "sys.noCurrentFindings": "No current finding details are available.",
  "sys.dataStale": "Status check is incomplete — observation may be stale.",
  "sys.recentIncidents": "Recent incidents",
  "sys.noIncidents": "No recent incidents.",
  "sys.loading": "Loading…",
  "sys.error": "Something went wrong.",
  "sys.contractMissing": "API contract not implemented.",

  "pageWan.findUplink": "Find an uplink",
  "pageWan.defaultGateway": "Default gateway:",
  "pageWan.via": "via",
  "pageWan.noMatch": "No uplinks match this view.",
  "pageWan.noUplinks": "No uplinks configured.",
  "pageWan.failoverOrder": "Failover order",
  "pageWan.colInterface": "Interface",
  "pageWan.colUplink": "Uplink",
  "pageWan.colState": "State",
  "pageWan.colRole": "Role",
  "pageWan.colHealth": "Health",
  "pageWan.colIPv4": "IPv4",
  "pageWan.colIPv6": "IPv6",
  "pageWan.colMTU": "MTU",

  /* ActionBar */
  "actionbar.idleAllGood": "No pending changes",
  "actionbar.idleQuiet": "Quiet · nothing to apply",
  "actionbar.idleStale": "Status check incomplete · observation may be stale",
  "actionbar.pending": (n: number) =>
    `${n} ${n === 1 ? "change" : "changes"} waiting to apply`,
  "actionbar.apply": "Validate & Apply",
  "actionbar.discard": "Discard all",
  "actionbar.applying": "Applying…",
  "actionbar.confirmRequired": (s: number) =>
    `Confirmation required · auto-rollback in ${s}s`,
  "actionbar.confirm": "Confirm",
  "actionbar.rollback": "Rollback",
  "actionbar.failed": "Apply failed",
  "actionbar.failedRecovery": "Open recovery",

  /* Health */
  "health.healthy": "Healthy",
  "health.needsAttention": "Needs attention",
  "health.statusIncomplete": "Status incomplete",
  "health.observationUntrusted": "Status check incomplete",
  "health.unavailable": "Unavailable",
  "health.observationFresh": "Observation fresh",
  "health.observationStale": "Observation stale",
  "health.observationNone": "No observation",

  /* Search overlay */
  "search.headingPages": "Pages",
  "search.headingObjects": "Devices & addresses",
  "search.headingSections": "Sections",
  "search.headingActions": "Actions",
  "search.placeholder": "Search pages and help",
  "search.openHint": "Press Esc to close",
  "search.empty": "No matches for this search",
  "search.emptyHint": "Try a different name, role, or section.",
  "search.tooShort": "Keep typing",
  "search.tooShortHint": "Type at least two characters.",
  "search.resultsHint": (n: number) =>
    `${n} ${n === 1 ? "result" : "results"} · ↵ to open · ↑↓ to move`,

  /* Empty states */
  "empty.wireguardPeers.title": "No peers yet",
  "empty.wireguardPeers.body":
    "WireGuard peers let other devices join this network securely. Add a peer to issue a key and a configuration.",
  "empty.wireguardPeers.action": "Add a WireGuard peer",
  "empty.firewallRules.title": "No custom firewall rules",
  "empty.firewallRules.body":
    "Traffic follows the default policy. Add a rule to allow, reject, or block specific flows.",
  "empty.firewallRules.action": "Create a firewall rule",
  "empty.dhcpLeases.title": "No devices have requested an address yet",
  "empty.dhcpLeases.body":
    "No clients have received an address in the current observation window.",
  "empty.dhcpLeases.action": "View DHCP scopes",
  "empty.xray.title": "No Xray profiles",
  "empty.xray.body":
    "Create a profile to issue client access. Each profile describes one user and its routing rules.",
  "empty.xray.action": "Create an Xray profile",
  "empty.sites.title": "No replication sites",
  "empty.sites.body":
    "Sites connect this device to other DuckOS installations so configuration and state can travel together.",
  "empty.sites.action": "Add a site",
  "empty.vpn.title": "No VPN is configured",
  "empty.vpn.body":
    "Set up WireGuard or Xray to expose this device to a remote network or to remote clients.",
  "empty.interfaces.title": "No interfaces",
  "empty.interfaces.body":
    "No interfaces are currently visible. Check that the kernel reports at least one device.",
  "empty.routes.title": "No routes",
  "empty.routes.body":
    "Routes will appear here as soon as the kernel learns about them.",
  "empty.dns.title": "No DNS configuration",
  "empty.dns.body":
    "Add a resolver to tell this device where to send its DNS queries.",
  "empty.services.title": "No services",
  "empty.services.body":
    "No services are currently listed. New services will appear after the first observation cycle.",
  "empty.generic.title": "Nothing here yet",
  "empty.generic.body":
    "This view is empty right now. Once the related feature is configured, items will appear here.",

  /* Inspector */
  "inspector.tabs.what": "What is this?",
  "inspector.tabs.why": "Why use it?",
  "inspector.tabs.risk": "What changes if I edit it?",
  "inspector.tabs.example": "Example",
  "inspector.openCTA": "Open page help",
  "inspector.technicalDetails": "Technical details",
  "inspector.noObjectSelected": "Select a row to see its details.",
  "inspector.kind": "Kind",
  "inspector.snapshot": "Snapshot",
  "inspector.objectId": "Object id",

  /* Per-domain Inspector copy */
  "inspector.domain.network.interface.what":
    "Interfaces are the physical and virtual network connections on this router — LAN, WAN, bridges, VLANs, tunnels.",
  "inspector.domain.network.interface.why":
    "Browse and verify the active connections. Use this to confirm a port is administratively up and to inspect the IPv4/IPv6 addresses assigned to it.",
  "inspector.domain.network.interface.risk":
    "Reading is always safe. Editing here can change which interfaces exist and may disconnect active sessions.",
  "inspector.domain.network.interface.example":
    "eth0 — the upstream WAN port — shows as administratively up with one IPv4 address and one IPv6 address assigned by the provider.",

  "inspector.domain.network.address.what":
    "An IP address is bound to a specific interface. Together with the prefix length it defines the subnet the interface participates in.",
  "inspector.domain.network.address.why":
    "Look up which interface owns a given IP, check the prefix length, and verify that an address is currently reachable.",
  "inspector.domain.network.address.risk":
    "Reading is safe. Editing here may move an address between interfaces and disrupt active sessions.",
  "inspector.domain.network.address.example":
    "192.0.2.1/24 on eth0 belongs to the LAN bridge, while 203.0.113.4/32 on eth1 is the provider-assigned WAN address.",

  "inspector.domain.network.firewall.rule.what":
    "Firewall rules determine which traffic is allowed, rejected, or dropped between networks. Rules are evaluated in order until one matches.",
  "inspector.domain.network.firewall.rule.why":
    "Use this page to add custom exceptions to the default policy — for example, to expose a service on the LAN side or to block a specific destination.",
  "inspector.domain.network.firewall.rule.risk":
    "Incorrect rules can lock you out of the router or expose services to the internet. Edits follow the Safe Apply workflow.",
  "inspector.domain.network.firewall.rule.example":
    "Allow TCP 443 from the LAN to a specific internal host so a home dashboard is reachable from your phone on Wi-Fi.",

  "inspector.domain.network.firewall.set.what":
    "Firewall sets are reusable groups of addresses, ports, or interfaces that firewall rules can reference by name.",
  "inspector.domain.network.firewall.set.why":
    "Define sets once and reference them across many rules so the policy stays readable and a single edit propagates everywhere.",
  "inspector.domain.network.firewall.set.risk":
    "Renaming or deleting a set can break any rule that references it.",
  "inspector.domain.network.firewall.set.example":
    "A set called 'guest-clients' references every address on the guest bridge so one rule can grant guest devices internet-only access.",

  "inspector.domain.network.lease.what":
    "A DHCP lease is the temporary binding between a MAC address and an IP address that the DHCP server has handed out.",
  "inspector.domain.network.lease.why":
    "See which devices received an address, when the lease expires, and which scope it came from.",
  "inspector.domain.network.lease.risk":
    "Reading is safe. Manually removing a lease forces that client to renew it on next contact.",
  "inspector.domain.network.lease.example":
    "Phone-on-wifi — 192.168.1.42 — lease valid for 12 hours from the home scope.",

  "inspector.domain.wireguard.server.what":
    "A WireGuard server is the public side of the VPN — it listens on a UDP port and accepts connections from configured peers.",
  "inspector.domain.wireguard.server.why":
    "Configure a server when you want to expose the LAN, route all client traffic through this router, or link two sites together.",
  "inspector.domain.wireguard.server.risk":
    "Incorrect listening port or key may prevent peers from connecting. Key changes invalidate every peer's configuration.",
  "inspector.domain.wireguard.server.example":
    "Server 'home-vpn' listens on UDP 51820 with a 10.66.0.0/24 tunnel and routes all peer traffic through the WAN.",

  "inspector.domain.wireguard.peer.what":
    "A WireGuard peer is one client or remote site that connects to a WireGuard server on this router.",
  "inspector.domain.wireguard.peer.why":
    "Add a peer for every device or site you want to reach the VPN. Each peer gets its own key pair and tunnel address.",
  "inspector.domain.wireguard.peer.risk":
    "Deleting a peer immediately disconnects that device from the VPN. The peer's public key cannot be recovered after deletion.",
  "inspector.domain.wireguard.peer.example":
    "Peer 'alice-laptop' connects from a phone on roaming networks and gets a fixed 10.66.0.2 tunnel address.",

  "inspector.domain.xray.server.what":
    "An Xray server is a proxy endpoint that runs on this router and forwards traffic for configured users.",
  "inspector.domain.xray.server.why":
    "Set up an Xray server when you want users to reach the internet through this router with a configured protocol and port.",
  "inspector.domain.xray.server.risk":
    "Misconfigured transport or credentials can prevent all users from connecting. The protocol choice affects compatibility with clients.",
  "inspector.domain.xray.server.example":
    "Server 'edge-1' accepts VLESS+Reality connections on TCP 443 and forwards user traffic to the WAN.",

  "inspector.domain.xray.user.what":
    "An Xray user is a credentialed client of an Xray server. Each user gets an identity (UUID), an encryption setting, and traffic limits.",
  "inspector.domain.xray.user.why":
    "Issue one user per person or device. Users receive a connection string or link to paste into their client.",
  "inspector.domain.xray.user.risk":
    "Deleting a user immediately revokes their access. Revoked users cannot be recovered unless their credential was exported first.",
  "inspector.domain.xray.user.example":
    "User 'family-phone' uses UUID-based auth with flow 'xtls-rprx-vision' and 100 GiB per month.",

  "inspector.domain.dns.zone.what":
    "A DNS zone is a slice of the namespace this router is authoritative for — for example, the local home zone.",
  "inspector.domain.dns.zone.why":
    "Inspect which zones exist, which records they contain, and which servers serve them.",
  "inspector.domain.dns.zone.risk":
    "Reading is safe. Editing zones may change how names on the LAN resolve.",
  "inspector.domain.dns.zone.example":
    "Zone 'home.lan' serves records for every device on the LAN and forwards unknown names to the upstream resolvers.",

  "inspector.domain.dhcp.subnet.what":
    "A DHCP subnet is the address pool and parameters (gateway, DNS, lease time) handed out to clients on a given network segment.",
  "inspector.domain.dhcp.subnet.why":
    "Configure one DHCP subnet per LAN segment. Each defines the range of addresses the server is allowed to lease.",
  "inspector.domain.dhcp.subnet.risk":
    "Wrong gateway or DNS values can isolate clients from the internet or from local name resolution.",
  "inspector.domain.dhcp.subnet.example":
    "Subnet 'lan-home' leases 192.168.1.100–192.168.1.250 with a 24-hour lease time and the router as both gateway and DNS.",

  "inspector.domain.route.map.what":
    "A route map matches and rewrites routing attributes — for example, to set the local preference on a prefix announced to a BGP peer.",
  "inspector.domain.route.map.why":
    "Used by BGP and policy routing. Create one when you need to control which routes are accepted, advertised, or modified.",
  "inspector.domain.route.map.risk":
    "A route map with an empty 'permit' clause can unexpectedly allow every prefix through. Always include an explicit deny or match.",
  "inspector.domain.route.map.example":
    "Map 'set-localpref-200' matches prefix-list 'customer-routes' and raises the local preference to 200 on accepted routes.",

  "inspector.domain.bgp.peer.what":
    "A BGP peer is a neighboring router with which this router exchanges routing information.",
  "inspector.domain.bgp.peer.why":
    "Configure a peer to learn routes from an upstream provider or to advertise your own prefixes.",
  "inspector.domain.bgp.peer.risk":
    "A misconfigured peer can flap or advertise unintended prefixes. Sessions must agree on ASN, hold time, and authentication.",
  "inspector.domain.bgp.peer.example":
    "Peer 'upstream-A' connects to ASN 64500 over TCP 179 with BGP MD5 authentication and advertises 203.0.113.0/24.",

  "inspector.domain.ospf.area.what":
    "An OSPF area is a logical grouping of routers and links inside an OSPF autonomous system.",
  "inspector.domain.ospf.area.why":
    "Inspect the area topology and the routers participating in each area.",
  "inspector.domain.ospf.area.risk":
    "Area misconfiguration can break intra-area routing. Read-only by default; edits require explicit confirmation.",
  "inspector.domain.ospf.area.example":
    "Area 0.0.0.0 (backbone) connects this router with the rest of the site network.",

  "inspector.domain.qos.class.what":
    "A QoS class groups traffic that should receive the same treatment — for example, voice traffic that needs low latency.",
  "inspector.domain.qos.class.why":
    "Define a class for each traffic type, then attach queueing, shaping, or policing rules to it.",
  "inspector.domain.qos.class.risk":
    "Editing a class affects every flow that matches it. A misconfigured class can starve other traffic.",
  "inspector.domain.qos.class.example":
    "Class 'voice' matches UDP 5060 and 10000–20000 and is assigned to a priority queue.",

  "inspector.domain.page.network":
    "Browse every active interface on this router. Read-only at a glance; full configuration lives in the related sub-pages.",
  "inspector.domain.page.wan":
    "Manage the upstream connection(s) to your provider. Edit only when you understand the new values — apply goes through Safe Apply.",
  "inspector.domain.page.lan":
    "Configure the local networks and the clients attached to them. Changes here may briefly disconnect Wi-Fi clients.",
  "inspector.domain.page.routes":
    "Inspect and edit the routing table and BGP sessions. Read-only at a glance; route-map edits propagate immediately.",
  "inspector.domain.page.firewall":
    "Custom firewall rules live here. The default policy still applies unless an explicit rule matches first.",
  "inspector.domain.page.dns":
    "Inspect the zones this router serves and the upstream resolvers it forwards to.",
  "inspector.domain.page.dhcp":
    "See the active leases and the configured subnets. Removing a lease forces that client to renew it on next contact.",
  "inspector.domain.page.services":
    "Read the runtime state of every subsystem — listening sockets, process health, last error. Edits follow Safe Apply.",

  /* Legacy chrome strings */
  "chrome.filter": "Filter",
  "chrome.typeToFilter": "Type to filter…",
  "chrome.allAvailability": "All",
  "chrome.availableOnly": "Available",
  "chrome.nonAvailable": "Not available",
  "chrome.availability": "Availability",
  "chrome.create": "Create",
  "chrome.edit": "Edit",
  "chrome.delete": "Delete",
  "chrome.actions": "Actions",
  "chrome.ofTotal": (n: number, total: number) => `${n} of ${total}`,
  "chrome.technicalDetails": "Technical details",
  "chrome.collectionSnapshot": "Internal snapshot",
  "chrome.emptyKind": "Nothing to show yet.",
  "chrome.total": "Total",
  "chrome.active": "Active",
  "chrome.issues": "Issues",

  /* V4 page chrome */
  "v4.allTechnicalColumns": "Show technical columns",
  "v4.hideTechnicalColumns": "Hide technical columns",
  "v4.emptyFor": "No items for this view.",
  "v4.viewFull": "View in Full mode",
  "v4.refresh": "Refresh",
  "v4.copyAction": "Copy",
  "v4.copied": "Copied",
  "v4.since": "since",
  "v4.units.bytes": (n: number) => formatBytes(n),
  "v4.units.percent": (n: number) => `${Math.round(n)}%`,
};

const ru: V4Bundle = {
  /* Page meta */
  "pageMeta.title": "Страница",
  "pageMeta.designOnly": "Дизайн-каркас",
  "pageMeta.readOnly": "Только чтение",
  "pageMeta.configurable": "Настраивается",
  "pageMeta.irreversible": "Необратимо",

  /* Topbar */
  "topbar.searchOpen": "Поиск",
  "topbar.openSearchHint": "Поиск страниц и справки",
  "topbar.openInspector": "Справка страницы",
  "topbar.themeToggle": "Переключить тему",
  "topbar.localeSelect": "Язык",
  "topbar.endSession": "Завершить сеанс",
  "topbar.modeEasy": "Просто",
  "topbar.modeFull": "Полный",

  /* Shell */
  "shell.skipToMain": "Перейти к содержимому",
  "shell.primaryNav": "Главная навигация",

  /* Rail groups */
  "rail.network": "Сеть",
  "rail.routing": "Маршрутизация",
  "rail.security": "Безопасность",
  "rail.services": "Службы",
  "rail.system": "Система",
  "rail.logs": "Журналы и диагностика",

  /* Easy rail */
  "easy.home": "Дом",
  "easy.internet": "Интернет",
  "easy.wifi": "Wi-Fi",
  "easy.devices": "Устройства",
  "easy.vpn": "VPN",
  "easy.system": "Система",

  /* Easy Home */
  "easyHome.title": "Всё ли работает?",
  "easyHome.subtitle":
    "Однооконный обзор сети. Если что-то не так, вы увидите это здесь первым.",
  "easyHome.headlineLabel": "Общее состояние",
  "easyHome.statusHealthy": "Всё работает",
  "easyHome.statusAttention": "Некоторые вещи требуют внимания",
  "easyHome.statusIncomplete": "Проверка состояния не завершена",
  "easyHome.statusUnavailable": "Состояние недоступно",
  "easyHome.statusVerified": "проверено",
  "easyHome.statusUnverified": "проверка не завершена",
  "easyHome.statusInternetObserved": "Интернет доступен",
  "easyHome.statusInternetVerified": "Интернет доступен",
  "easyHome.statusInternetIncomplete": "Связь не проверена",
  "easyHome.statusInternetUnavailable": "Проверка связи недоступна",
  "easyHome.internetCard": "Интернет",
  "easyHome.internetSub": "Как это устройство связывается с остальным миром.",
  "easyHome.internetConfigure": "Настроить интернет",
  "easyHome.wifiCard": "Wi-Fi",
  "easyHome.wifiSub": "Беспроводные сети, доступные этому устройству.",
  "easyHome.wifiConfigure": "Настроить Wi-Fi",
  "easyHome.wifiDetails": "Подробнее",
  "easyHome.devicesCard": "Устройства",
  "easyHome.devicesSub": "Устройства, видимые в вашей сети.",
  "easyHome.devicesOpen": "Список устройств",
  "easyHome.vpnCard": "VPN",
  "easyHome.vpnSub": "WireGuard и Xray, настроенные на этом устройстве.",
  "easyHome.vpnConfigure": "Открыть VPN",
  "easyHome.systemCard": "Система",
  "easyHome.systemSub": "Общее состояние устройства.",
  "easyHome.systemOpen": "Открыть Doctor",
  "easyHome.aboutTitle": "Об этом устройстве",
  "easyHome.aboutHostname": "Имя узла",
  "easyHome.aboutUptime": "Время работы",
  "easyHome.aboutEnvironment": "Окружение",
  "easyHome.aboutOs": "Операционная система",
  "easyHome.aboutKernel": "Ядро",
  "easyHome.aboutArchitecture": "Архитектура",
  "easyHome.aboutObserved": "Наблюдалось",
  "easyHome.details": "Подробнее",
  "easyHome.unknownValue": "—",

  /* Easy Care */
  "easyCare.title": "Уход и восстановление",
  "easyCare.subtitle":
    "Что видит Doctor и какие кнопки можно нажать для восстановления, когда что-то не так.",
  "easyCare.apiReachable": "API доступен",
  "easyCare.apiUnreachable": "API недоступен",
  "easyCare.findingsCount": "замечаний",
  "easyCare.doctorEmptyTitle": "Текущих замечаний Doctor нет",
  "easyCare.doctorEmptyBody":
    "Doctor вернул пустой отчёт. Подтверждённое состояние системы требует дополнительных данных, которых у нас пока нет.",
  "easyCare.doctorEmptyAction": "Открыть Doctor в полном режиме",
  "easyCare.doctorUnavailableTitle": "Doctor недоступен",
  "easyCare.doctorUnavailableBody":
    "Мы не смогли связаться с Doctor. Это не означает «замечаний нет» — мы ничего не можем подтвердить, пока Doctor не ответит.",
  "easyCare.doctorUnavailableAction": "Повторить",
  "easyCare.findingOpenAction": "Открыть Doctor",
  "easyCare.doctorLoadingTitle": "Проверяем Doctor…",
  "easyCare.doctorLoadingBody":
    "Doctor ещё не ответил. Мы ждём первый отчёт — это не означает «замечаний нет».",

  /* System Overview */
  "sys.title": "Обзор системы",
  "sys.subtitle":
    "Что сейчас не работает, что работает, и есть ли ожидающие изменения.",
  "sys.headline": "Общее состояние",
  "sys.headlineDescription":
    "Один верхний ответ для устройства, опирающийся на наблюдение и Doctor.",
  "sys.sectionHeadline": "Ресурсы в одной сводке",
  "sys.sectionHeadlineSub":
    "Актуальные сигналы системы, обновляются по запросу.",
  "sys.cards": "Карточки ресурсов",
  "sys.cardCpu": "ЦП",
  "sys.cardMemory": "Память",
  "sys.cardStorage": "Хранилище",
  "sys.cardInterfaces": "Интерфейсы",
  "sys.cardRoutes": "Маршруты",
  "sys.cardUptime": "Аптайм",
  "sys.cardDns": "DNS",
  "sys.cardClock": "Часы",
  "sys.identity": "Идентификация",
  "sys.identitySub": "Как это устройство представляется.",
  "sys.storage": "Хранилище",
  "sys.storageSub": "Файловые системы, смонтированные на устройстве.",
  "sys.cores": "логических ядер",
  "sys.percentUsed": "используется",
  "sys.coresFormat": (n: number) =>
    `${n} ${n === 1 ? "ядро" : n < 5 ? "ядра" : "ядер"}`,
  "sys.percentFormat": (pct: number) => `${pct}% используется`,
  "sys.uptimeDaysHours": (d: number, h: number) =>
    `${d} дн ${h} ч с последней загрузки`,
  "sys.uptimeHours": (h: number) => `${h} ч с последней загрузки`,
  "sys.uptimeMinutes": (m: number) => `${m} мин с последней загрузки`,
  "sys.recentFindings": "Недавние находки",
  "sys.noCurrentFindings": "Свежих находок сейчас нет.",
  "sys.dataStale":
    "Проверка состояния не завершена — наблюдение может быть устаревшим.",
  "sys.recentIncidents": "Недавние инциденты",
  "sys.noIncidents": "Недавних инцидентов нет.",
  "sys.loading": "Загрузка…",
  "sys.error": "Что-то пошло не так.",
  "sys.contractMissing": "API-контракт не реализован.",

  "pageWan.findUplink": "Найти восходящий канал",
  "pageWan.defaultGateway": "Шлюз по умолчанию:",
  "pageWan.via": "через",
  "pageWan.noMatch": "Подходящих восходящих каналов нет.",
  "pageWan.noUplinks": "Восходящие каналы не настроены.",
  "pageWan.failoverOrder": "Порядок переключения",
  "pageWan.colInterface": "Интерфейс",
  "pageWan.colUplink": "Канал",
  "pageWan.colState": "Состояние",
  "pageWan.colRole": "Роль",
  "pageWan.colHealth": "Здоровье",
  "pageWan.colIPv4": "IPv4",
  "pageWan.colIPv6": "IPv6",
  "pageWan.colMTU": "MTU",

  /* ActionBar */
  "actionbar.idleAllGood": "Ожидающих изменений нет",
  "actionbar.idleQuiet": "Тихо · нечего применять",
  "actionbar.idleStale":
    "Проверка состояния не завершена · наблюдение может быть устаревшим",
  "actionbar.pending": (n: number) =>
    `${n} ${pluralRu(n, "изменение", "изменения", "изменений")} ожидают применения`,
  "actionbar.apply": "Проверить и применить",
  "actionbar.discard": "Отменить все",
  "actionbar.applying": "Применяется…",
  "actionbar.confirmRequired": (s: number) =>
    `Требуется подтверждение · авто-откат через ${s} с`,
  "actionbar.confirm": "Подтвердить",
  "actionbar.rollback": "Откатить",
  "actionbar.failed": "Применение не удалось",
  "actionbar.failedRecovery": "Открыть восстановление",

  /* Health */
  "health.healthy": "Здоровье",
  "health.needsAttention": "Нужно внимание",
  "health.statusIncomplete": "Состояние неполное",
  "health.observationUntrusted": "Состояние не определено",
  "health.unavailable": "Недоступно",
  "health.observationFresh": "Наблюдение свежее",
  "health.observationStale": "Наблюдение устарело",
  "health.observationNone": "Нет наблюдения",

  /* Search overlay */
  "search.headingPages": "Страницы",
  "search.headingObjects": "Устройства и адреса",
  "search.headingSections": "Разделы",
  "search.headingActions": "Действия",
  "search.placeholder": "Поиск страниц и справки",
  "search.openHint": "Закрыть · Esc",
  "search.empty": "Ничего не найдено",
  "search.emptyHint": "Попробуйте другое имя, роль или раздел.",
  "search.tooShort": "Продолжайте вводить",
  "search.tooShortHint": "Нужно хотя бы два символа.",
  "search.resultsHint": (n: number) =>
    `${n} ${pluralRu(n, "результат", "результата", "результатов")} · ↵ чтобы открыть · ↑↓ для перехода`,

  /* Empty states */
  "empty.wireguardPeers.title": "Пир-узлов пока нет",
  "empty.wireguardPeers.body":
    "Пир-узлы WireGuard позволяют другим устройствам безопасно подключаться к этой сети. Создайте пир-узел, чтобы выпустить ключ и конфигурацию.",
  "empty.wireguardPeers.action": "Добавить пир-узел WireGuard",
  "empty.firewallRules.title": "Пользовательских правил нет",
  "empty.firewallRules.body":
    "Трафик идёт по политике по умолчанию. Добавьте правило, чтобы разрешить, отклонить или заблокировать нужные потоки.",
  "empty.firewallRules.action": "Создать правило межсетевого экрана",
  "empty.dhcpLeases.title": "Пока никто не запросил адрес",
  "empty.dhcpLeases.body":
    "За текущее окно наблюдения ни одно устройство не получило адрес.",
  "empty.dhcpLeases.action": "Показать области DHCP",
  "empty.xray.title": "Профилей Xray нет",
  "empty.xray.body":
    "Создайте профиль, чтобы выдать клиентский доступ. Каждый профиль описывает пользователя и его правила маршрутизации.",
  "empty.xray.action": "Создать профиль Xray",
  "empty.sites.title": "Сайтов репликации нет",
  "empty.sites.body":
    "Сайты связывают это устройство с другими установками DuckOS, чтобы конфигурация и состояние перемещались вместе.",
  "empty.sites.action": "Добавить сайт",
  "empty.vpn.title": "VPN не настроен",
  "empty.vpn.body":
    "Настройте WireGuard или Xray, чтобы открыть это устройство в удалённой сети или для удалённых клиентов.",
  "empty.interfaces.title": "Интерфейсов нет",
  "empty.interfaces.body":
    "Сейчас не видно ни одного интерфейса. Проверьте, что ядро сообщает хотя бы об одном устройстве.",
  "empty.routes.title": "Маршрутов нет",
  "empty.routes.body":
    "Маршруты появятся здесь, как только ядро узнает о них.",
  "empty.dns.title": "DNS не настроен",
  "empty.dns.body":
    "Добавьте резолвер, чтобы устройство знало, куда отправлять DNS-запросы.",
  "empty.services.title": "Служб нет",
  "empty.services.body":
    "Сейчас служб нет. Новые службы появятся после первого цикла наблюдения.",
  "empty.generic.title": "Здесь пока пусто",
  "empty.generic.body":
    "Сейчас этот вид пуст. Когда соответствующая функция будет настроена, элементы появятся здесь.",

  /* Inspector */
  "inspector.tabs.what": "Что это?",
  "inspector.tabs.why": "Зачем это?",
  "inspector.tabs.risk": "Что меняется, если я редактирую?",
  "inspector.tabs.example": "Пример",
  "inspector.openCTA": "Открыть справку страницы",
  "inspector.technicalDetails": "Технические детали",
  "inspector.noObjectSelected": "Выберите строку, чтобы увидеть детали.",
  "inspector.kind": "Тип",
  "inspector.snapshot": "Снимок",
  "inspector.objectId": "Идентификатор объекта",

  /* Per-domain Inspector copy */
  "inspector.domain.network.interface.what":
    "Интерфейсы — физические и виртуальные сетевые подключения на этом маршрутизаторе: LAN, WAN, мосты, VLAN, туннели.",
  "inspector.domain.network.interface.why":
    "Просмотрите активные подключения. Здесь удобно проверить, что порт поднят, и какие IPv4/IPv6-адреса ему назначены.",
  "inspector.domain.network.interface.risk":
    "Чтение всегда безопасно. Редактирование может изменить состав интерфейсов и разорвать активные сессии.",
  "inspector.domain.network.interface.example":
    "eth0 — внешний WAN-порт — отображается как administratively up с одним IPv4 и одним IPv6-адресом от провайдера.",

  "inspector.domain.network.address.what":
    "IP-адрес привязан к конкретному интерфейсу. Вместе с длиной префикса он определяет подсеть, в которой участвует интерфейс.",
  "inspector.domain.network.address.why":
    "Узнайте, какой интерфейс владеет нужным адресом, длину префикса и убедитесь, что адрес сейчас доступен.",
  "inspector.domain.network.address.risk":
    "Чтение безопасно. Редактирование может перенести адрес между интерфейсами и нарушить работу активных сессий.",
  "inspector.domain.network.address.example":
    "192.0.2.1/24 на eth0 относится к LAN-мосту, а 203.0.113.4/32 на eth1 — это WAN-адрес от провайдера.",

  "inspector.domain.network.firewall.rule.what":
    "Правила firewall определяют, какой трафик разрешён, отклонён или отброшен между сетями. Правила применяются по порядку до первого совпадения.",
  "inspector.domain.network.firewall.rule.why":
    "Добавляйте исключения к политике по умолчанию — например, чтобы открыть сервис на LAN или заблокировать конкретный адрес.",
  "inspector.domain.network.firewall.rule.risk":
    "Ошибка в правиле может заблокировать доступ к маршрутизатору или открыть сервисы наружу. Изменения проходят через Safe Apply.",
  "inspector.domain.network.firewall.rule.example":
    "Разрешить TCP 443 с LAN на конкретный внутренний хост, чтобы домашняя панель была доступна с телефона по Wi-Fi.",

  "inspector.domain.network.firewall.set.what":
    "Наборы firewall — это группы адресов, портов или интерфейсов, на которые правила могут ссылаться по имени.",
  "inspector.domain.network.firewall.set.why":
    "Определите набор один раз и используйте его во многих правилах, чтобы политика оставалась читаемой и обновлялась в одном месте.",
  "inspector.domain.network.firewall.set.risk":
    "Переименование или удаление набора ломает все правила, которые на него ссылаются.",
  "inspector.domain.network.firewall.set.example":
    "Набор «guest-clients» содержит все адреса гостевого моста — одно правило открывает гостям только интернет.",

  "inspector.domain.network.lease.what":
    "DHCP-lease — это временная привязка MAC-адреса к IP-адресу, выданная DHCP-сервером.",
  "inspector.domain.network.lease.why":
    "Смотрите, какие устройства получили адрес, когда истекает lease и из какого диапазона он выдан.",
  "inspector.domain.network.lease.risk":
    "Чтение безопасно. Удаление lease принудит клиента получить новый адрес при следующем обращении.",
  "inspector.domain.network.lease.example":
    "Phone-on-wifi — 192.168.1.42 — lease действует 12 часов из домашнего диапазона.",

  "inspector.domain.wireguard.server.what":
    "WireGuard-сервер — это публичная сторона VPN: он слушает UDP-порт и принимает подключения от настроенных peer-ов.",
  "inspector.domain.wireguard.server.why":
    "Создайте сервер, если нужно открыть LAN, направить весь клиентский трафик через маршрутизатор или связать две площадки.",
  "inspector.domain.wireguard.server.risk":
    "Неверный порт или ключ не позволят peer-ам подключиться. Смена ключа делает недействительной конфигурацию всех peer-ов.",
  "inspector.domain.wireguard.server.example":
    "Сервер «home-vpn» слушает UDP 51820 в туннеле 10.66.0.0/24 и направляет весь клиентский трафик через WAN.",

  "inspector.domain.wireguard.peer.what":
    "WireGuard peer — это один клиент или удалённая площадка, которая подключается к WireGuard-серверу на этом маршрутизаторе.",
  "inspector.domain.wireguard.peer.why":
    "Добавьте peer для каждого устройства или площадки в VPN. У каждого peer-а своя пара ключей и адрес в туннеле.",
  "inspector.domain.wireguard.peer.risk":
    "Удаление peer-а сразу отключает устройство от VPN. Публичный ключ peer-а после удаления восстановить нельзя.",
  "inspector.domain.wireguard.peer.example":
    "Peer «alice-laptop» подключается с телефона в роуминге и получает фиксированный туннельный адрес 10.66.0.2.",

  "inspector.domain.xray.server.what":
    "Xray-сервер — это прокси-узел, работающий на маршрутизаторе и пересылающий трафик настроенных пользователей.",
  "inspector.domain.xray.server.why":
    "Настройте Xray-сервер, когда нужно, чтобы пользователи выходили в интернет через этот маршрутизатор по заданному протоколу и порту.",
  "inspector.domain.xray.server.risk":
    "Неверный транспорт или учётные данные сделают подключение невозможным для всех пользователей. Выбор протокола влияет на совместимость.",
  "inspector.domain.xray.server.example":
    "Сервер «edge-1» принимает VLESS+Reality на TCP 443 и отправляет пользовательский трафик в WAN.",

  "inspector.domain.xray.user.what":
    "Xray-пользователь — это клиент Xray-сервера с учётными данными. У каждого свой UUID, шифрование и лимиты трафика.",
  "inspector.domain.xray.user.why":
    "Создайте по пользователю на человека или устройство. Пользователь получает строку подключения или ссылку для клиента.",
  "inspector.domain.xray.user.risk":
    "Удаление пользователя сразу отзывает его доступ. Удалённого пользователя нельзя восстановить без экспортированных учётных данных.",
  "inspector.domain.xray.user.example":
    "Пользователь «family-phone» использует UUID-аутентификацию, flow «xtls-rprx-vision» и 100 ГБ в месяц.",

  "inspector.domain.dns.zone.what":
    "DNS-зона — это часть пространства имён, для которой этот маршрутизатор авторитативен, например локальная домашняя зона.",
  "inspector.domain.dns.zone.why":
    "Посмотрите, какие зоны существуют, какие записи в них хранятся и какие серверы их обслуживают.",
  "inspector.domain.dns.zone.risk":
    "Чтение безопасно. Изменение зон может повлиять на то, как разрешаются имена в LAN.",
  "inspector.domain.dns.zone.example":
    "Зона «home.lan» обслуживает записи всех устройств LAN и перенаправляет неизвестные имена на внешние резолверы.",

  "inspector.domain.dhcp.subnet.what":
    "DHCP-подсеть — это пул адресов и параметры (шлюз, DNS, время lease), выдаваемые клиентам в сегменте сети.",
  "inspector.domain.dhcp.subnet.why":
    "Создайте одну DHCP-подсеть на каждый сегмент LAN. В ней задаётся диапазон адресов, которые сервер вправе выдавать.",
  "inspector.domain.dhcp.subnet.risk":
    "Неверные шлюз или DNS могут изолировать клиентов от интернета или от локального разрешения имён.",
  "inspector.domain.dhcp.subnet.example":
    "Подсеть «lan-home» выдаёт 192.168.1.100–192.168.1.250 с временем lease 24 часа; шлюз и DNS — сам маршрутизатор.",

  "inspector.domain.route.map.what":
    "Route map сопоставляет и переписывает атрибуты маршрутов — например, задаёт local preference для префикса, анонсируемого BGP peer-у.",
  "inspector.domain.route.map.why":
    "Используется в BGP и policy routing. Создавайте route map, когда нужно контролировать, какие маршруты принимаются, анонсируются или изменяются.",
  "inspector.domain.route.map.risk":
    "Route map с пустым «permit» может непреднамеренно пропустить все префиксы. Всегда добавляйте явный deny или match.",
  "inspector.domain.route.map.example":
    "Map «set-localpref-200» сопоставляет prefix-list «customer-routes» и поднимает local preference до 200 для принятых маршрутов.",

  "inspector.domain.bgp.peer.what":
    "BGP peer — это соседний маршрутизатор, с которым этот маршрутизатор обменивается информацией о маршрутах.",
  "inspector.domain.bgp.peer.why":
    "Настройте peer-а, чтобы получать маршруты от внешнего провайдера или анонсировать свои собственные префиксы.",
  "inspector.domain.bgp.peer.risk":
    "Неверно настроенный peer может флапать или анонсировать ненужные префиксы. Соседи должны согласовать ASN, hold time и аутентификацию.",
  "inspector.domain.bgp.peer.example":
    "Peer «upstream-A» подключается к ASN 64500 по TCP 179 с BGP MD5-аутентификацией и анонсирует 203.0.113.0/24.",

  "inspector.domain.ospf.area.what":
    "Область OSPF — это логическая группа маршрутизаторов и каналов внутри автономной системы OSPF.",
  "inspector.domain.ospf.area.why":
    "Просмотрите топологию областей и маршрутизаторы, участвующие в каждой из них.",
  "inspector.domain.ospf.area.risk":
    "Неверная конфигурация области может сломать внутриобластную маршрутизацию. По умолчанию только чтение.",
  "inspector.domain.ospf.area.example":
    "Область 0.0.0.0 (backbone) связывает этот маршрутизатор с остальной сетью площадки.",

  "inspector.domain.qos.class.what":
    "Класс QoS объединяет трафик, который должен обрабатываться одинаково, например голосовой трафик с низкой задержкой.",
  "inspector.domain.qos.class.why":
    "Определите класс для каждого типа трафика, затем прикрепите к нему очереди, шейпинг или полисинг.",
  "inspector.domain.qos.class.risk":
    "Изменение класса влияет на все потоки, которые ему соответствуют. Неверный класс может задушить другой трафик.",
  "inspector.domain.qos.class.example":
    "Класс «voice» соответствует UDP 5060 и 10000–20000 и помещается в приоритетную очередь.",

  "inspector.domain.page.network":
    "Просмотрите все активные интерфейсы на этом маршрутизаторе. Только чтение; полная настройка — в связанных подразделах.",
  "inspector.domain.page.wan":
    "Управляйте подключениями к провайдеру. Редактируйте, только если понимаете новые значения: изменения проходят через Safe Apply.",
  "inspector.domain.page.lan":
    "Настройте локальные сети и подключённых к ним клиентов. Изменения могут кратковременно отключить Wi-Fi-клиентов.",
  "inspector.domain.page.routes":
    "Просмотр и редактирование таблицы маршрутизации и BGP-сессий. Только чтение; правки route map применяются сразу.",
  "inspector.domain.page.firewall":
    "Здесь живут пользовательские правила firewall. Политика по умолчанию применяется, пока нет явного совпадения.",
  "inspector.domain.page.dns":
    "Посмотрите зоны, которые обслуживает этот маршрутизатор, и внешние резолверы, на которые он перенаправляет запросы.",
  "inspector.domain.page.dhcp":
    "Активные lease-ы и настроенные подсети. Удаление lease-а заставит клиента получить новый адрес при следующем обращении.",
  "inspector.domain.page.services":
    "Прочтите состояние каждой подсистемы: слушающие сокеты, здоровье процессов, последняя ошибка. Изменения через Safe Apply.",

  /* Legacy chrome strings */
  "chrome.filter": "Фильтр",
  "chrome.typeToFilter": "Введите для фильтра…",
  "chrome.allAvailability": "Все",
  "chrome.availableOnly": "Доступные",
  "chrome.nonAvailable": "Недоступные",
  "chrome.availability": "Доступность",
  "chrome.create": "Создать",
  "chrome.edit": "Изменить",
  "chrome.delete": "Удалить",
  "chrome.actions": "Действия",
  "chrome.ofTotal": (n: number, total: number) => `${n} из ${total}`,
  "chrome.technicalDetails": "Технические детали",
  "chrome.collectionSnapshot": "Внутренний снимок",
  "chrome.emptyKind": "Пока ничего нет.",
  "chrome.total": "Всего",
  "chrome.active": "Активно",
  "chrome.issues": "Проблем",

  /* V4 page chrome */
  "v4.allTechnicalColumns": "Показать технические колонки",
  "v4.hideTechnicalColumns": "Скрыть технические колонки",
  "v4.emptyFor": "Нет элементов для этого вида.",
  "v4.viewFull": "Открыть в Полном режиме",
  "v4.refresh": "Обновить",
  "v4.copyAction": "Скопировать",
  "v4.copied": "Скопировано",
  "v4.since": "с",
  "v4.units.bytes": (n: number) => formatBytes(n),
  "v4.units.percent": (n: number) => `${Math.round(n)}%`,
};

const de: V4Bundle = {
  /* Page meta */
  "pageMeta.title": "Seite",
  "pageMeta.designOnly": "Design-Entwurf",
  "pageMeta.readOnly": "Schreibgeschützt",
  "pageMeta.configurable": "Konfigurierbar",
  "pageMeta.irreversible": "Irreversibel",

  "topbar.searchOpen": "Suche",
  "topbar.openSearchHint": "Seiten und Hilfe suchen",
  "topbar.openInspector": "Hilfe zur Seite",
  "topbar.themeToggle": "Thema wechseln",
  "topbar.localeSelect": "Sprache",
  "topbar.endSession": "Sitzung beenden",
  "topbar.modeEasy": "Einfach",
  "topbar.modeFull": "Voll",

  "shell.skipToMain": "Zum Inhalt springen",
  "shell.primaryNav": "Hauptnavigation",

  "rail.network": "Netzwerk",
  "rail.routing": "Routing",
  "rail.security": "Sicherheit",
  "rail.services": "Dienste",
  "rail.system": "System",
  "rail.logs": "Protokolle & Diagnose",

  "easy.home": "Start",
  "easy.internet": "Internet",
  "easy.wifi": "WLAN",
  "easy.devices": "Geräte",
  "easy.vpn": "VPN",
  "easy.system": "System",

  "easyHome.title": "Läuft alles?",
  "easyHome.subtitle":
    "Eine Übersicht über das Netzwerk. Wenn etwas nicht stimmt, sehen Sie es hier zuerst.",
  "easyHome.headlineLabel": "Gesamt",
  "easyHome.statusHealthy": "Alles funktioniert",
  "easyHome.statusAttention": "Manche Dinge brauchen Aufmerksamkeit",
  "easyHome.statusIncomplete": "Statusprüfung unvollständig",
  "easyHome.statusUnavailable": "Status nicht verfügbar",
  "easyHome.statusVerified": "geprüft",
  "easyHome.statusUnverified": "Prüfung ausstehend",
  "easyHome.statusInternetObserved": "Internet erreichbar",
  "easyHome.statusInternetVerified": "Internet erreichbar",
  "easyHome.statusInternetIncomplete": "Verbindung nicht verifiziert",
  "easyHome.statusInternetUnavailable": "Verbindungsprüfung nicht verfügbar",
  "easyHome.internetCard": "Internet",
  "easyHome.internetSub": "Wie dieses Gerät die Welt erreicht.",
  "easyHome.internetConfigure": "Internet einrichten",
  "easyHome.wifiCard": "WLAN",
  "easyHome.wifiSub": "Drahtlosnetze, die dieses Gerät erreichen kann.",
  "easyHome.wifiConfigure": "WLAN einrichten",
  "easyHome.wifiDetails": "Details",
  "easyHome.devicesCard": "Geräte",
  "easyHome.devicesSub": "Geräte in Ihrem Netz.",
  "easyHome.devicesOpen": "Geräte anzeigen",
  "easyHome.vpnCard": "VPN",
  "easyHome.vpnSub": "WireGuard- und Xray-Dienste auf diesem Gerät.",
  "easyHome.vpnConfigure": "VPN öffnen",
  "easyHome.systemCard": "System",
  "easyHome.systemSub": "Gesamtzustand des Geräts.",
  "easyHome.systemOpen": "Doctor öffnen",
  "easyHome.aboutTitle": "Über dieses Gerät",
  "easyHome.aboutHostname": "Hostname",
  "easyHome.aboutUptime": "Betriebszeit",
  "easyHome.aboutEnvironment": "Umgebung",
  "easyHome.aboutOs": "Betriebssystem",
  "easyHome.aboutKernel": "Kernel",
  "easyHome.aboutArchitecture": "Architektur",
  "easyHome.aboutObserved": "Beobachtet",
  "easyHome.details": "Details",
  "easyHome.unknownValue": "—",

  /* Easy Care */
  "easyCare.title": "Pflege & Wiederherstellung",
  "easyCare.subtitle":
    "Was Doctor sieht und welche Schaltflächen Sie drücken können, wenn etwas nicht stimmt.",
  "easyCare.apiReachable": "API erreichbar",
  "easyCare.apiUnreachable": "API nicht erreichbar",
  "easyCare.findingsCount": "Befunde",
  "easyCare.doctorEmptyTitle": "Keine aktuellen Doctor-Befunde",
  "easyCare.doctorEmptyBody":
    "Doctor hat einen leeren Bericht zurückgegeben. Verifizierter Systemzustand erfordert zusätzliche Nachweise, die wir derzeit nicht haben.",
  "easyCare.doctorEmptyAction": "Doctor im Vollmodus öffnen",
  "easyCare.doctorUnavailableTitle": "Doctor-Status nicht verfügbar",
  "easyCare.doctorUnavailableBody":
    "Wir konnten Doctor nicht erreichen. Das ist nicht dasselbe wie \u201ekeine Befunde\u201c \u2014 wir können nichts bestätigen, bis Doctor antwortet.",
  "easyCare.doctorUnavailableAction": "Wiederholen",
  "easyCare.findingOpenAction": "Doctor öffnen",
  "easyCare.doctorLoadingTitle": "Doctor wird geprüft …",
  "easyCare.doctorLoadingBody":
    "Doctor hat noch nicht geantwortet. Wir warten auf den ersten Bericht — das ist nicht dasselbe wie \u201ekeine Befunde\u201c.",

  "sys.title": "Systemübersicht",
  "sys.subtitle":
    "Was fällt gerade aus, was läuft, und ist eine Änderung offen?",
  "sys.headline": "Gesamt",
  "sys.headlineDescription":
    "Eine klare Aussage zum Gerät, abgeleitet aus Beobachtung und Doctor.",
  "sys.sectionHeadline": "Ressourcen im Überblick",
  "sys.sectionHeadlineSub":
    "Live-Signale aus dem System, auf Abruf aktualisiert.",
  "sys.cards": "Ressourcenkarten",
  "sys.cardCpu": "CPU",
  "sys.cardMemory": "Speicher",
  "sys.cardStorage": "Speicherplatz",
  "sys.cardInterfaces": "Schnittstellen",
  "sys.cardRoutes": "Routen",
  "sys.cardUptime": "Laufzeit",
  "sys.cardDns": "DNS",
  "sys.cardClock": "Uhr",
  "sys.identity": "Identität",
  "sys.identitySub": "Wie sich dieses Gerät darstellt.",
  "sys.storage": "Speicher",
  "sys.storageSub": "Eingehängte Dateisysteme.",
  "sys.cores": "logische Kerne",
  "sys.percentUsed": "in Benutzung",
  "sys.coresFormat": (n: number) => `${n} Kerne`,
  "sys.percentFormat": (pct: number) => `${pct}% in Benutzung`,
  "sys.uptimeDaysHours": (d: number, h: number) =>
    `${d} T ${h} Std seit dem letzten Start`,
  "sys.uptimeHours": (h: number) => `${h} Std seit dem letzten Start`,
  "sys.uptimeMinutes": (m: number) => `${m} Min seit dem letzten Start`,
  "sys.recentFindings": "Aktuelle Befunde",
  "sys.noCurrentFindings": "Derzeit sind keine Befunde verfügbar.",
  "sys.dataStale":
    "Statusprüfung unvollständig — Beobachtung kann veraltet sein.",
  "sys.recentIncidents": "Aktuelle Vorfälle",
  "sys.noIncidents": "Keine aktuellen Vorfälle.",
  "sys.loading": "Lädt…",
  "sys.error": "Etwas ist schiefgelaufen.",
  "sys.contractMissing": "API-Vertrag nicht implementiert.",

  "pageWan.findUplink": "Uplink suchen",
  "pageWan.defaultGateway": "Standard-Gateway:",
  "pageWan.via": "über",
  "pageWan.noMatch": "Keine Uplinks für diese Ansicht.",
  "pageWan.noUplinks": "Keine Uplinks konfiguriert.",
  "pageWan.failoverOrder": "Failover-Reihenfolge",
  "pageWan.colInterface": "Schnittstelle",
  "pageWan.colUplink": "Uplink",
  "pageWan.colState": "Status",
  "pageWan.colRole": "Rolle",
  "pageWan.colHealth": "Zustand",
  "pageWan.colIPv4": "IPv4",
  "pageWan.colIPv6": "IPv6",
  "pageWan.colMTU": "MTU",

  "actionbar.idleAllGood": "Keine ausstehenden Änderungen",
  "actionbar.idleQuiet": "Ruhig · nichts anzuwenden",
  "actionbar.idleStale":
    "Statusprüfung unvollständig · Beobachtung kann veraltet sein",
  "actionbar.pending": (n: number) =>
    `${n} Änderung${n === 1 ? "" : "en"} wartet${n === 1 ? "" : "en"} auf Anwendung`,
  "actionbar.apply": "Prüfen & Anwenden",
  "actionbar.discard": "Alle verwerfen",
  "actionbar.applying": "Wird angewendet…",
  "actionbar.confirmRequired": (s: number) =>
    `Bestätigung erforderlich · automatisches Rollback in ${s}s`,
  "actionbar.confirm": "Bestätigen",
  "actionbar.rollback": "Rollback",
  "actionbar.failed": "Anwendung fehlgeschlagen",
  "actionbar.failedRecovery": "Wiederherstellung öffnen",

  "health.healthy": "Gesund",
  "health.needsAttention": "Aufmerksamkeit nötig",
  "health.statusIncomplete": "Status unvollständig",
  "health.observationUntrusted": "Status nicht verifiziert",
  "health.unavailable": "Nicht verfügbar",
  "health.observationFresh": "Beobachtung aktuell",
  "health.observationStale": "Beobachtung veraltet",
  "health.observationNone": "Keine Beobachtung",

  "search.headingPages": "Seiten",
  "search.headingObjects": "Geräte & Adressen",
  "search.headingSections": "Bereiche",
  "search.headingActions": "Aktionen",
  "search.placeholder": "Seiten und Hilfe suchen",
  "search.openHint": "Schließen · Esc",
  "search.empty": "Keine Treffer",
  "search.emptyHint":
    "Versuchen Sie einen anderen Namen, eine andere Rolle oder einen anderen Bereich.",
  "search.tooShort": "Weiter tippen",
  "search.tooShortHint": "Mindestens zwei Zeichen eingeben.",
  "search.resultsHint": (n: number) =>
    `${n} Treffer · ↵ zum Öffnen · ↑↓ zum Navigieren`,

  "empty.wireguardPeers.title": "Noch keine Peers",
  "empty.wireguardPeers.body":
    "WireGuard-Peers ermöglichen anderen Geräten, diesem Netz sicher beizutreten. Erstellen Sie einen Peer, um Schlüssel und Konfiguration auszugeben.",
  "empty.wireguardPeers.action": "WireGuard-Peer hinzufügen",
  "empty.firewallRules.title": "Keine individuellen Firewall-Regeln",
  "empty.firewallRules.body":
    "Der Verkehr folgt der Standardrichtlinie. Fügen Sie eine Regel hinzu, um Datenflüsse zu erlauben, abzulehnen oder zu blockieren.",
  "empty.firewallRules.action": "Firewall-Regel erstellen",
  "empty.dhcpLeases.title": "Noch hat kein Gerät eine Adresse angefordert",
  "empty.dhcpLeases.body":
    "Im aktuellen Beobachtungsfenster hat kein Client eine Adresse erhalten.",
  "empty.dhcpLeases.action": "DHCP-Bereiche anzeigen",
  "empty.xray.title": "Keine Xray-Profile",
  "empty.xray.body":
    "Erstellen Sie ein Profil, um Client-Zugang auszustellen.",
  "empty.xray.action": "Xray-Profil erstellen",
  "empty.sites.title": "Keine Replikationsstandorte",
  "empty.sites.body":
    "Standorte verbinden dieses Gerät mit anderen DuckOS-Installationen, damit Konfiguration und Zustand zusammen reisen.",
  "empty.sites.action": "Standort hinzufügen",
  "empty.vpn.title": "Kein VPN eingerichtet",
  "empty.vpn.body":
    "Richten Sie WireGuard oder Xray ein, um dieses Gerät für ein entferntes Netz oder für entfernte Clients zu öffnen.",
  "empty.interfaces.title": "Keine Schnittstellen",
  "empty.interfaces.body":
    "Aktuell sind keine Schnittstellen sichtbar. Prüfen Sie, dass der Kernel mindestens ein Gerät meldet.",
  "empty.routes.title": "Keine Routen",
  "empty.routes.body":
    "Routen erscheinen hier, sobald der Kernel sie gelernt hat.",
  "empty.dns.title": "Keine DNS-Konfiguration",
  "empty.dns.body":
    "Fügen Sie einen Resolver hinzu, damit dieses Gerät weiß, wohin DNS-Anfragen gehen.",
  "empty.services.title": "Keine Dienste",
  "empty.services.body":
    "Derzeit sind keine Dienste aufgeführt. Neue Dienste erscheinen nach dem ersten Beobachtungszyklus.",
  "empty.generic.title": "Hier ist noch nichts",
  "empty.generic.body":
    "Diese Ansicht ist gerade leer. Sobald die zugehörige Funktion konfiguriert ist, erscheinen hier Einträge.",

  "inspector.tabs.what": "Was ist das?",
  "inspector.tabs.why": "Wozu dient es?",
  "inspector.tabs.risk": "Was passiert, wenn ich es ändere?",
  "inspector.tabs.example": "Beispiel",
  "inspector.openCTA": "Hilfe zur Seite öffnen",
  "inspector.technicalDetails": "Technische Details",
  "inspector.noObjectSelected":
    "Wählen Sie eine Zeile aus, um Details anzuzeigen.",
  "inspector.kind": "Typ",
  "inspector.snapshot": "Schnappschuss",
  "inspector.objectId": "Objekt-ID",

  /* Per-domain Inspector copy */
  "inspector.domain.network.interface.what":
    "Schnittstellen sind die physischen und virtuellen Netzwerkverbindungen dieses Routers — LAN, WAN, Bridges, VLANs, Tunnel.",
  "inspector.domain.network.interface.why":
    "Aktive Verbindungen durchsehen. Hier prüfen Sie, ob ein Port administrativ aktiv ist und welche IPv4/IPv6-Adressen ihm zugewiesen sind.",
  "inspector.domain.network.interface.risk":
    "Lesen ist immer sicher. Bearbeitungen können die Schnittstellenliste ändern und aktive Sitzungen trennen.",
  "inspector.domain.network.interface.example":
    "eth0 — der WAN-Uplink — wird als administrativ aktiv angezeigt, mit einer IPv4- und einer IPv6-Adresse vom Provider.",

  "inspector.domain.network.address.what":
    "Eine IP-Adresse ist an eine bestimmte Schnittstelle gebunden. Zusammen mit der Präfixlänge definiert sie das Subnetz.",
  "inspector.domain.network.address.why":
    "Hier sehen Sie, welche Schnittstelle eine IP besitzt, wie lang das Präfix ist und ob die Adresse erreichbar ist.",
  "inspector.domain.network.address.risk":
    "Lesen ist sicher. Bearbeitungen können Adressen zwischen Schnittstellen verschieben und aktive Sitzungen stören.",
  "inspector.domain.network.address.example":
    "192.0.2.1/24 auf eth0 gehört zur LAN-Bridge, 203.0.113.4/32 auf eth1 ist die vom Provider zugewiesene WAN-Adresse.",

  "inspector.domain.network.firewall.rule.what":
    "Firewall-Regeln legen fest, welcher Verkehr zwischen Netzen erlaubt, abgelehnt oder verworfen wird. Regeln werden in der Reihenfolge geprüft, bis eine passt.",
  "inspector.domain.network.firewall.rule.why":
    "Ergänzen Sie die Standardrichtlinie um Ausnahmen — etwa um einen Dienst im LAN freizugeben oder ein Ziel zu sperren.",
  "inspector.domain.network.firewall.rule.risk":
    "Falsche Regeln können Sie aussperren oder Dienste im Internet freigeben. Änderungen laufen über Safe Apply.",
  "inspector.domain.network.firewall.rule.example":
    "Erlauben Sie TCP 443 vom LAN zu einem internen Host, damit das Heim-Dashboard vom Telefon im WLAN erreichbar ist.",

  "inspector.domain.network.firewall.set.what":
    "Firewall-Sets sind wiederverwendbare Gruppen aus Adressen, Ports oder Schnittstellen, auf die Regeln mit Namen verweisen.",
  "inspector.domain.network.firewall.set.why":
    "Sets einmal anlegen und in vielen Regeln referenzieren — so bleibt die Richtlinie lesbar und ein Edit wirkt überall.",
  "inspector.domain.network.firewall.set.risk":
    "Umbenennen oder Löschen eines Sets kann jede Regel brechen, die darauf verweist.",
  "inspector.domain.network.firewall.set.example":
    "Das Set «guest-clients» enthält alle Adressen der Gast-Bridge — eine Regel gewährt Gästen ausschließlich Internetzugang.",

  "inspector.domain.network.lease.what":
    "Ein DHCP-Lease ist die temporäre Bindung zwischen MAC- und IP-Adresse, die der DHCP-Server vergeben hat.",
  "inspector.domain.network.lease.why":
    "Sehen Sie, welche Geräte eine Adresse erhalten haben, wann der Lease abläuft und aus welchem Bereich er stammt.",
  "inspector.domain.network.lease.risk":
    "Lesen ist sicher. Das manuelle Entfernen eines Leases erzwingt eine Erneuerung beim nächsten Kontakt.",
  "inspector.domain.network.lease.example":
    "Phone-on-wifi — 192.168.1.42 — Lease 12 Stunden gültig aus dem Heim-Bereich.",

  "inspector.domain.wireguard.server.what":
    "Ein WireGuard-Server ist die öffentliche Seite des VPN — er hört auf einem UDP-Port und nimmt Peers entgegen.",
  "inspector.domain.wireguard.server.why":
    "Richten Sie einen Server ein, um das LAN freizugeben, den gesamten Clientverkehr umzuleiten oder zwei Standorte zu verbinden.",
  "inspector.domain.wireguard.server.risk":
    "Falscher Port oder Schlüssel verhindert Peer-Verbindungen. Schlüsseländerungen machen alle Peer-Konfigurationen ungültig.",
  "inspector.domain.wireguard.server.example":
    "Server «home-vpn» hört auf UDP 51820 im Tunnel 10.66.0.0/24 und leitet den gesamten Peer-Verkehr durch das WAN.",

  "inspector.domain.wireguard.peer.what":
    "Ein WireGuard-Peer ist ein einzelner Client oder entfernter Standort, der sich mit einem WireGuard-Server dieses Routers verbindet.",
  "inspector.domain.wireguard.peer.why":
    "Pro Gerät oder Standort im VPN einen Peer anlegen. Jeder Peer erhält ein eigenes Schlüsselpaar und eine Tunneladresse.",
  "inspector.domain.wireguard.peer.risk":
    "Löschen trennt das Gerät sofort vom VPN. Der öffentliche Schlüssel des Peers ist danach nicht wiederherstellbar.",
  "inspector.domain.wireguard.peer.example":
    "Peer «alice-laptop» verbindet sich vom Telefon im Roaming und bekommt die feste Tunneladresse 10.66.0.2.",

  "inspector.domain.xray.server.what":
    "Ein Xray-Server ist ein Proxy-Endpunkt auf diesem Router, der den Verkehr konfigurierter Benutzer weiterleitet.",
  "inspector.domain.xray.server.why":
    "Richten Sie einen Xray-Server ein, wenn Benutzer über diesen Router mit einem bestimmten Protokoll und Port ins Internet gehen sollen.",
  "inspector.domain.xray.server.risk":
    "Falscher Transport oder falsche Anmeldedaten verhindern Verbindungen für alle Benutzer. Die Protokollwahl beeinflusst die Client-Kompatibilität.",
  "inspector.domain.xray.server.example":
    "Server «edge-1» nimmt VLESS+Reality auf TCP 443 an und leitet Benutzerverkehr ins WAN.",

  "inspector.domain.xray.user.what":
    "Ein Xray-Benutzer ist ein autorisierter Client eines Xray-Servers. Jeder Benutzer hat eine Identität (UUID), Verschlüsselung und Limits.",
  "inspector.domain.xray.user.why":
    "Pro Person oder Gerät einen Benutzer anlegen. Benutzer erhalten eine Verbindungszeichenkette oder einen Link für ihren Client.",
  "inspector.domain.xray.user.risk":
    "Löschen entzieht den Zugang sofort. Gelöschte Benutzer sind ohne vorherigen Export der Anmeldedaten nicht wiederherstellbar.",
  "inspector.domain.xray.user.example":
    "Benutzer «family-phone» nutzt UUID-Authentifizierung mit Flow «xtls-rprx-vision» und 100 GiB pro Monat.",

  "inspector.domain.dns.zone.what":
    "Eine DNS-Zone ist ein Namensraum-Bereich, für den dieser Router autoritativ ist — etwa die lokale Heimzone.",
  "inspector.domain.dns.zone.why":
    "Sehen Sie, welche Zonen existieren, welche Einträge sie enthalten und welche Server sie ausliefern.",
  "inspector.domain.dns.zone.risk":
    "Lesen ist sicher. Zonenänderungen können beeinflussen, wie Namen im LAN aufgelöst werden.",
  "inspector.domain.dns.zone.example":
    "Zone «home.lan» liefert Einträge für jedes LAN-Gerät und leitet unbekannte Namen an die Upstream-Resolver weiter.",

  "inspector.domain.dhcp.subnet.what":
    "Eine DHCP-Subnetz legt Adresspool und Parameter (Gateway, DNS, Lease-Zeit) fest, die Clients in einem Segment erhalten.",
  "inspector.domain.dhcp.subnet.why":
    "Pro LAN-Segment ein DHCP-Subnetz anlegen. Es definiert den Bereich, aus dem der Server Adressen vergeben darf.",
  "inspector.domain.dhcp.subnet.risk":
    "Falsche Gateway- oder DNS-Werte können Clients vom Internet oder von der lokalen Namensauflösung abschneiden.",
  "inspector.domain.dhcp.subnet.example":
    "Subnetz «lan-home» vergibt 192.168.1.100–192.168.1.250 mit 24-Stunden-Lease; Gateway und DNS sind der Router selbst.",

  "inspector.domain.route.map.what":
    "Eine Route Map gleicht Routing-Attribute ab und schreibt sie um — etwa um die Local Preference eines Präfixes für einen BGP-Peer zu setzen.",
  "inspector.domain.route.map.why":
    "Wird in BGP und Policy Routing verwendet. Erstellen Sie eine Route Map, wenn Sie akzeptierte, announced oder geänderte Routen steuern möchten.",
  "inspector.domain.route.map.risk":
    "Eine Route Map mit leerem «permit» kann ungewollt alle Präfixe durchlassen. Immer ein explizites deny oder match ergänzen.",
  "inspector.domain.route.map.example":
    "Map «set-localpref-200» matcht die Prefix-Liste «customer-routes» und hebt die Local Preference akzeptierter Routen auf 200.",

  "inspector.domain.bgp.peer.what":
    "Ein BGP-Peer ist ein benachbarter Router, mit dem dieser Router Routing-Informationen austauscht.",
  "inspector.domain.bgp.peer.why":
    "Richten Sie einen Peer ein, um Routen von einem Upstream zu lernen oder eigene Präfixe zu announced.",
  "inspector.domain.bgp.peer.risk":
    "Ein falsch konfigurierter Peer kann flapen oder ungewollte Präfixe announced. ASN, Hold Time und Authentifizierung müssen übereinstimmen.",
  "inspector.domain.bgp.peer.example":
    "Peer «upstream-A» verbindet sich zu ASN 64500 über TCP 179 mit BGP-MD5-Authentifizierung und announced 203.0.113.0/24.",

  "inspector.domain.ospf.area.what":
    "Eine OSPF-Area ist eine logische Gruppe von Routern und Links innerhalb eines OSPF-Autonomen Systems.",
  "inspector.domain.ospf.area.why":
    "Verschaffen Sie sich einen Überblick über die Area-Topologie und die beteiligten Router.",
  "inspector.domain.ospf.area.risk":
    "Fehler in der Area-Konfiguration können Intra-Area-Routing unterbrechen. Standardmäßig nur lesend.",
  "inspector.domain.ospf.area.example":
    "Area 0.0.0.0 (Backbone) verbindet diesen Router mit dem übrigen Standortnetz.",

  "inspector.domain.qos.class.what":
    "Eine QoS-Klasse gruppiert Verkehr, der gleich behandelt werden soll — etwa Sprachverkehr mit niedriger Latenz.",
  "inspector.domain.qos.class.why":
    "Pro Verkehrstyp eine Klasse definieren und Queuing, Shaping oder Policing zuweisen.",
  "inspector.domain.qos.class.risk":
    "Änderungen an einer Klasse wirken auf alle zugeordneten Flows. Eine falsche Klasse kann anderen Verkehr aushungern.",
  "inspector.domain.qos.class.example":
    "Klasse «voice» matched UDP 5060 und 10000–20000 und liegt in einer Priority Queue.",

  "inspector.domain.page.network":
    "Alle aktiven Schnittstellen dieses Routers ansehen. Lesezugriff; vollständige Konfiguration in den Unterseiten.",
  "inspector.domain.page.wan":
    "Upstream-Verbindungen zum Provider verwalten. Nur bearbeiten, wenn Sie die neuen Werte verstehen — Änderungen laufen über Safe Apply.",
  "inspector.domain.page.lan":
    "Lokale Netze und verbundene Clients konfigurieren. Änderungen können WLAN-Clients kurz trennen.",
  "inspector.domain.page.routes":
    "Routingtabelle und BGP-Sitzungen einsehen und bearbeiten. Lesezugriff; Route-Map-Änderungen wirken sofort.",
  "inspector.domain.page.firewall":
    "Eigene Firewall-Regeln leben hier. Die Standardrichtlinie greift, bis eine explizite Regel passt.",
  "inspector.domain.page.dns":
    "Sehen Sie, welche Zonen dieser Router bedient und an welche Upstream-Resolver er weiterleitet.",
  "inspector.domain.page.dhcp":
    "Aktive Leases und konfigurierte Subnetze. Ein Lease zu entfernen erzwingt eine Erneuerung beim nächsten Kontakt.",
  "inspector.domain.page.services":
    "Laufzeitstatus jeder Subsystem: lauschende Sockets, Prozesszustand, letzter Fehler. Änderungen laufen über Safe Apply.",

  /* Legacy chrome strings */
  "chrome.filter": "Filter",
  "chrome.typeToFilter": "Tippen, um zu filtern…",
  "chrome.allAvailability": "Alle",
  "chrome.availableOnly": "Verfügbar",
  "chrome.nonAvailable": "Nicht verfügbar",
  "chrome.availability": "Verfügbarkeit",
  "chrome.create": "Anlegen",
  "chrome.edit": "Bearbeiten",
  "chrome.delete": "Löschen",
  "chrome.actions": "Aktionen",
  "chrome.ofTotal": (n: number, total: number) => `${n} von ${total}`,
  "chrome.technicalDetails": "Technische Details",
  "chrome.collectionSnapshot": "Interner Schnappschuss",
  "chrome.emptyKind": "Hier ist noch nichts.",
  "chrome.total": "Gesamt",
  "chrome.active": "Aktiv",
  "chrome.issues": "Probleme",

  "v4.allTechnicalColumns": "Technische Spalten anzeigen",
  "v4.hideTechnicalColumns": "Technische Spalten verbergen",
  "v4.emptyFor": "Keine Einträge für diese Ansicht.",
  "v4.viewFull": "Im Vollmodus öffnen",
  "v4.refresh": "Aktualisieren",
  "v4.copyAction": "Kopieren",
  "v4.copied": "Kopiert",
  "v4.since": "seit",
  "v4.units.bytes": (n: number) => formatBytes(n),
  "v4.units.percent": (n: number) => `${Math.round(n)}%`,
};

const es: V4Bundle = {
  /* Page meta */
  "pageMeta.title": "Página",
  "pageMeta.designOnly": "Superficie de diseño",
  "pageMeta.readOnly": "Solo lectura",
  "pageMeta.configurable": "Configurable",
  "pageMeta.irreversible": "Irreversible",

  "topbar.searchOpen": "Buscar",
  "topbar.openSearchHint": "Buscar páginas y ayuda",
  "topbar.openInspector": "Ayuda de la página",
  "topbar.themeToggle": "Cambiar tema",
  "topbar.localeSelect": "Idioma",
  "topbar.endSession": "Cerrar sesión",
  "topbar.modeEasy": "Fácil",
  "topbar.modeFull": "Completo",

  "shell.skipToMain": "Saltar al contenido",
  "shell.primaryNav": "Navegación principal",

  "rail.network": "Red",
  "rail.routing": "Enrutamiento",
  "rail.security": "Seguridad",
  "rail.services": "Servicios",
  "rail.system": "Sistema",
  "rail.logs": "Registros y diagnóstico",

  "easy.home": "Inicio",
  "easy.internet": "Internet",
  "easy.wifi": "Wi-Fi",
  "easy.devices": "Dispositivos",
  "easy.vpn": "VPN",
  "easy.system": "Sistema",

  "easyHome.title": "¿Todo funciona?",
  "easyHome.subtitle":
    "Una vista de la red. Si algo no va bien, lo verá aquí primero.",
  "easyHome.headlineLabel": "Estado general",
  "easyHome.statusHealthy": "Todo funciona",
  "easyHome.statusAttention": "Algunas cosas requieren atención",
  "easyHome.statusIncomplete": "Comprobación incompleta",
  "easyHome.statusUnavailable": "Estado no disponible",
  "easyHome.statusVerified": "verificado",
  "easyHome.statusUnverified": "verificación pendiente",
  "easyHome.statusInternetObserved": "Internet disponible",
  "easyHome.statusInternetVerified": "Internet disponible",
  "easyHome.statusInternetIncomplete": "Conectividad no verificada",
  "easyHome.statusInternetUnavailable":
    "Comprobación de conectividad no disponible",
  "easyHome.internetCard": "Internet",
  "easyHome.internetSub": "Cómo este dispositivo llega al resto del mundo.",
  "easyHome.internetConfigure": "Configurar Internet",
  "easyHome.wifiCard": "Wi-Fi",
  "easyHome.wifiSub": "Redes inalámbricas que este dispositivo puede alcanzar.",
  "easyHome.wifiConfigure": "Configurar Wi-Fi",
  "easyHome.wifiDetails": "Detalles",
  "easyHome.devicesCard": "Dispositivos",
  "easyHome.devicesSub": "Dispositivos visibles en su red.",
  "easyHome.devicesOpen": "Ver dispositivos",
  "easyHome.vpnCard": "VPN",
  "easyHome.vpnSub": "Servicios WireGuard y Xray de este dispositivo.",
  "easyHome.vpnConfigure": "Abrir VPN",
  "easyHome.systemCard": "Sistema",
  "easyHome.systemSub": "Estado general del dispositivo.",
  "easyHome.systemOpen": "Abrir Doctor",
  "easyHome.aboutTitle": "Acerca de este dispositivo",
  "easyHome.aboutHostname": "Nombre",
  "easyHome.aboutUptime": "Tiempo activo",
  "easyHome.aboutEnvironment": "Entorno",
  "easyHome.aboutOs": "Sistema operativo",
  "easyHome.aboutKernel": "Núcleo",
  "easyHome.aboutArchitecture": "Arquitectura",
  "easyHome.aboutObserved": "Observado",
  "easyHome.details": "Detalles",
  "easyHome.unknownValue": "—",

  /* Easy Care */
  "easyCare.title": "Cuidado y recuperación",
  "easyCare.subtitle":
    "Lo que ve Doctor y los botones que puede pulsar para recuperarse cuando algo va mal.",
  "easyCare.apiReachable": "API disponible",
  "easyCare.apiUnreachable": "API no disponible",
  "easyCare.findingsCount": "hallazgos",
  "easyCare.doctorEmptyTitle": "Sin hallazgos actuales de Doctor",
  "easyCare.doctorEmptyBody":
    "Doctor devolvió un informe vacío. El estado verificado del sistema requiere evidencia adicional que aún no tenemos.",
  "easyCare.doctorEmptyAction": "Abrir Doctor en modo completo",
  "easyCare.doctorUnavailableTitle": "Estado de Doctor no disponible",
  "easyCare.doctorUnavailableBody":
    "No pudimos contactar con Doctor. Esto no es lo mismo que «sin hallazgos»: no podemos confirmar nada hasta que Doctor responda.",
  "easyCare.doctorUnavailableAction": "Reintentar",
  "easyCare.findingOpenAction": "Abrir Doctor",
  "easyCare.doctorLoadingTitle": "Comprobando Doctor…",
  "easyCare.doctorLoadingBody":
    "Doctor aún no ha respondido. Esperamos su primer informe: esto no es lo mismo que «sin hallazgos».",

  "sys.title": "Resumen del sistema",
  "sys.subtitle":
    "¿Qué falla ahora, qué sigue y hay un cambio esperando aplicarse?",
  "sys.headline": "Estado general",
  "sys.headlineDescription":
    "Una respuesta clara sobre el dispositivo, basada en observación y Doctor.",
  "sys.sectionHeadline": "Recursos de un vistazo",
  "sys.sectionHeadlineSub":
    "Señales en vivo del sistema, actualizadas a demanda.",
  "sys.cards": "Tarjetas de recursos",
  "sys.cardCpu": "CPU",
  "sys.cardMemory": "Memoria",
  "sys.cardStorage": "Almacenamiento",
  "sys.cardInterfaces": "Interfaces",
  "sys.cardRoutes": "Rutas",
  "sys.cardUptime": "Tiempo activo",
  "sys.cardDns": "DNS",
  "sys.cardClock": "Reloj",
  "sys.identity": "Identidad",
  "sys.identitySub": "Cómo se presenta este dispositivo.",
  "sys.storage": "Almacenamiento",
  "sys.storageSub": "Sistemas de archivos montados.",
  "sys.cores": "núcleos lógicos",
  "sys.percentUsed": "en uso",
  "sys.coresFormat": (n: number) => `${n} núcleos`,
  "sys.percentFormat": (pct: number) => `${pct}% en uso`,
  "sys.uptimeDaysHours": (d: number, h: number) =>
    `${d}d ${h}h desde el último arranque`,
  "sys.uptimeHours": (h: number) => `${h}h desde el último arranque`,
  "sys.uptimeMinutes": (m: number) =>
    `${m}min desde el último arranque`,
  "sys.recentFindings": "Hallazgos recientes",
  "sys.noCurrentFindings": "No hay detalles de hallazgos disponibles.",
  "sys.dataStale":
    "Comprobación incompleta: la observación puede estar anticuada.",
  "sys.recentIncidents": "Incidentes recientes",
  "sys.noIncidents": "Sin incidentes recientes.",
  "sys.loading": "Cargando…",
  "sys.error": "Algo salió mal.",
  "sys.contractMissing": "Contrato de API no implementado.",

  "pageWan.findUplink": "Buscar un uplink",
  "pageWan.defaultGateway": "Puerta de enlace predeterminada:",
  "pageWan.via": "vía",
  "pageWan.noMatch": "Ningún uplink coincide con esta vista.",
  "pageWan.noUplinks": "No hay uplinks configurados.",
  "pageWan.failoverOrder": "Orden de conmutación",
  "pageWan.colInterface": "Interfaz",
  "pageWan.colUplink": "Uplink",
  "pageWan.colState": "Estado",
  "pageWan.colRole": "Rol",
  "pageWan.colHealth": "Salud",
  "pageWan.colIPv4": "IPv4",
  "pageWan.colIPv6": "IPv6",
  "pageWan.colMTU": "MTU",

  "actionbar.idleAllGood": "Sin cambios pendientes",
  "actionbar.idleQuiet": "En silencio · nada que aplicar",
  "actionbar.idleStale":
    "Comprobación incompleta · la observación puede estar anticuada",
  "actionbar.pending": (n: number) =>
    `${n} cambio${n === 1 ? "" : "s"} esperando aplicarse`,
  "actionbar.apply": "Validar y aplicar",
  "actionbar.discard": "Descartar todo",
  "actionbar.applying": "Aplicando…",
  "actionbar.confirmRequired": (s: number) =>
    `Se requiere confirmación · reversión automática en ${s}s`,
  "actionbar.confirm": "Confirmar",
  "actionbar.rollback": "Revertir",
  "actionbar.failed": "Aplicación falló",
  "actionbar.failedRecovery": "Abrir recuperación",

  "health.healthy": "Saludable",
  "health.needsAttention": "Necesita atención",
  "health.statusIncomplete": "Estado incompleto",
  "health.observationUntrusted": "Estado no verificado",
  "health.unavailable": "No disponible",
  "health.observationFresh": "Observación reciente",
  "health.observationStale": "Observación anticuada",
  "health.observationNone": "Sin observación",

  "search.headingPages": "Páginas",
  "search.headingObjects": "Dispositivos y direcciones",
  "search.headingSections": "Secciones",
  "search.headingActions": "Acciones",
  "search.placeholder": "Buscar páginas y ayuda",
  "search.openHint": "Cerrar · Esc",
  "search.empty": "Sin coincidencias",
  "search.emptyHint":
    "Pruebe con otro nombre, rol o sección.",
  "search.tooShort": "Sigue escribiendo",
  "search.tooShortHint": "Escribe al menos dos caracteres.",
  "search.resultsHint": (n: number) =>
    `${n} resultado${n === 1 ? "" : "s"} · ↵ para abrir · ↑↓ para mover`,

  "empty.wireguardPeers.title": "Aún no hay pares",
  "empty.wireguardPeers.body":
    "Los pares de WireGuard permiten que otros dispositivos se unan a esta red de forma segura. Cree un par para emitir una clave y configuración.",
  "empty.wireguardPeers.action": "Añadir par de WireGuard",
  "empty.firewallRules.title": "Sin reglas personalizadas",
  "empty.firewallRules.body":
    "El tráfico sigue la política por defecto. Añada una regla para permitir, rechazar o bloquear flujos específicos.",
  "empty.firewallRules.action": "Crear regla de cortafuegos",
  "empty.dhcpLeases.title": "Ningún dispositivo ha pedido una dirección todavía",
  "empty.dhcpLeases.body":
    "Ningún cliente ha obtenido dirección en la ventana de observación actual.",
  "empty.dhcpLeases.action": "Ver ámbitos DHCP",
  "empty.xray.title": "Sin perfiles Xray",
  "empty.xray.body":
    "Cree un perfil para emitir acceso de cliente.",
  "empty.xray.action": "Crear perfil Xray",
  "empty.sites.title": "Sin sitios de replicación",
  "empty.sites.body":
    "Los sitios conectan este dispositivo con otras instalaciones de DuckOS para que la configuración y el estado viajen juntos.",
  "empty.sites.action": "Añadir sitio",
  "empty.vpn.title": "No hay VPN configurada",
  "empty.vpn.body":
    "Configure WireGuard o Xray para exponer este dispositivo a una red remota o a clientes remotos.",
  "empty.interfaces.title": "Sin interfaces",
  "empty.interfaces.body":
    "No hay interfaces visibles. Verifique que el kernel reporta al menos un dispositivo.",
  "empty.routes.title": "Sin rutas",
  "empty.routes.body":
    "Las rutas aparecerán aquí en cuanto el kernel las aprenda.",
  "empty.dns.title": "Sin configuración DNS",
  "empty.dns.body":
    "Añada un resolver para que este dispositivo sepa adónde enviar las consultas DNS.",
  "empty.services.title": "Sin servicios",
  "empty.services.body":
    "Sin servicios listados. Aparecerán nuevos servicios tras el primer ciclo de observación.",
  "empty.generic.title": "Aún no hay nada",
  "empty.generic.body":
    "Esta vista está vacía por ahora. Cuando la función relacionada se configure, aparecerán elementos aquí.",

  "inspector.tabs.what": "¿Qué es esto?",
  "inspector.tabs.why": "¿Para qué sirve?",
  "inspector.tabs.risk": "¿Qué cambia si lo edito?",
  "inspector.tabs.example": "Ejemplo",
  "inspector.openCTA": "Abrir ayuda de la página",
  "inspector.technicalDetails": "Detalles técnicos",
  "inspector.noObjectSelected":
    "Seleccione una fila para ver sus detalles.",
  "inspector.kind": "Tipo",
  "inspector.snapshot": "Instantánea",
  "inspector.objectId": "ID del objeto",

  /* Per-domain Inspector copy */
  "inspector.domain.network.interface.what":
    "Las interfaces son las conexiones de red físicas y virtuales de este router: LAN, WAN, puentes, VLAN, túneles.",
  "inspector.domain.network.interface.why":
    "Revise las conexiones activas. Aquí confirma si un puerto está arriba y qué direcciones IPv4/IPv6 tiene asignadas.",
  "inspector.domain.network.interface.risk":
    "Leer siempre es seguro. Editar puede cambiar las interfaces existentes y cortar sesiones activas.",
  "inspector.domain.network.interface.example":
    "eth0 — el puerto WAN — aparece arriba con una dirección IPv4 y una IPv6 asignadas por el proveedor.",

  "inspector.domain.network.address.what":
    "Una dirección IP está ligada a una interfaz concreta. Junto con la longitud de prefijo define la subred en la que participa.",
  "inspector.domain.network.address.why":
    "Compruebe qué interfaz posee una IP, la longitud del prefijo y si la dirección es accesible.",
  "inspector.domain.network.address.risk":
    "Leer es seguro. Editar puede mover direcciones entre interfaces y afectar sesiones activas.",
  "inspector.domain.network.address.example":
    "192.0.2.1/24 en eth0 pertenece al puente LAN; 203.0.113.4/32 en eth1 es la dirección WAN del proveedor.",

  "inspector.domain.network.firewall.rule.what":
    "Las reglas del firewall deciden qué tráfico se permite, rechaza o descarta entre redes. Se evalúan en orden hasta que una coincida.",
  "inspector.domain.network.firewall.rule.why":
    "Añada excepciones a la política por defecto, por ejemplo para exponer un servicio en la LAN o bloquear un destino.",
  "inspector.domain.network.firewall.rule.risk":
    "Reglas mal escritas pueden dejarle fuera del router o exponer servicios. Los cambios pasan por Safe Apply.",
  "inspector.domain.network.firewall.rule.example":
    "Permitir TCP 443 desde la LAN a un host interno para que el panel del hogar sea accesible desde el teléfono por Wi-Fi.",

  "inspector.domain.network.firewall.set.what":
    "Los sets de firewall son grupos reutilizables de direcciones, puertos o interfaces a los que las reglas pueden referirse por nombre.",
  "inspector.domain.network.firewall.set.why":
    "Defina el set una vez y úselo en muchas reglas: la política se mantiene legible y un cambio se aplica en todos lados.",
  "inspector.domain.network.firewall.set.risk":
    "Renombrar o eliminar un set puede romper cualquier regla que lo use.",
  "inspector.domain.network.firewall.set.example":
    "Un set llamado «guest-clients» contiene todas las direcciones del puente de invitados y una sola regla les da solo internet.",

  "inspector.domain.network.lease.what":
    "Una concesión DHCP es la asignación temporal entre una dirección MAC y una IP que el servidor DHCP ha entregado.",
  "inspector.domain.network.lease.why":
    "Vea qué dispositivos obtuvieron dirección, cuándo expira y de qué rango procede.",
  "inspector.domain.network.lease.risk":
    "Leer es seguro. Quitar una concesión obliga al cliente a renovarla en el siguiente contacto.",
  "inspector.domain.network.lease.example":
    "Phone-on-wifi — 192.168.1.42 — concesión válida 12 horas desde el rango del hogar.",

  "inspector.domain.wireguard.server.what":
    "Un servidor WireGuard es la cara pública de la VPN: escucha en un puerto UDP y acepta conexiones de los peers configurados.",
  "inspector.domain.wireguard.server.why":
    "Configure un servidor cuando quiera exponer la LAN, enrutar todo el tráfico de los clientes por este router o enlazar dos sedes.",
  "inspector.domain.wireguard.server.risk":
    "Un puerto o clave incorrectos impedirán que los peers se conecten. Cambiar la clave invalida la configuración de todos los peers.",
  "inspector.domain.wireguard.server.example":
    "Servidor «home-vpn» escucha en UDP 51820 en el túnel 10.66.0.0/24 y envía el tráfico de los peers por la WAN.",

  "inspector.domain.wireguard.peer.what":
    "Un peer WireGuard es un cliente o sede remota que se conecta a un servidor WireGuard de este router.",
  "inspector.domain.wireguard.peer.why":
    "Añada un peer por cada dispositivo o sede en la VPN. Cada peer obtiene su propio par de claves y dirección de túnel.",
  "inspector.domain.wireguard.peer.risk":
    "Eliminar un peer desconecta de inmediato a ese dispositivo de la VPN. La clave pública no se recupera tras el borrado.",
  "inspector.domain.wireguard.peer.example":
    "Peer «alice-laptop» se conecta desde el teléfono en itinerancia y obtiene la dirección de túnel fija 10.66.0.2.",

  "inspector.domain.xray.server.what":
    "Un servidor Xray es un punto de proxy en este router que reenvía el tráfico de los usuarios configurados.",
  "inspector.domain.xray.server.why":
    "Configure un servidor Xray cuando quiera que los usuarios salgan a internet por este router con un protocolo y puerto concretos.",
  "inspector.domain.xray.server.risk":
    "Un transporte o credenciales mal configurados impedirán la conexión de todos los usuarios. La elección del protocolo afecta a la compatibilidad.",
  "inspector.domain.xray.server.example":
    "Servidor «edge-1» acepta VLESS+Reality en TCP 443 y envía el tráfico de los usuarios a la WAN.",

  "inspector.domain.xray.user.what":
    "Un usuario Xray es un cliente con credenciales de un servidor Xray. Cada usuario tiene identidad (UUID), cifrado y límites de tráfico.",
  "inspector.domain.xray.user.why":
    "Cree un usuario por persona o dispositivo. Reciben una cadena de conexión o enlace para su cliente.",
  "inspector.domain.xray.user.risk":
    "Eliminar un usuario revoca su acceso al instante. No se recupera a menos que se hayan exportado sus credenciales.",
  "inspector.domain.xray.user.example":
    "Usuario «family-phone» usa autenticación por UUID, flujo «xtls-rprx-vision» y 100 GiB al mes.",

  "inspector.domain.dns.zone.what":
    "Una zona DNS es un trozo del espacio de nombres sobre el que este router es autoritativo, por ejemplo la zona local del hogar.",
  "inspector.domain.dns.zone.why":
    "Inspeccione qué zonas existen, qué registros contienen y qué servidores las sirven.",
  "inspector.domain.dns.zone.risk":
    "Leer es seguro. Editar zonas puede cambiar cómo se resuelven los nombres en la LAN.",
  "inspector.domain.dns.zone.example":
    "La zona «home.lan» sirve registros para cada dispositivo de la LAN y reenvía los nombres desconocidos a los resolvers externos.",

  "inspector.domain.dhcp.subnet.what":
    "Una subred DHCP define el pool de direcciones y los parámetros (puerta de enlace, DNS, tiempo de concesión) que reciben los clientes de un segmento.",
  "inspector.domain.dhcp.subnet.why":
    "Configure una subred DHCP por cada segmento de LAN. Define el rango desde el que el servidor reparte direcciones.",
  "inspector.domain.dhcp.subnet.risk":
    "Valores de gateway o DNS incorrectos pueden aislar a los clientes de internet o de la resolución de nombres local.",
  "inspector.domain.dhcp.subnet.example":
    "Subred «lan-home» reparte 192.168.1.100–192.168.1.250 con 24 horas de concesión; gateway y DNS son el propio router.",

  "inspector.domain.route.map.what":
    "Un route map compara y reescribe atributos de enrutamiento, por ejemplo la preferencia local de un prefijo anunciado a un peer BGP.",
  "inspector.domain.route.map.why":
    "Se usa en BGP y policy routing. Cree uno cuando necesite controlar qué rutas se aceptan, anuncian o modifican.",
  "inspector.domain.route.map.risk":
    "Un route map con «permit» vacío puede dejar pasar todos los prefijos sin querer. Añada siempre un deny o match explícito.",
  "inspector.domain.route.map.example":
    "Map «set-localpref-200» coincide con la prefix-list «customer-routes» y sube la preferencia local a 200 en las rutas aceptadas.",

  "inspector.domain.bgp.peer.what":
    "Un peer BGP es un router vecino con el que este router intercambia información de enrutamiento.",
  "inspector.domain.bgp.peer.why":
    "Configure un peer para aprender rutas de un proveedor o para anunciar sus propios prefijos.",
  "inspector.domain.bgp.peer.risk":
    "Un peer mal configurado puede flappear o anunciar prefijos no deseados. Las sesiones deben acordar ASN, hold time y autenticación.",
  "inspector.domain.bgp.peer.example":
    "Peer «upstream-A» conecta con ASN 64500 sobre TCP 179 con autenticación BGP MD5 y anuncia 203.0.113.0/24.",

  "inspector.domain.ospf.area.what":
    "Un área OSPF es una agrupación lógica de routers y enlaces dentro de un sistema autónomo OSPF.",
  "inspector.domain.ospf.area.why":
    "Vea la topología de áreas y los routers que participan en cada una.",
  "inspector.domain.ospf.area.risk":
    "Una mala configuración de área puede romper el enrutamiento intra-área. Solo lectura por defecto.",
  "inspector.domain.ospf.area.example":
    "Área 0.0.0.0 (backbone) conecta este router con el resto de la red de la sede.",

  "inspector.domain.qos.class.what":
    "Una clase QoS agrupa el tráfico que debe recibir el mismo tratamiento, por ejemplo el tráfico de voz con baja latencia.",
  "inspector.domain.qos.class.why":
    "Defina una clase por tipo de tráfico y luego asigne colas, shaping o policing.",
  "inspector.domain.qos.class.risk":
    "Editar una clase afecta a todos los flujos que coincidan. Una clase mal configurada puede dejar sin ancho de banda a otros tráficos.",
  "inspector.domain.qos.class.example":
    "La clase «voice» coincide con UDP 5060 y 10000–20000 y va a una cola prioritaria.",

  "inspector.domain.page.network":
    "Vea todas las interfaces activas de este router. Solo lectura; la configuración completa está en las subpáginas.",
  "inspector.domain.page.wan":
    "Gestione las conexiones hacia el proveedor. Edite solo si entiende los nuevos valores: los cambios pasan por Safe Apply.",
  "inspector.domain.page.lan":
    "Configure las redes locales y los clientes conectados. Los cambios pueden desconectar brevemente a los clientes Wi-Fi.",
  "inspector.domain.page.routes":
    "Consulte y edite la tabla de rutas y las sesiones BGP. Solo lectura; las ediciones de route map aplican al instante.",
  "inspector.domain.page.firewall":
    "Aquí viven las reglas personalizadas del firewall. La política por defecto sigue aplicando mientras no haya coincidencia explícita.",
  "inspector.domain.page.dns":
    "Vea las zonas que sirve este router y los resolvers externos a los que reenvía.",
  "inspector.domain.page.dhcp":
    "Vea las concesiones activas y las subredes configuradas. Quitar una concesión obliga al cliente a renovarla en el siguiente contacto.",
  "inspector.domain.page.services":
    "Consulte el estado de cada subsistema: sockets a la escucha, salud del proceso, último error. Los cambios pasan por Safe Apply.",

  /* Legacy chrome strings */
  "chrome.filter": "Filtro",
  "chrome.typeToFilter": "Escriba para filtrar…",
  "chrome.allAvailability": "Todos",
  "chrome.availableOnly": "Disponibles",
  "chrome.nonAvailable": "No disponibles",
  "chrome.availability": "Disponibilidad",
  "chrome.create": "Crear",
  "chrome.edit": "Editar",
  "chrome.delete": "Eliminar",
  "chrome.actions": "Acciones",
  "chrome.ofTotal": (n: number, total: number) => `${n} de ${total}`,
  "chrome.technicalDetails": "Detalles técnicos",
  "chrome.collectionSnapshot": "Instantánea interna",
  "chrome.emptyKind": "Aún no hay nada aquí.",
  "chrome.total": "Total",
  "chrome.active": "Activos",
  "chrome.issues": "Problemas",

  "v4.allTechnicalColumns": "Mostrar columnas técnicas",
  "v4.hideTechnicalColumns": "Ocultar columnas técnicas",
  "v4.emptyFor": "Sin elementos para esta vista.",
  "v4.viewFull": "Ver en modo Completo",
  "v4.refresh": "Actualizar",
  "v4.copyAction": "Copiar",
  "v4.copied": "Copiado",
  "v4.since": "desde",
  "v4.units.bytes": (n: number) => formatBytes(n),
  "v4.units.percent": (n: number) => `${Math.round(n)}%`,
};

const v4: V4Messages = { en, ru, de, es };

/* Verify at module load: every key in `en` must exist in every locale. */
const REQUIRED = Object.keys(en);
for (const loc of ["ru", "de", "es"] as const) {
  const missing = REQUIRED.filter((k) => !(k in v4[loc]));
  if (missing.length) {
    throw new Error(
      `v4-strings: locale "${loc}" is missing keys: ${missing.join(", ")}`,
    );
  }
  for (const k of REQUIRED) {
    const eVal = (en as Record<string, unknown>)[k];
    const rVal = (v4[loc] as Record<string, unknown>)[k];
    if (typeof eVal === "function" && typeof rVal !== "function") {
      throw new Error(`v4-strings: locale "${loc}" key "${k}" must be a function`);
    }
  }
}

export type { V4 };
export const v4Messages = v4;

export function v4t(
  locale: Locale,
  path: keyof V4 | string,
  ...args: unknown[]
): string {
  const bundle = v4[locale];
  const fallback = v4.en;
  const raw =
    (bundle as Record<string, unknown>)[path as string] ??
    (fallback as Record<string, unknown>)[path as string];
  if (typeof raw === "function") {
    return (raw as (...a: unknown[]) => string)(...args);
  }
  if (typeof raw === "string") return raw;
  return String(path);
}

/* — helpers — */
function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n === 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  const fixed = v >= 100 ? 0 : v >= 10 ? 1 : 2;
  return `${v.toFixed(fixed)} ${units[i]}`;
}

function pluralRu(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}
