/**
 * DuckOS V1 — Inspector object selectors.
 *
 * Each domain row can produce an InspectorObject that drives the
 * contextual Inspector. Keeping the projections here means pages never
 * leak their internal row shape into the Inspector, and the same row
 * data can drive the table highlight via `selectedRowKey`.
 */
import type { Overview } from "../../../../shared/api/types";
import type {
  WireguardPeerView,
  WireguardServerView,
  XrayServerView,
} from "../../api/types";
import type { InspectorObject } from "../../state/store";

/** Interface row (Overview.interfaces). */
export function interfaceToInspector(
  pageId: string,
  iface: Overview["interfaces"][number],
): InspectorObject {
  const state = iface.state?.toLowerCase() ?? "unknown";
  return {
    pageId,
    kind: "interface",
    ref: iface.mac || `${iface.index}:${iface.name}`,
    displayName: iface.name,
    status: state,
    fields: [
      { label: "Index", value: String(iface.index), mono: true },
      { label: "MAC", value: iface.mac || "—", mono: true },
      { label: "MTU", value: String(iface.mtu), mono: true },
      { label: "Admin state", value: iface.state || "—" },
      {
        label: "IPv4",
        value:
          iface.addresses
            .filter((a) => a.family === "ipv4")
            .map((a) => `${a.address}/${a.prefixLength}`)
            .join(", ") || "—",
        mono: true,
      },
      {
        label: "IPv6",
        value:
          iface.addresses
            .filter((a) => a.family === "ipv6")
            .map((a) => `${a.address}/${a.prefixLength}`)
            .join(", ") || "—",
        mono: true,
      },
      {
        label: "Carrier changes",
        value: "—",
        mono: true,
      },
    ],
  };
}

/** Route row (Overview.routes). */
export function routeToInspector(
  pageId: string,
  route: Overview["routes"][number],
): InspectorObject {
  const status = route.scope ?? "—";
  return {
    pageId,
    kind: "route",
    ref: `${route.destination}→${route.gateway ?? "—"}`,
    displayName: route.destination,
    status: status,
    fields: [
      { label: "Destination", value: route.destination, mono: true },
      { label: "Gateway", value: route.gateway ?? "—", mono: true },
      { label: "Interface", value: route.device ?? "—" },
      { label: "Source", value: route.table ?? "—" },
      { label: "Scope", value: route.scope ?? "—" },
      { label: "Metric", value: String(route.metric ?? "—"), mono: true },
      { label: "Protocol", value: route.protocol ?? "—" },
    ],
  };
}

/** WireGuard server row. */
export function wireguardServerToInspector(
  pageId: string,
  s: WireguardServerView,
): InspectorObject {
  return {
    pageId,
    kind: "wireguard-server",
    ref: s.ref.id,
    displayName: s.displayName,
    status: s.availability,
    fields: [
      { label: "Listen port", value: String(s.listenPort), mono: true },
      { label: "Interface", value: s.interface ?? "—" },
      { label: "Addresses", value: s.addresses.join(", ") || "—", mono: true },
      { label: "DNS", value: (s.dns ?? []).join(", ") || "—", mono: true },
      { label: "MTU", value: String(s.mtu ?? "—"), mono: true },
      {
        label: "Public key digest",
        value: s.publicKeyDigest.slice(0, 32) + "…",
        mono: true,
      },
      { label: "Peers", value: String(s.peerCount), mono: true },
      {
        label: "Private key",
        value: s.hasPrivateKey ? "present (VaultExport)" : "absent",
      },
      { label: "Availability", value: s.availability },
    ],
  };
}

/** WireGuard peer row. */
export function wireguardPeerToInspector(
  pageId: string,
  peer: WireguardPeerView,
): InspectorObject {
  return {
    pageId,
    kind: "wireguard-peer",
    ref: peer.ref.id,
    displayName: peer.displayName,
    status: peer.availability,
    fields: [
      {
        label: "Public key digest",
        value: peer.publicKeyDigest.slice(0, 32) + "…",
        mono: true,
      },
      { label: "Endpoint", value: peer.endpoint ?? "—", mono: true },
      {
        label: "Allowed IPs",
        value: peer.allowedIPs.join(", ") || "—",
        mono: true,
      },
      {
        label: "Persistent keepalive",
        value: String(peer.persistentKeepalive ?? "—"),
        mono: true,
      },
      { label: "Server", value: peer.serverRef.id, mono: true },
      {
        label: "Preshared key",
        value: peer.hasPresharedKey ? "present" : "absent",
      },
      { label: "Enabled", value: peer.enabled ? "yes" : "no" },
      { label: "Availability", value: peer.availability },
    ],
  };
}

/** Xray server row. */
export function xrayServerToInspector(
  pageId: string,
  s: XrayServerView,
): InspectorObject {
  return {
    pageId,
    kind: "xray-server",
    ref: s.ref.id,
    displayName: s.displayName,
    status: s.availability,
    fields: [
      { label: "Protocol", value: s.protocol ?? "—" },
      { label: "Listen", value: s.listen ?? "—", mono: true },
      { label: "Port", value: String(s.port ?? "—"), mono: true },
      { label: "Availability", value: s.availability },
    ],
  };
}

/** Firewall rule row (synthesised from typed firewall state). */
export function firewallRuleToInspector(
  pageId: string,
  rule: {
    id: string;
    family: string;
    chain: string;
    action: string;
    src?: string;
    dst?: string;
    proto?: string;
    comment?: string;
    packets?: number;
    bytes?: number;
  },
): InspectorObject {
  return {
    pageId,
    kind: "firewall-rule",
    ref: rule.id,
    displayName: `${rule.chain} → ${rule.action}`,
    status: rule.action,
    fields: [
      { label: "ID", value: rule.id, mono: true },
      { label: "Family", value: rule.family },
      { label: "Chain", value: rule.chain },
      { label: "Action", value: rule.action },
      { label: "Source", value: rule.src ?? "—", mono: true },
      { label: "Destination", value: rule.dst ?? "—", mono: true },
      { label: "Protocol", value: rule.proto ?? "—" },
      { label: "Comment", value: rule.comment ?? "—" },
      { label: "Packets", value: String(rule.packets ?? "—"), mono: true },
      { label: "Bytes", value: String(rule.bytes ?? "—"), mono: true },
    ],
  };
}

/** DNS server row. */
export function dnsServerToInspector(
  pageId: string,
  server: {
    address: string;
    port?: number;
    protocol?: string;
    enabled?: boolean;
  },
): InspectorObject {
  return {
    pageId,
    kind: "dns-server",
    ref: server.address,
    displayName: server.address,
    status: server.enabled ? "live" : "disabled",
    fields: [
      { label: "Address", value: server.address, mono: true },
      { label: "Port", value: String(server.port ?? 53), mono: true },
      { label: "Protocol", value: server.protocol ?? "udp" },
      { label: "Status", value: server.enabled ? "enabled" : "disabled" },
    ],
  };
}

/** Service row (Overview.services / listener). */
export function serviceToInspector(
  pageId: string,
  svc: {
    name: string;
    kind?: string;
    listen?: string;
    port?: number;
    protocol?: string;
    state?: string;
  },
): InspectorObject {
  return {
    pageId,
    kind: "service",
    ref: svc.name,
    displayName: svc.name,
    status: svc.state ?? "unknown",
    fields: [
      { label: "Kind", value: svc.kind ?? "—" },
      { label: "Listen", value: svc.listen ?? "—", mono: true },
      { label: "Port", value: String(svc.port ?? "—"), mono: true },
      { label: "Protocol", value: svc.protocol ?? "—" },
      { label: "State", value: svc.state ?? "—" },
    ],
  };
}

/** DHCP lease row. */
export function leaseToInspector(
  pageId: string,
  lease: {
    ip: string;
    mac: string;
    hostname?: string;
    iface?: string;
    expiresAt?: number | null;
  },
): InspectorObject {
  return {
    pageId,
    kind: "dhcp-lease",
    ref: lease.mac,
    displayName: lease.hostname || lease.ip,
    status: lease.expiresAt ? "live" : "unknown",
    fields: [
      { label: "IP", value: lease.ip, mono: true },
      { label: "MAC", value: lease.mac, mono: true },
      { label: "Hostname", value: lease.hostname ?? "—" },
      { label: "Interface", value: lease.iface ?? "—" },
      {
        label: "Expires",
        value: lease.expiresAt ? new Date(lease.expiresAt).toISOString() : "—",
        mono: true,
      },
    ],
  };
}

/** Networks / LAN row (interfaces projected as networks). */
export function networkToInspector(
  pageId: string,
  iface: Overview["interfaces"][number],
): InspectorObject {
  const ipv4 =
    iface.addresses
      .filter((a) => a.family === "ipv4")
      .map((a) => `${a.address}/${a.prefixLength}`)
      .join(", ") || "—";
  return {
    pageId,
    kind: "network",
    ref: `${iface.index}:${iface.name}`,
    displayName: iface.name,
    status: iface.state?.toLowerCase() ?? "unknown",
    fields: [
      { label: "Subnet", value: ipv4, mono: true },
      { label: "Interface", value: iface.name },
      { label: "State", value: iface.state ?? "—" },
      { label: "MTU", value: String(iface.mtu), mono: true },
      {
        label: "Devices",
        value: "—",
      },
    ],
  };
}
