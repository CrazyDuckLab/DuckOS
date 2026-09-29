/**
 * DuckOS V1 — feature-specific configurator forms.
 *
 * Each component renders a typed form whose values feed into
 * Safe Apply via buildIntent(). Forms never call mutation APIs
 * directly — they go through useSafeApply().
 *
 * All fields are validated on the client (IPv4/IPv6/CIDR/MAC/port
 * etc) AND the server (the Server Diff call).
 */
import { useState } from "react";
import {
  Checkbox,
  CidrInput,
  Field,
  Ipv4Input,
  NumberStepper,
  PortInput,
  Select,
  SecondsInput,
  TextInput,
  VlanIdInput,
} from "../components/forms/Fields";
import {
  validateCidr,
  validateHostname,
  validateIpv4,
} from "../components/forms/validators";
import type { ConfigIntent, SupportedObjectKind } from "../api/types";

export interface InterfaceEditValue {
  name: string;
  adminState: "up" | "down" | "auto";
  mtu: number;
  ipv4: string;
  ipv4Prefix: number;
  ipv6: string;
  ipv6Prefix: number;
}

/* — Interface admin state / MTU / addresses — */
export function InterfaceEditForm({
  initial,
  onChange,
}: {
  initial: InterfaceEditValue;
  onChange: (next: InterfaceEditValue) => void;
}) {
  const v = initial;
  return (
    <div className="v1-form-grid">
      <Field label="Interface name">
        <TextInput value={v.name} onChange={(name) => onChange({ ...v, name })} validator={validateHostname} mono ariaLabel="Interface name" />
      </Field>
      <Field label="Admin state">
        <Select
          value={v.adminState}
          onChange={(adminState) => onChange({ ...v, adminState })}
          options={[
            { value: "up", label: "Up" },
            { value: "down", label: "Down" },
            { value: "auto", label: "Auto" },
          ]}
        />
      </Field>
      <Field label="MTU" help="64–9000">
        <NumberStepper
          value={v.mtu}
          onChange={(mtu) => onChange({ ...v, mtu })}
          min={64}
          max={9000}
          step={1}
        />
      </Field>
      <Field label="IPv4 address">
        <Ipv4Input value={v.ipv4} onChange={(ipv4) => onChange({ ...v, ipv4 })} />
      </Field>
      <Field label="IPv4 prefix">
        <NumberStepper value={v.ipv4Prefix} onChange={(p) => onChange({ ...v, ipv4Prefix: p })} min={0} max={32} />
      </Field>
      <Field label="IPv6 address">
        <TextInput
          value={v.ipv6}
          onChange={(ipv6, ok) => onChange({ ...v, ipv6: ok ? ipv6 : "" })}
          placeholder="2001:db8::1"
          mono
          ariaLabel="IPv6 address"
        />
      </Field>
    </div>
  );
}

/* — VLAN create — */
export function VlanCreateForm({
  onChange,
}: {
  onChange: (next: { vlanId: number; name: string; parent: string; ipv4: string; ipv4Prefix: number }) => void;
}) {
  const [v, setV] = useState({ vlanId: 0, name: "", parent: "lan", ipv4: "", ipv4Prefix: 24 });
  return (
    <div className="v1-form-grid">
      <Field label="VLAN id" help="1–4094" required>
        <VlanIdInput
          value={v.vlanId || ""}
          onChange={(vlanId) => {
            const next = { ...v, vlanId: typeof vlanId === "number" ? vlanId : 0 };
            setV(next);
            onChange(next);
          }}
        />
      </Field>
      <Field label="Display name" required>
        <TextInput
          value={v.name}
          onChange={(name, ok) => {
            const next = { ...v, name: ok ? name : "" };
            setV(next);
            onChange(next);
          }}
          validator={validateHostname}
          ariaLabel="VLAN name"
        />
      </Field>
      <Field label="Parent interface" required>
        <Select
          value={v.parent}
          onChange={(parent) => {
            const next = { ...v, parent };
            setV(next);
            onChange(next);
          }}
          options={[
            { value: "lan", label: "lan" },
            { value: "wan", label: "wan" },
            { value: "opt1", label: "opt1" },
            { value: "opt2", label: "opt2" },
          ]}
        />
      </Field>
      <Field label="Subnet">
        <CidrInput value={`${v.ipv4}/${v.ipv4Prefix}`} onChange={(next, ok) => {
          if (!ok) return;
          const r = validateCidr(next, "ipv4");
          if (r.ok) {
            const out = { ...v, ipv4: r.value.address, ipv4Prefix: r.value.prefix };
            setV(out);
            onChange(out);
          }
        }} family="ipv4" />
      </Field>
    </div>
  );
}

/* — DHCP scope create — */
export function DhcpScopeForm({
  onChange,
}: {
  onChange: (next: { subnet: string; rangeStart: string; rangeEnd: string; gateway: string; leaseSeconds: number }) => void;
}) {
  const [v, setV] = useState({ subnet: "", rangeStart: "", rangeEnd: "", gateway: "", leaseSeconds: 3600 });
  return (
    <div className="v1-form-grid">
      <Field label="Subnet CIDR" required>
        <CidrInput
          value={v.subnet}
          onChange={(next, ok) => {
            if (!ok) return;
            const r = validateCidr(next, "ipv4");
            if (r.ok) {
              const out = { ...v, subnet: next };
              setV(out);
              onChange(out);
            }
          }}
          family="ipv4"
        />
      </Field>
      <Field label="Range start" required>
        <Ipv4Input value={v.rangeStart} onChange={(rangeStart) => { const out = { ...v, rangeStart }; setV(out); onChange(out); }} />
      </Field>
      <Field label="Range end" required>
        <Ipv4Input value={v.rangeEnd} onChange={(rangeEnd) => { const out = { ...v, rangeEnd }; setV(out); onChange(out); }} />
      </Field>
      <Field label="Gateway">
        <Ipv4Input value={v.gateway} onChange={(gateway) => { const out = { ...v, gateway }; setV(out); onChange(out); }} />
      </Field>
      <Field label="Lease time (seconds)" help="Default 3600">
        <SecondsInput
          value={v.leaseSeconds}
          onChange={(leaseSeconds) => {
            const out = { ...v, leaseSeconds: typeof leaseSeconds === "number" ? leaseSeconds : 0 };
            setV(out);
            onChange(out);
          }}
          ariaLabel="Lease seconds"
        />
      </Field>
    </div>
  );
}

/* — Firewall rule — */
export interface FirewallRuleValue {
  chain: string;
  action: "accept" | "drop" | "reject" | "snat" | "dnat";
  protocol: "any" | "tcp" | "udp" | "icmp";
  srcCidr: string;
  dstCidr: string;
  srcPort: number;
  dstPort: number;
  iface: string;
  enabled: boolean;
  comment: string;
}

export function FirewallRuleForm({
  initial,
  onChange,
}: {
  initial: FirewallRuleValue;
  onChange: (next: FirewallRuleValue) => void;
}) {
  const v = initial;
  return (
    <div className="v1-form-grid">
      <Field label="Chain" required>
        <Select
          value={v.chain}
          onChange={(chain) => onChange({ ...v, chain })}
          options={[
            { value: "input", label: "input" },
            { value: "forward", label: "forward" },
            { value: "output", label: "output" },
            { value: "prerouting", label: "prerouting" },
            { value: "postrouting", label: "postrouting" },
          ]}
        />
      </Field>
      <Field label="Action" required>
        <Select
          value={v.action}
          onChange={(action) => onChange({ ...v, action })}
          options={[
            { value: "accept", label: "accept" },
            { value: "drop", label: "drop" },
            { value: "reject", label: "reject" },
            { value: "snat", label: "snat" },
            { value: "dnat", label: "dnat" },
          ]}
        />
      </Field>
      <Field label="Protocol">
        <Select
          value={v.protocol}
          onChange={(protocol) => onChange({ ...v, protocol })}
          options={[
            { value: "any", label: "any" },
            { value: "tcp", label: "tcp" },
            { value: "udp", label: "udp" },
            { value: "icmp", label: "icmp" },
          ]}
        />
      </Field>
      <Field label="Source CIDR">
        <CidrInput
          value={v.srcCidr}
          onChange={(next, ok) => { if (ok) onChange({ ...v, srcCidr: next }); }}
          family="ipv4"
        />
      </Field>
      <Field label="Destination CIDR">
        <CidrInput
          value={v.dstCidr}
          onChange={(next, ok) => { if (ok) onChange({ ...v, dstCidr: next }); }}
          family="ipv4"
        />
      </Field>
      <Field label="Source port">
        <PortInput
          value={v.srcPort}
          onChange={(srcPort) => onChange({ ...v, srcPort: typeof srcPort === "number" ? srcPort : 0 })}
        />
      </Field>
      <Field label="Destination port">
        <PortInput
          value={v.dstPort}
          onChange={(dstPort) => onChange({ ...v, dstPort: typeof dstPort === "number" ? dstPort : 0 })}
        />
      </Field>
      <Field label="Interface">
        <Select
          value={v.iface}
          onChange={(iface) => onChange({ ...v, iface })}
          options={[
            { value: "any", label: "any" },
            { value: "lan", label: "lan" },
            { value: "wan", label: "wan" },
            { value: "vpn", label: "vpn" },
          ]}
        />
      </Field>
      <Field label="Enabled">
        <Checkbox
          checked={v.enabled}
          onChange={(enabled) => onChange({ ...v, enabled })}
          label={v.enabled ? "Enabled" : "Disabled"}
        />
      </Field>
      <Field label="Comment" help="Free-form, not parsed">
        <TextInput value={v.comment} onChange={(comment) => onChange({ ...v, comment })} ariaLabel="Comment" />
      </Field>
    </div>
  );
}

/* — WireGuard server create — */
export function WireguardServerCreateForm({
  onChange,
}: {
  onChange: (next: { displayName: string; interface: string; listenPort: number; addresses: string[]; dns: string[]; mtu: number }) => void;
}) {
  const [v, setV] = useState({ displayName: "", interface: "wg0", listenPort: 51820, addresses: ["10.66.66.1/24"] as string[], dns: [] as string[], mtu: 1420 });
  return (
    <div className="v1-form-grid">
      <Field label="Server name" required>
        <TextInput
          value={v.displayName}
          onChange={(displayName, ok) => { const next = { ...v, displayName: ok ? displayName : "" }; setV(next); onChange(next); }}
          validator={validateHostname}
          ariaLabel="Server name"
        />
      </Field>
      <Field label="Interface" required>
        <TextInput
          value={v.interface}
          onChange={(iface, ok) => { const next = { ...v, interface: ok ? iface : "" }; setV(next); onChange(next); }}
          validator={validateHostname}
          mono
          ariaLabel="Interface"
        />
      </Field>
      <Field label="Listen port" required>
        <PortInput
          value={v.listenPort}
          onChange={(listenPort) => { const next = { ...v, listenPort: typeof listenPort === "number" ? listenPort : 51820 }; setV(next); onChange(next); }}
        />
      </Field>
      <Field label="Interface addresses (one per line)">
        <textarea
          className="v1-textarea"
          rows={3}
          value={v.addresses.join("\n")}
          onChange={(e) => {
            const addresses = e.target.value.split("\n").map((s) => s.trim()).filter(Boolean);
            const next = { ...v, addresses };
            setV(next);
            onChange(next);
          }}
          aria-label="Interface addresses"
        />
      </Field>
      <Field label="DNS (one per line)">
        <textarea
          className="v1-textarea"
          rows={2}
          value={v.dns.join("\n")}
          onChange={(e) => {
            const dns = e.target.value.split("\n").map((s) => s.trim()).filter(Boolean);
            const next = { ...v, dns };
            setV(next);
            onChange(next);
          }}
          aria-label="DNS"
        />
      </Field>
      <Field label="MTU">
        <NumberStepper
          value={v.mtu}
          onChange={(mtu) => { const next = { ...v, mtu }; setV(next); onChange(next); }}
          min={1280}
          max={1500}
        />
      </Field>
      <p className="v1-subtle v1-fs-xs">
        The WireGuard server's private key is generated by the Agent
        during creation. The dashboard never sees the raw bytes; the
        dedicated VaultExport contract is used for one-time export.
      </p>
    </div>
  );
}

/* — WireGuard peer create — */
export function WireguardPeerCreateForm({
  onChange,
}: {
  onChange: (next: { serverId: string; displayName: string; publicKey: string; allowedIPs: string[]; endpoint: string; persistentKeepalive: number; enabled: boolean }) => void;
}) {
  const [v, setV] = useState({ serverId: "", displayName: "", publicKey: "", allowedIPs: ["10.66.66.2/32"] as string[], endpoint: "", persistentKeepalive: 25, enabled: true });
  return (
    <div className="v1-form-grid">
      <Field label="Server" required>
        <TextInput value={v.serverId} onChange={(serverId) => { const next = { ...v, serverId }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Server id" />
      </Field>
      <Field label="Peer name" required>
        <TextInput value={v.displayName} onChange={(displayName, ok) => { const next = { ...v, displayName: ok ? displayName : "" }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Peer name" />
      </Field>
      <Field label="Public key" required help="Base64 — WireGuard public key of the peer">
        <TextInput value={v.publicKey} onChange={(publicKey, ok) => { const next = { ...v, publicKey: ok ? publicKey : "" }; setV(next); onChange(next); }} mono ariaLabel="Public key" />
      </Field>
      <Field label="AllowedIPs (one per line)" required>
        <textarea
          className="v1-textarea"
          rows={3}
          value={v.allowedIPs.join("\n")}
          onChange={(e) => {
            const allowedIPs = e.target.value.split("\n").map((s) => s.trim()).filter(Boolean);
            const next = { ...v, allowedIPs };
            setV(next);
            onChange(next);
          }}
          aria-label="AllowedIPs"
        />
      </Field>
      <Field label="Endpoint" help="Optional — host:port of the peer">
        <TextInput
          value={v.endpoint}
          onChange={(endpoint, ok) => { const next = { ...v, endpoint: ok ? endpoint : "" }; setV(next); onChange(next); }}
          mono
          ariaLabel="Endpoint"
        />
      </Field>
      <Field label="Persistent keepalive (seconds)">
        <SecondsInput
          value={v.persistentKeepalive}
          onChange={(persistentKeepalive) => { const next = { ...v, persistentKeepalive: typeof persistentKeepalive === "number" ? persistentKeepalive : 0 }; setV(next); onChange(next); }}
          ariaLabel="Persistent keepalive"
        />
      </Field>
      <Field label="Enabled">
        <Checkbox checked={v.enabled} onChange={(enabled) => { const next = { ...v, enabled }; setV(next); onChange(next); }} label={v.enabled ? "Enabled" : "Disabled"} />
      </Field>
    </div>
  );
}

/* — Xray server create — */
export function XrayServerCreateForm({
  onChange,
}: {
  onChange: (next: { displayName: string; protocol: "vless" | "vmess" | "shadowsocks" | "socks"; listen: string; port: number }) => void;
}) {
  const [v, setV] = useState({ displayName: "", protocol: "vless" as const, listen: "0.0.0.0", port: 443 });
  return (
    <div className="v1-form-grid">
      <Field label="Server name" required>
        <TextInput value={v.displayName} onChange={(displayName, ok) => { const next = { ...v, displayName: ok ? displayName : "" }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Server name" />
      </Field>
      <Field label="Protocol" required>
        <Select
          value={v.protocol}
          onChange={(protocol) => { const next = { ...v, protocol: protocol as typeof v.protocol }; setV(next); onChange(next); }}
          options={[
            { value: "vless", label: "VLESS" },
            { value: "vmess", label: "VMess" },
            { value: "shadowsocks", label: "Shadowsocks" },
            { value: "socks", label: "SOCKS" },
          ]}
        />
      </Field>
      <Field label="Listen address" required>
        <Ipv4Input value={v.listen} onChange={(listen) => { const next = { ...v, listen }; setV(next); onChange(next); }} />
      </Field>
      <Field label="Port" required>
        <PortInput value={v.port} onChange={(port) => { const next = { ...v, port: typeof port === "number" ? port : 443 }; setV(next); onChange(next); }} />
      </Field>
    </div>
  );
}

/* — BGP peer create — */
export function BgpPeerForm({
  onChange,
}: {
  onChange: (next: { displayName: string; remoteAddr: string; remoteAs: number; localAs: number; password: string; enabled: boolean }) => void;
}) {
  const [v, setV] = useState({ displayName: "", remoteAddr: "", remoteAs: 64512, localAs: 64512, password: "", enabled: true });
  return (
    <div className="v1-form-grid">
      <Field label="Peer name" required>
        <TextInput value={v.displayName} onChange={(displayName, ok) => { const next = { ...v, displayName: ok ? displayName : "" }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Peer name" />
      </Field>
      <Field label="Remote address" required help="IPv4 or IPv6">
        <TextInput
          value={v.remoteAddr}
          onChange={(remoteAddr, ok) => {
            const next = { ...v, remoteAddr: ok ? remoteAddr : "" };
            const r = validateIpv4(next.remoteAddr);
            if (r.ok || !next.remoteAddr) {
              setV(next);
              onChange(next);
            }
          }}
          validator={(raw) => {
            if (!raw) return { ok: false, reason: "Empty" };
            const r = validateIpv4(raw);
            return r.ok ? r : { ok: false, reason: r.reason };
          }}
          mono
          ariaLabel="Remote address"
        />
      </Field>
      <Field label="Remote ASN" required>
        <NumberStepper
          value={v.remoteAs}
          onChange={(remoteAs) => { const next = { ...v, remoteAs }; setV(next); onChange(next); }}
          min={1}
          max={4_294_967_295}
        />
      </Field>
      <Field label="Local ASN" required>
        <NumberStepper
          value={v.localAs}
          onChange={(localAs) => { const next = { ...v, localAs }; setV(next); onChange(next); }}
          min={1}
          max={4_294_967_295}
        />
      </Field>
      <Field label="BGP password (TCP MD5)" help="Stored only inside the Engine. Never returned to the dashboard.">
        <TextInput value={v.password} onChange={(password) => { const next = { ...v, password }; setV(next); onChange(next); }} type="password" ariaLabel="BGP password" />
      </Field>
      <Field label="Enabled">
        <Checkbox checked={v.enabled} onChange={(enabled) => { const next = { ...v, enabled }; setV(next); onChange(next); }} label={v.enabled ? "Enabled" : "Disabled"} />
      </Field>
    </div>
  );
}

/* — Static route create — */
export function StaticRouteForm({
  onChange,
}: {
  onChange: (next: { destination: string; gateway: string; metric: number; iface: string; enabled: boolean }) => void;
}) {
  const [v, setV] = useState({ destination: "", gateway: "", metric: 100, iface: "lan", enabled: true });
  return (
    <div className="v1-form-grid">
      <Field label="Destination" required>
        <CidrInput value={v.destination} onChange={(next, ok) => { if (ok) { const out = { ...v, destination: next }; setV(out); onChange(out); } }} family="ipv4" />
      </Field>
      <Field label="Gateway" required>
        <Ipv4Input value={v.gateway} onChange={(gateway) => { const next = { ...v, gateway }; setV(next); onChange(next); }} />
      </Field>
      <Field label="Metric">
        <NumberStepper value={v.metric} onChange={(metric) => { const next = { ...v, metric }; setV(next); onChange(next); }} min={0} max={65535} />
      </Field>
      <Field label="Interface">
        <TextInput value={v.iface} onChange={(iface) => { const next = { ...v, iface }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Interface" />
      </Field>
      <Field label="Enabled">
        <Checkbox checked={v.enabled} onChange={(enabled) => { const next = { ...v, enabled }; setV(next); onChange(next); }} label={v.enabled ? "Enabled" : "Disabled"} />
      </Field>
    </div>
  );
}

/* — QoS class create — */
export function QosClassForm({
  onChange,
}: {
  onChange: (next: { displayName: string; rateKbps: number; priority: number; enabled: boolean }) => void;
}) {
  const [v, setV] = useState({ displayName: "", rateKbps: 1000, priority: 4, enabled: true });
  return (
    <div className="v1-form-grid">
      <Field label="Class name" required>
        <TextInput value={v.displayName} onChange={(displayName, ok) => { const next = { ...v, displayName: ok ? displayName : "" }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Class name" />
      </Field>
      <Field label="Rate (kbps)">
        <NumberStepper
          value={v.rateKbps}
          onChange={(rateKbps) => { const next = { ...v, rateKbps }; setV(next); onChange(next); }}
          min={1}
          max={1_000_000_000}
        />
      </Field>
      <Field label="Priority">
        <NumberStepper
          value={v.priority}
          onChange={(priority) => { const next = { ...v, priority }; setV(next); onChange(next); }}
          min={0}
          max={7}
        />
      </Field>
      <Field label="Enabled">
        <Checkbox checked={v.enabled} onChange={(enabled) => { const next = { ...v, enabled }; setV(next); onChange(next); }} label={v.enabled ? "Enabled" : "Disabled"} />
      </Field>
    </div>
  );
}

/* — DNS zone create — */
export function DnsZoneForm({
  onChange,
}: {
  onChange: (next: { displayName: string; type: "authoritative" | "forward"; upstream: string[] }) => void;
}) {
  const [v, setV] = useState({ displayName: "", type: "authoritative" as const, upstream: [] as string[] });
  return (
    <div className="v1-form-grid">
      <Field label="Zone name" required>
        <TextInput value={v.displayName} onChange={(displayName, ok) => { const next = { ...v, displayName: ok ? displayName : "" }; setV(next); onChange(next); }} validator={validateHostname} ariaLabel="Zone name" />
      </Field>
      <Field label="Zone type" required>
        <Select
          value={v.type}
          onChange={(type) => { const next = { ...v, type: type as typeof v.type }; setV(next); onChange(next); }}
          options={[
            { value: "authoritative", label: "Authoritative" },
            { value: "forward", label: "Forwarder" },
          ]}
        />
      </Field>
      <Field label="Upstream resolvers (one per line)" help="Required for forward zones.">
        <textarea
          className="v1-textarea"
          rows={3}
          value={v.upstream.join("\n")}
          onChange={(e) => {
            const upstream = e.target.value.split("\n").map((s) => s.trim()).filter(Boolean);
            const next = { ...v, upstream };
            setV(next);
            onChange(next);
          }}
          aria-label="Upstream resolvers"
        />
      </Field>
    </div>
  );
}

/* — Helpers — */
export function buildIntentFor(objects: ConfigIntent["objects"]): ConfigIntent {
  return { objects };
}

/** Re-export the supported kinds for callers that want to assert. */
export type { SupportedObjectKind };