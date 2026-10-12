/**
 * DuckOS V1 — per-kind intent builders + form bindings.
 *
 * Maps each `SupportedObjectKind` (and a few legacy per-area kinds)
 * to the typed create/edit form component plus the
 * `ConfigIntent` builder that turns the form's values into the
 * Safe Apply body.
 *
 * The dashboard NEVER invents fields the backend doesn't know.
 * Each binding declares its own typed `V`; the public `KindBinding`
 * interface uses `unknown` so any kind can be stored in the
 * BINDINGS map. Internal narrowing happens at the binding's own
 * type-safe functions.
 *
 * Destructive actions (delete) are also expressed as
 * `ConfigIntent.objects[]` with `desired: null` so the Engine can
 * validate, apply, and roll back through the same transaction
 * flow as create/edit.
 */
import { createElement, type ReactNode } from "react";
import type { ConfigIntent, SupportedObjectKind } from "../api/types";
import type { Messages } from "../i18n/messages";
import {
  BgpPeerForm,
  DhcpScopeForm,
  DnsZoneForm,
  FirewallRuleForm,
  InterfaceEditForm,
  QosClassForm,
  StaticRouteForm,
  VlanCreateForm,
  WireguardPeerCreateForm,
  WireguardServerCreateForm,
  XrayServerCreateForm,
} from "./Forms";

export interface KindFormProps {
  initial?: unknown;
  onChange: (next: unknown) => void;
}

export interface KindBinding {
  /**
   * Title shown in the create dialog header. Returns the localized title
   * for the given bundle so the binding can render in any locale without
   * the consumer having to do string-key lookup.
   */
  createTitle: (bundle: Messages) => string;
  editTitle: (bundle: Messages) => string;
  deleteTitle: (bundle: Messages) => string;
  /** Render the create form. The returned ReactNode must call
   * `onChange(typedValue)` whenever a field changes. */
  CreateForm: (props: { onChange: (next: unknown) => void }) => ReactNode;
  /** Render the edit form, seeded with `initial`. */
  EditForm: (props: KindFormProps) => ReactNode;
  intentForCreate: (value: unknown) => ConfigIntent;
  intentForUpdate: (
    id: string,
    value: unknown,
    revision?: string,
  ) => ConfigIntent;
  intentForDelete: (id: string, revision?: string) => ConfigIntent;
  /** Return true when a typed value is structurally valid for
   * submission. The ConfiguratorShell gates Apply on this. */
  validate: (value: unknown) => boolean;
  help?: ReactNode;
}

/* — Helper to bind a typed form component to a KindBinding — */
function bind<V>(config: {
  createTitle: (bundle: Messages) => string;
  editTitle: (bundle: Messages) => string;
  deleteTitle: (bundle: Messages) => string;
  Form: React.ComponentType<{
    initial: V;
    onChange: (next: V) => void;
  }>;
  intentForCreate: (value: V) => ConfigIntent;
  intentForUpdate: (id: string, value: V, revision?: string) => ConfigIntent;
  intentForDelete: (id: string, revision?: string) => ConfigIntent;
  validate: (value: V) => boolean;
  initial: V;
  help?: ReactNode;
}): KindBinding {
  return {
    createTitle: config.createTitle,
    editTitle: config.editTitle,
    deleteTitle: config.deleteTitle,
    help: config.help,
    CreateForm: ({ onChange }) =>
      createElement(config.Form, {
        initial: config.initial,
        onChange: (next: V) => onChange(next),
      }),
    EditForm: ({ initial, onChange }) => {
      const seed: V = (initial as V | undefined) ?? config.initial;
      return createElement(config.Form, {
        initial: seed,
        onChange: (next: V) => onChange(next),
      });
    },
    intentForCreate: (v) => config.intentForCreate(v as V),
    intentForUpdate: (id, v, rev) => config.intentForUpdate(id, v as V, rev),
    intentForDelete: config.intentForDelete,
    validate: (v) => config.validate(v as V),
  };
}

/* — network.interface — */
const interfaceBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createInterface,
  editTitle: (b) => b.intent.editInterface,
  deleteTitle: (b) => b.intent.deleteInterface,
  Form: InterfaceEditForm,
  initial: {
    name: "",
    adminState: "auto",
    mtu: 1500,
    ipv4: "",
    ipv4Prefix: 24,
    ipv6: "",
    ipv6Prefix: 64,
  },
  intentForCreate: (v) => ({
    objects: [{ kind: "network.interface", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "network.interface", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "network.interface", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.name === "string" &&
    v.name.length > 0 &&
    v.mtu >= 64 &&
    v.mtu <= 9000,
  help: "Interface changes take effect after Safe Apply confirms. The dashboard never restarts interfaces without explicit operator action.",
});

/* — network.firewall.rule — */
const firewallRuleBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createFirewallRule,
  editTitle: (b) => b.intent.editFirewallRule,
  deleteTitle: (b) => b.intent.deleteFirewallRule,
  Form: FirewallRuleForm,
  initial: {
    chain: "input",
    action: "accept",
    protocol: "any",
    srcCidr: "",
    dstCidr: "",
    srcPort: 0,
    dstPort: 0,
    iface: "any",
    enabled: true,
    comment: "",
  },
  intentForCreate: (v) => ({
    objects: [{ kind: "network.firewall.rule", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "network.firewall.rule", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "network.firewall.rule", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.chain === "string" &&
    v.chain.length > 0 &&
    typeof v.action === "string" &&
    v.action.length > 0,
  help: "Firewall rules are evaluated in display order. Editing or deleting a rule is committed through Safe Apply and is reversible within the commit-confirmed window.",
});

/* — wireguard.server — */
const wireguardServerBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createWireguardServer,
  editTitle: (b) => b.intent.editWireguardServer,
  deleteTitle: (b) => b.intent.deleteWireguardServer,
  Form: WireguardServerCreateForm,
  initial: {
    displayName: "",
    interface: "wg0",
    listenPort: 51820,
    addresses: ["10.66.66.1/24"],
    dns: [],
    mtu: 1420,
  },
  intentForCreate: (v) => ({
    objects: [{ kind: "wireguard.server", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "wireguard.server", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "wireguard.server", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.interface === "string" &&
    v.interface.length > 0,
  help: "The server's private key is generated by the Agent during creation. Private keys are never visible to the dashboard; export through VaultExport.",
});

/* — wireguard.peer — */
const wireguardPeerBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createWireguardServer,
  editTitle: (b) => b.intent.editWireguardPeer,
  deleteTitle: (b) => b.intent.deleteWireguardServer,
  Form: WireguardPeerCreateForm,
  initial: {
    serverId: "",
    displayName: "",
    publicKey: "",
    allowedIPs: ["10.66.66.2/32"],
    endpoint: "",
    persistentKeepalive: 25,
    enabled: true,
  },
  intentForCreate: (v) => ({
    objects: [{ kind: "wireguard.peer", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "wireguard.peer", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "wireguard.peer", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.publicKey === "string" &&
    v.publicKey.length > 0 &&
    Array.isArray(v.allowedIPs) &&
    v.allowedIPs.length > 0,
  help: "Public keys are required. The dashboard shows sha256 digests; paste the raw base64 key here only on creation.",
});

/* — xray.server — */
const xrayServerBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createXrayServer,
  editTitle: (b) => b.intent.editXrayServer,
  deleteTitle: (b) => b.intent.deleteXrayServer,
  Form: XrayServerCreateForm,
  initial: { displayName: "", protocol: "vless", listen: "0.0.0.0", port: 443 },
  intentForCreate: (v) => ({ objects: [{ kind: "xray.server", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "xray.server", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "xray.server", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" && v.displayName.length > 0,
});

/* — qos.class — */
const qosClassBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createQosClass,
  editTitle: (b) => b.intent.editQosClass,
  deleteTitle: (b) => b.intent.deleteQosClass,
  Form: QosClassForm,
  initial: { displayName: "", rateKbps: 1000, priority: 4, enabled: true },
  intentForCreate: (v) => ({ objects: [{ kind: "qos.class", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "qos.class", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "qos.class", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.rateKbps === "number" &&
    v.rateKbps > 0,
});

/* — dns.zone — */
const dnsZoneBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createDnsZone,
  editTitle: (b) => b.intent.editDnsZone,
  deleteTitle: (b) => b.intent.deleteDnsZone,
  Form: DnsZoneForm,
  initial: { displayName: "", type: "authoritative", upstream: [] },
  intentForCreate: (v) => ({ objects: [{ kind: "dns.zone", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "dns.zone", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "dns.zone", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" && v.displayName.length > 0,
});

/* — dhcp.subnet — */
const dhcpSubnetBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createDhcpScope,
  editTitle: (b) => b.intent.editDhcpScope,
  deleteTitle: (b) => b.intent.deleteDhcpScope,
  Form: DhcpScopeForm,
  initial: {
    subnet: "",
    rangeStart: "",
    rangeEnd: "",
    gateway: "",
    leaseSeconds: 3600,
  },
  intentForCreate: (v) => ({ objects: [{ kind: "dhcp.subnet", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "dhcp.subnet", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "dhcp.subnet", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.subnet === "string" &&
    v.subnet.length > 0 &&
    typeof v.rangeStart === "string" &&
    v.rangeStart.length > 0 &&
    typeof v.rangeEnd === "string" &&
    v.rangeEnd.length > 0,
});

/* — bgp.peer — */
const bgpPeerBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createBgpPeer,
  editTitle: (b) => b.intent.editBgpPeer,
  deleteTitle: (b) => b.intent.deleteBgpPeer,
  Form: BgpPeerForm,
  initial: {
    displayName: "",
    remoteAddr: "",
    remoteAs: 64512,
    localAs: 64512,
    password: "",
    enabled: true,
  },
  intentForCreate: (v) => ({ objects: [{ kind: "bgp.peer", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "bgp.peer", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "bgp.peer", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.remoteAddr === "string" &&
    v.remoteAddr.length > 0,
  help: "BGP passwords are stored only inside the Engine and are never returned to the dashboard.",
});

/* — route.map — */
const routeMapBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createRouteMap,
  editTitle: (b) => b.intent.editRouteMap,
  deleteTitle: (b) => b.intent.deleteRouteMap,
  Form: StaticRouteForm,
  initial: {
    destination: "",
    gateway: "",
    metric: 100,
    iface: "lan",
    enabled: true,
  },
  intentForCreate: (v) => ({ objects: [{ kind: "route.map", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "route.map", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "route.map", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.destination === "string" &&
    v.destination.length > 0 &&
    typeof v.gateway === "string" &&
    v.gateway.length > 0,
  help: "Route maps apply policy to routes installed by routing protocols. Edit through Safe Apply to keep the change atomic.",
});

/* — VLAN helper (network.interface kind with vlanId flag) — */
const vlanBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createVlan,
  editTitle: (b) => b.intent.editVlan,
  deleteTitle: (b) => b.intent.deleteVlan,
  Form: VlanCreateForm,
  initial: { vlanId: 0, name: "", parent: "lan", ipv4: "", ipv4Prefix: 24 },
  intentForCreate: (v) => ({
    objects: [
      {
        kind: "network.interface",
        desired: { ...(v as object), kind: "vlan" },
      },
    ],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [
      {
        kind: "network.interface",
        id,
        desired: { ...(v as object), kind: "vlan" },
        revision,
      },
    ],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "network.interface", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.vlanId === "number" &&
    v.vlanId >= 1 &&
    v.vlanId <= 4094 &&
    typeof v.name === "string" &&
    v.name.length > 0,
  help: "VLANs are sub-interfaces of a parent. Deleting a VLAN is destructive but is committed atomically with Safe Apply.",
});

/* — Inline forms for kinds without a per-page form module — */
import {
  Checkbox,
  Field,
  Ipv4Input,
  NumberStepper,
  TextInput,
} from "../components/forms/Fields";
import { validateHostname, validateMac } from "../components/forms/validators";

function AddressForm({
  initial,
  onChange,
}: {
  initial?: {
    address: string;
    prefix: number;
    iface: string;
    enabled: boolean;
  };
  onChange: (next: {
    address: string;
    prefix: number;
    iface: string;
    enabled: boolean;
  }) => void;
}) {
  const v = initial ?? { address: "", prefix: 24, iface: "lan", enabled: true };
  return (
    <div className="v1-form-grid">
      <Field label="IPv4 address" required>
        <Ipv4Input
          value={v.address}
          onChange={(address, ok) =>
            onChange({ ...v, address: ok ? address : "" })
          }
        />
      </Field>
      <Field label="Prefix length">
        <NumberStepper
          value={v.prefix}
          onChange={(prefix) => onChange({ ...v, prefix })}
          min={0}
          max={32}
        />
      </Field>
      <Field label="Interface" required>
        <TextInput
          value={v.iface}
          onChange={(iface, ok) => onChange({ ...v, iface: ok ? iface : "" })}
          validator={validateHostname}
          ariaLabel="Interface"
        />
      </Field>
      <Field label="Enabled">
        <Checkbox
          checked={v.enabled}
          onChange={(enabled) => onChange({ ...v, enabled })}
          label={v.enabled ? "Enabled" : "Disabled"}
        />
      </Field>
    </div>
  );
}

function FirewallSetForm({
  initial,
  onChange,
}: {
  initial?: { name: string; enabled: boolean; rules: string[] };
  onChange: (next: { name: string; enabled: boolean; rules: string[] }) => void;
}) {
  const v = initial ?? { name: "", enabled: true, rules: [] };
  return (
    <div className="v1-form-grid">
      <Field label="Set name" required>
        <TextInput
          value={v.name}
          onChange={(name, ok) => onChange({ ...v, name: ok ? name : "" })}
          validator={validateHostname}
          ariaLabel="Set name"
        />
      </Field>
      <Field label="Enabled">
        <Checkbox
          checked={v.enabled}
          onChange={(enabled) => onChange({ ...v, enabled })}
          label={v.enabled ? "Enabled" : "Disabled"}
        />
      </Field>
    </div>
  );
}

function LeaseForm({
  initial,
  onChange,
}: {
  initial?: {
    mac: string;
    ipv4: string;
    hostname: string;
    leaseSeconds: number;
  };
  onChange: (next: {
    mac: string;
    ipv4: string;
    hostname: string;
    leaseSeconds: number;
  }) => void;
}) {
  const v = initial ?? { mac: "", ipv4: "", hostname: "", leaseSeconds: 3600 };
  return (
    <div className="v1-form-grid">
      <Field label="MAC address" required>
        <TextInput
          value={v.mac}
          onChange={(mac, ok) =>
            onChange({ ...v, mac: ok ? mac.toLowerCase() : "" })
          }
          validator={validateMac}
          mono
          placeholder="aa:bb:cc:dd:ee:ff"
          ariaLabel="MAC address"
        />
      </Field>
      <Field label="IPv4 address" required>
        <Ipv4Input
          value={v.ipv4}
          onChange={(ipv4, ok) => onChange({ ...v, ipv4: ok ? ipv4 : "" })}
        />
      </Field>
      <Field label="Hostname">
        <TextInput
          value={v.hostname}
          onChange={(hostname, ok) =>
            onChange({ ...v, hostname: ok ? hostname : "" })
          }
          validator={validateHostname}
          ariaLabel="Hostname"
        />
      </Field>
      <Field label="Lease time (seconds)">
        <NumberStepper
          value={v.leaseSeconds}
          onChange={(leaseSeconds) => onChange({ ...v, leaseSeconds })}
          min={0}
          max={31_536_000}
        />
      </Field>
    </div>
  );
}

function XrayUserForm({
  initial,
  onChange,
}: {
  initial?: {
    displayName: string;
    serverId: string;
    used: number;
    quota?: number;
    enabled: boolean;
  };
  onChange: (next: {
    displayName: string;
    serverId: string;
    used: number;
    quota?: number;
    enabled: boolean;
  }) => void;
}) {
  const v = initial ?? {
    displayName: "",
    serverId: "",
    used: 0,
    enabled: true,
  };
  return (
    <div className="v1-form-grid">
      <Field label="User name" required>
        <TextInput
          value={v.displayName}
          onChange={(displayName, ok) =>
            onChange({ ...v, displayName: ok ? displayName : "" })
          }
          validator={validateHostname}
          ariaLabel="User name"
        />
      </Field>
      <Field label="Server id" required>
        <TextInput
          value={v.serverId}
          onChange={(serverId, ok) =>
            onChange({ ...v, serverId: ok ? serverId : "" })
          }
          validator={validateHostname}
          ariaLabel="Server id"
        />
      </Field>
      <Field label="Used (bytes)">
        <NumberStepper
          value={v.used}
          onChange={(used) => onChange({ ...v, used })}
          min={0}
          max={10_000_000_000_000}
        />
      </Field>
      <Field label="Enabled">
        <Checkbox
          checked={v.enabled}
          onChange={(enabled) => onChange({ ...v, enabled })}
          label={v.enabled ? "Enabled" : "Disabled"}
        />
      </Field>
    </div>
  );
}

function OspfAreaForm({
  initial,
  onChange,
}: {
  initial?: { displayName: string; areaId: string; enabled: boolean };
  onChange: (next: {
    displayName: string;
    areaId: string;
    enabled: boolean;
  }) => void;
}) {
  const v = initial ?? { displayName: "", areaId: "0.0.0.0", enabled: true };
  return (
    <div className="v1-form-grid">
      <Field label="Name" required>
        <TextInput
          value={v.displayName}
          onChange={(displayName, ok) =>
            onChange({ ...v, displayName: ok ? displayName : "" })
          }
          validator={validateHostname}
          ariaLabel="Name"
        />
      </Field>
      <Field label="Area id" required>
        <TextInput
          value={v.areaId}
          onChange={(areaId, ok) =>
            onChange({ ...v, areaId: ok ? areaId : "" })
          }
          mono
          ariaLabel="Area id"
          placeholder="0.0.0.0"
        />
      </Field>
      <Field label="Enabled">
        <Checkbox
          checked={v.enabled}
          onChange={(enabled) => onChange({ ...v, enabled })}
          label={v.enabled ? "Enabled" : "Disabled"}
        />
      </Field>
    </div>
  );
}

function IsisAreaForm({
  initial,
  onChange,
}: {
  initial?: { displayName: string; net: string; enabled: boolean };
  onChange: (next: {
    displayName: string;
    net: string;
    enabled: boolean;
  }) => void;
}) {
  const v = initial ?? {
    displayName: "",
    net: "49.0001.0000.0000.0000.00",
    enabled: true,
  };
  return (
    <div className="v1-form-grid">
      <Field label="Name" required>
        <TextInput
          value={v.displayName}
          onChange={(displayName, ok) =>
            onChange({ ...v, displayName: ok ? displayName : "" })
          }
          validator={validateHostname}
          ariaLabel="Name"
        />
      </Field>
      <Field label="NET" required>
        <TextInput
          value={v.net}
          onChange={(net, ok) => onChange({ ...v, net: ok ? net : "" })}
          mono
          ariaLabel="NET"
          placeholder="49.0001.0000.0000.0000.00"
        />
      </Field>
      <Field label="Enabled">
        <Checkbox
          checked={v.enabled}
          onChange={(enabled) => onChange({ ...v, enabled })}
          label={v.enabled ? "Enabled" : "Disabled"}
        />
      </Field>
    </div>
  );
}

/* — Inline-form-backed bindings — */
const addressBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createAddress,
  editTitle: (b) => b.intent.editAddress,
  deleteTitle: (b) => b.intent.deleteAddress,
  Form: AddressForm,
  initial: { address: "", prefix: 24, iface: "lan", enabled: true },
  intentForCreate: (v) => ({
    objects: [{ kind: "network.address", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "network.address", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "network.address", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.address === "string" &&
    v.address.length > 0 &&
    typeof v.iface === "string" &&
    v.iface.length > 0,
  help: "Addresses are scoped to a single interface. Removing an address is a destructive change that the Engine applies atomically with Safe Apply.",
});

const firewallSetBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createFirewallRule,
  editTitle: (b) => b.intent.editFirewallRule,
  deleteTitle: (b) => b.intent.deleteFirewallRule,
  Form: FirewallSetForm,
  initial: { name: "", enabled: true, rules: [] },
  intentForCreate: (v) => ({
    objects: [{ kind: "network.firewall.set", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "network.firewall.set", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "network.firewall.set", id, desired: null, revision }],
  }),
  validate: (v) => typeof v.name === "string" && v.name.length > 0,
});

const leaseBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createAddress,
  editTitle: (b) => b.intent.editAddress,
  deleteTitle: (b) => b.intent.deleteAddress,
  Form: LeaseForm,
  initial: { mac: "", ipv4: "", hostname: "", leaseSeconds: 3600 },
  intentForCreate: (v) => ({
    objects: [{ kind: "network.lease", desired: v }],
  }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "network.lease", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "network.lease", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.mac === "string" &&
    v.mac.length > 0 &&
    typeof v.ipv4 === "string" &&
    v.ipv4.length > 0,
  help: "Static DHCP reservations pin an IP to a MAC. Releases return the address to the pool.",
});

const xrayUserBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createXrayServer,
  editTitle: (b) => b.intent.editXrayServer,
  deleteTitle: (b) => b.intent.deleteXrayServer,
  Form: XrayUserForm,
  initial: { displayName: "", serverId: "", used: 0, enabled: true },
  intentForCreate: (v) => ({ objects: [{ kind: "xray.user", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "xray.user", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "xray.user", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.serverId === "string" &&
    v.serverId.length > 0,
});

const ospfAreaBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createAddress,
  editTitle: (b) => b.intent.editAddress,
  deleteTitle: (b) => b.intent.deleteAddress,
  Form: OspfAreaForm,
  initial: { displayName: "", areaId: "0.0.0.0", enabled: true },
  intentForCreate: (v) => ({ objects: [{ kind: "ospf.area", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "ospf.area", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "ospf.area", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.areaId === "string" &&
    v.areaId.length > 0,
});

const isisAreaBinding: KindBinding = bind({
  createTitle: (b) => b.intent.createAddress,
  editTitle: (b) => b.intent.editAddress,
  deleteTitle: (b) => b.intent.deleteAddress,
  Form: IsisAreaForm,
  initial: { displayName: "", net: "49.0001.0000.0000.0000.00", enabled: true },
  intentForCreate: (v) => ({ objects: [{ kind: "isis.area", desired: v }] }),
  intentForUpdate: (id, v, revision) => ({
    objects: [{ kind: "isis.area", id, desired: v, revision }],
  }),
  intentForDelete: (id, revision) => ({
    objects: [{ kind: "isis.area", id, desired: null, revision }],
  }),
  validate: (v) =>
    typeof v.displayName === "string" &&
    v.displayName.length > 0 &&
    typeof v.net === "string" &&
    v.net.length > 0,
});

/* — Master registry — */
const BINDINGS: Record<SupportedObjectKind, KindBinding | undefined> = {
  "network.interface": interfaceBinding,
  "network.address": addressBinding,
  "network.route": routeMapBinding,
  "network.firewall.rule": firewallRuleBinding,
  "network.firewall.set": firewallSetBinding,
  "network.lease": leaseBinding,
  "wireguard.server": wireguardServerBinding,
  "wireguard.peer": wireguardPeerBinding,
  "xray.server": xrayServerBinding,
  "xray.user": xrayUserBinding,
  "xray.profile": undefined,
  "xray.subscription": undefined,
  "qos.class": qosClassBinding,
  "dns.zone": dnsZoneBinding,
  "dhcp.subnet": dhcpSubnetBinding,
  "bgp.peer": bgpPeerBinding,
  "ospf.area": ospfAreaBinding,
  "isis.area": isisAreaBinding,
  "route.map": routeMapBinding,
};

export function bindingForKind(
  kind: SupportedObjectKind,
): KindBinding | undefined {
  return BINDINGS[kind];
}

/** VLAN binding is exposed separately because VLAN pages route
 * through `network.interface` as their underlying kind. */
export const VLAN_BINDING: KindBinding = vlanBinding;
