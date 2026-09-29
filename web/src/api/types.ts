/**
 * DuckOS V1 — public-API contract types.
 *
 * These types mirror the shapes the backend actually returns.
 * Each type matches the canonical envelope defined in
 * internal/object/object.go and internal/api/*.go. Where the
 * backend uses `any` (Desired / Observed), the type system
 * uses `unknown` so the dashboard does not lie about what it
 * parsed; UI code is responsible for narrowing before
 * rendering.
 *
 * Crucially: this file deliberately only describes what the
 * production surface emits. Where a documented verb is absent
 * (e.g. an extra sort key), the type does not include it.
 */
export interface ObjectRef {
  kind: string;
  id: string;
  revision?: string;
}

export type Availability =
  | "available"
  | "experimental"
  | "hardware-pending"
  | "credential-pending"
  | "unsupported"
  | "unknown";

export interface PermissionSet {
  observe: boolean;
  configure: boolean;
  action: boolean;
  exportSecret: boolean;
  delete?: boolean;
}

export interface OwnerRef {
  kind?: string;
  id?: string;
  name?: string;
}

export interface CollectionIssue {
  source: string;
  reasonId: string;
  detail?: string;
}

export interface ViewObject {
  ref: ObjectRef;
  displayName: string;
  desired: unknown;
  observed?: unknown;
  availability: Availability;
  owner?: OwnerRef;
  supportedActions: string[];
  permissions: PermissionSet;
  tags?: string[];
  updatedAt?: string;
}

export interface Collection {
  snapshotId: string;
  cursor?: string;
  total?: number;
  complete: boolean;
  issues?: CollectionIssue[];
  items: ViewObject[];
}

export const SUPPORTED_OBJECT_KINDS = [
  "network.interface",
  "network.route",
  "network.firewall.rule",
  "network.firewall.set",
  "network.lease",
  "network.address",
  "wireguard.server",
  "wireguard.peer",
  "xray.server",
  "xray.user",
  "xray.profile",
  "xray.subscription",
  "qos.class",
  "dns.zone",
  "dhcp.subnet",
  "bgp.peer",
  "ospf.area",
  "isis.area",
  "route.map",
] as const;

export type SupportedObjectKind = (typeof SUPPORTED_OBJECT_KINDS)[number];

/* — Config transaction (Safe Apply) — */

export interface ConfigStatus {
  /** Last committed revision id, or empty when none. */
  revision: string;
  /** Number of pending changes. */
  pendingCount: number;
  /** Pending transaction id, if any. */
  pendingTransactionId?: string;
  /** Pending deadline in ms-since-epoch (commit-confirmed window). */
  pendingDeadline?: number;
  /** Authoritative per-kind availability summary. */
  availability?: Record<string, Availability>;
}

export interface ConfigDiffRequest {
  /** The new desired configuration. Server validates it. */
  desired: ConfigIntent;
}

export interface ConfigIntent {
  objects?: Array<{
    kind: SupportedObjectKind;
    id?: string;
    desired: unknown;
    /** Optional revision precondition (CAS). */
    revision?: string;
  }>;
}

export interface ConfigDiffResponse {
  /** Human-readable summary of every change. */
  changes: Array<{
    kind: string;
    id: string;
    before: unknown;
    after: unknown;
    risk: "R" | "C" | "I";
  }>;
  /** Validation issues that would block apply. */
  issues: Array<{ kind: string; id: string; reason: string }>;
  /** Estimated blast-radius summary by section. */
  blastRadius: Array<{ kind: string; count: number }>;
}

export interface ConfigApplyRequest {
  /** Diff hash to ensure the apply matches the diff the operator reviewed. */
  diffHash: string;
  /** Optional explicit commit-confirmed window in seconds. */
  confirmWindowSeconds?: number;
  /** Optional operator note (audit trail). */
  note?: string;
}

export interface ConfigApplyResponse {
  transactionId: string;
  /** Epoch ms-since-epoch at which confirm must complete. */
  deadline: number;
  /** Optional human-readable summary. */
  summary?: string;
}

export interface ConfigConfirmRequest {
  transactionId: string;
}

export interface ConfigConfirmResponse {
  transactionId: string;
  /** The revision the system is now at. */
  revision: string;
}

export interface ConfigRollbackRequest {
  transactionId: string;
  reason?: string;
}

export interface ConfigRollbackResponse {
  transactionId: string;
  /** The revision the system is back at. */
  revision: string;
}

/* — WireGuard Server / Peer views (public projection) — */

export interface WireguardServerView {
  ref: ObjectRef;
  displayName: string;
  interface: string;
  listenPort: number;
  addresses: string[];
  dns?: string[];
  mtu?: number;
  /** sha256 hex of the public key, never the raw bytes. */
  publicKeyDigest: string;
  /** True iff the server has a stored private key. */
  hasPrivateKey: boolean;
  peerCount: number;
  availability: Availability;
  supportedActions: string[];
  permissions: PermissionSet;
  tags?: string[];
  updatedAt?: string;
}

export interface WireguardPeerView {
  ref: ObjectRef;
  serverRef: ObjectRef;
  displayName: string;
  /** sha256 hex digest of the public key. */
  publicKeyDigest: string;
  hasPresharedKey: boolean;
  allowedIPs: string[];
  endpoint?: string;
  persistentKeepalive?: number;
  expiresAt?: string;
  intent: string;
  enabled: boolean;
  availability: Availability;
  supportedActions: string[];
  permissions: PermissionSet;
  tags?: string[];
  updatedAt?: string;
}

/* — Xray views — */

export interface XrayServerView {
  ref: ObjectRef;
  displayName: string;
  protocol: string;
  listen: string;
  port: number;
  userCount: number;
  availability: Availability;
  supportedActions: string[];
  permissions: PermissionSet;
}

export interface XrayUserView {
  ref: ObjectRef;
  serverRef: ObjectRef;
  displayName: string;
  used: string;
  quota?: number;
  availability: Availability;
  supportedActions: string[];
  permissions: PermissionSet;
}

/* — Doctor — */

export interface DoctorFinding {
  component: string;
  severity: "info" | "warn" | "warning" | "error" | "critical" | "fatal" | string;
  code: string;
  message?: string;
  description?: string;
  evidence?: unknown;
}

/* — Topology — */

export interface TopologyNode {
  id: string;
  kind: string;
  label: string;
}

export interface TopologyEdge {
  from: string;
  to: string;
  kind: string;
}

/* — Multi-WAN — */

export interface UplinkStatus {
  name: string;
  type?: string;
  role?: "primary" | "secondary" | "fallback" | string;
  state?: string;
  health?: "healthy" | "degraded" | "failed" | "unknown" | string;
  gateway?: string;
  ipv4?: string;
  ipv6?: string;
  mtu?: number;
}