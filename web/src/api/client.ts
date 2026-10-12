/**
 * DuckOS V1 — typed API client.
 *
 * Every endpoint listed in the backend's server.go is wrapped
 * here. Pages call typed functions; we never raw-fetch from a
 * page. If a contract is missing we return `{ ok: false, reason:
 * "missing" }` so pages render an "API contract gap" state
 * instead of fake success.
 *
 * Mutation endpoints ALWAYS go through the Safe Apply workflow
 * (config/diff → config/apply → config/confirm/rollback). The
 * dashboard never invokes a low-level implementation API on a
 * mutable system.
 */
import type { Locale } from "../i18n/messages";
import type {
  Collection,
  ConfigApplyResponse,
  ConfigConfirmResponse,
  ConfigDiffResponse,
  ConfigIntent,
  ConfigRollbackResponse,
  ConfigStatus,
  DoctorFinding,
  SupportedObjectKind,
  TopologyEdge,
  TopologyNode,
  UplinkStatus,
  WireguardPeerView,
  WireguardServerView,
  XrayServerView,
  XrayUserView,
} from "./types";
import type { HealthTone, HealthSummary, TimeTrust } from "../state/store";
import type { Overview } from "../../../shared/api/types";

export class APIError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

export type FetchResult<T> =
  | { ok: true; data: T; age: number }
  | {
      ok: false;
      reason: "missing" | "unauthorized" | "forbidden" | "failed" | "network";
      message: string;
      status?: number;
    };

export interface FetchOptions {
  signal?: AbortSignal;
  method?: "GET" | "POST" | "DELETE";
  body?: unknown;
  fallbackToMissing?: boolean;
  /** Pass revision preconditions for conditional mutation. */
  revision?: string;
}

interface APIClientConfig {
  token: string;
  locale: Locale;
  onFetch?: (path: string, outcome: "ok" | "fail") => void;
}

export class APIClient {
  constructor(private config: APIClientConfig) {}

  setToken(token: string) {
    this.config.token = token;
  }
  setLocale(locale: Locale) {
    this.config.locale = locale;
  }
  token() {
    return this.config.token;
  }

  /**
   * The full set of public endpoints exposed by the V1 API.
   * Pages use the typed wrappers below; this constant is the
   * source of truth for what we know about.
   */
  static readonly ENDPOINTS = {
    authStatus: "/api/v1/auth/status",
    authLogin: "/api/v1/auth/login",
    authLogout: "/api/v1/auth/logout",
    authMe: "/api/v1/auth/me",
    authSessions: "/api/v1/auth/sessions",
    authUsers: "/api/v1/auth/users",
    authAudit: "/api/v1/auth/audit",
    authEnroll: "/api/v1/auth/enroll",

    overview: "/api/v1/overview",
    health: "/api/v1/health",
    ready: "/api/v1/ready",
    readiness: "/api/v1/readiness",
    doctor: "/api/v1/doctor",
    topology: "/api/v1/topology",
    multiwan: "/api/v1/multiwan",
    qos: "/api/v1/qos",
    qosApply: "/api/v1/qos/apply",
    qosRollback: "/api/v1/qos/rollback",
    ups: "/api/v1/ups",
    upsProbe: "/api/v1/ups/probe",
    migrate: "/api/v1/migrate",
    sites: "/api/v1/sites",
    wifi: "/api/v1/wifi",
    upgrade: "/api/v1/upgrade",
    upgradeStage: "/api/v1/upgrade/stage",
    upgradeActivate: "/api/v1/upgrade/activate",
    upgradeRollback: "/api/v1/upgrade/rollback",
    backup: "/api/v1/backup",
    backupRestore: "/api/v1/backup/restore",

    frr: "/api/v1/frr",
    frrApply: "/api/v1/frr/apply",
    frrRollback: "/api/v1/frr/rollback",

    wireguard: "/api/v1/wireguard",
    wireguardPeers: "/api/v1/wireguard/peers",
    xray: "/api/v1/xray",
    xrayUsers: "/api/v1/xray/users",

    networkPreview: "/api/v1/network/preview",
    networkSimulate: "/api/v1/network/simulate",

    objects: "/api/v1/objects",

    config: "/api/v1/config",
    configStatus: "/api/v1/config/status",
    configDiff: "/api/v1/config/diff",
    configApply: "/api/v1/config/apply",
    configConfirm: "/api/v1/config/confirm",
    configRollback: "/api/v1/config/rollback",

    clientsExport: "/api/v1/clients/export",
    ztpIssue: "/api/v1/ztp/issue",
    ztpRedeem: "/api/v1/ztp/redeem",
  } as const;

  /** Generic typed fetch. */
  async fetch<T>(
    path: string,
    expectedSchemaVersion: string | null,
    options: FetchOptions = {},
  ): Promise<FetchResult<T>> {
    const startedAt = Date.now();
    const headers: Record<string, string> = {
      Accept: "application/json",
      Authorization: `Bearer ${this.config.token}`,
      "Accept-Language": this.config.locale,
    };
    let body: BodyInit | undefined;
    if (options.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(options.body);
    }
    if (options.revision) {
      headers["If-Match"] = options.revision;
    }
    try {
      const response = await fetch(path, {
        method: options.method ?? "GET",
        headers,
        cache: "no-store",
        credentials: "omit",
        signal: options.signal ?? null,
        body,
      });
      if (response.status === 401) {
        this.config.onFetch?.(path, "fail");
        return {
          ok: false,
          reason: "unauthorized",
          message:
            "Access was denied. Check your administrator token and reconnect.",
          status: 401,
        };
      }
      if (response.status === 403) {
        this.config.onFetch?.(path, "fail");
        return {
          ok: false,
          reason: "forbidden",
          message: "You do not have permission for this view.",
          status: 403,
        };
      }
      if (response.status === 404 && options.fallbackToMissing) {
        this.config.onFetch?.(path, "fail");
        return {
          ok: false,
          reason: "missing",
          message: `No contract yet for ${path}.`,
          status: 404,
        };
      }
      if (!response.ok) {
        this.config.onFetch?.(path, "fail");
        // Product language first — the technical evidence (HTTP code +
        // path) is preserved in the returned `status` and travels with
        // the FetchResult so callers can surface it under "Technical
        // details" instead of in the headline.
        let message =
          response.status === 400
            ? "DuckOS could not verify the current system state. Try again before applying network changes."
            : response.status === 401
              ? "Your session is no longer valid. Reconnect to continue."
              : response.status === 403
                ? "You don't have permission for this view."
                : response.status === 404
                  ? "This page is not available on this installation."
                  : response.status === 503
                    ? "A DuckOS service is unavailable right now. Try again, or open diagnostics."
                    : "DuckOS couldn't reach the network at the moment. Try again, or open diagnostics.";
        try {
          const text = await response.text();
          if (text.length > 0) {
            const parsed = JSON.parse(text) as {
              code?: string;
              detail?: string;
            };
            // Prefer the backend's detail when it gives actionable text.
            if (
              parsed.detail &&
              parsed.detail.length > 0 &&
              parsed.detail.length < 200
            ) {
              message = parsed.detail;
            }
          }
        } catch {
          // ignore — fall back to generic message
        }
        return {
          ok: false,
          reason: "failed",
          message,
          status: response.status,
        };
      }
      const text = await response.text();
      const parsed: unknown = text.length === 0 ? null : JSON.parse(text);
      if (
        expectedSchemaVersion !== null &&
        typeof parsed === "object" &&
        parsed !== null &&
        (parsed as { schemaVersion?: unknown }).schemaVersion !==
          expectedSchemaVersion
      ) {
        this.config.onFetch?.(path, "fail");
        return {
          ok: false,
          reason: "failed",
          message:
            "This API schema is not supported by the installed dashboard.",
        };
      }
      this.config.onFetch?.(path, "ok");
      return { ok: true, data: parsed as T, age: Date.now() - startedAt };
    } catch (err) {
      this.config.onFetch?.(path, "fail");
      return {
        ok: false,
        reason: "network",
        message:
          err instanceof Error
            ? `The API at ${path} did not respond. (${err.message})`
            : `The API at ${path} did not respond.`,
      };
    }
  }

  /* — Overview / health — */
  fetchOverview(options: FetchOptions = {}) {
    return this.fetch<Overview>(APIClient.ENDPOINTS.overview, "1", options);
  }
  fetchHealth(options: FetchOptions = {}) {
    return this.fetch<HealthPayload>(APIClient.ENDPOINTS.health, null, options);
  }
  fetchReadiness(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.readiness, null, options);
  }
  fetchDoctor(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.doctor, null, options);
  }
  fetchTopology(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.topology, null, options);
  }
  fetchMultiwan(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.multiwan, null, options);
  }
  fetchSites(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.sites, null, options);
  }
  fetchWifi(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.wifi, null, options);
  }
  fetchQos(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.qos, null, options);
  }
  fetchUps(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.ups, null, options);
  }
  fetchBackup(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.backup, null, options);
  }
  fetchUpgrade(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.upgrade, null, options);
  }
  fetchFrr(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.frr, null, options);
  }
  fetchWireguardServers(options: FetchOptions = {}) {
    return this.fetch<WireguardServerView[]>(
      `${APIClient.ENDPOINTS.wireguard}/servers`,
      null,
      options,
    );
  }
  fetchWireguardPeers(options: FetchOptions = {}) {
    return this.fetch<WireguardPeerView[]>(
      `${APIClient.ENDPOINTS.wireguard}/peers`,
      null,
      options,
    );
  }
  fetchXrayServers(options: FetchOptions = {}) {
    return this.fetch<XrayServerView[]>(
      `${APIClient.ENDPOINTS.xray}/servers`,
      null,
      options,
    );
  }
  fetchXrayUsers(options: FetchOptions = {}) {
    return this.fetch<XrayUserView[]>(
      `${APIClient.ENDPOINTS.xray}/users`,
      null,
      options,
    );
  }
  fetchAuthStatus(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.authStatus, null, options);
  }
  fetchAuthMe(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.authMe, null, options);
  }

  /* — Generic objects list — */
  fetchObjects(
    kind: SupportedObjectKind,
    params: { limit?: number; after?: string } = { limit: 100 },
    options: FetchOptions = {},
  ) {
    const qs = new URLSearchParams();
    if (params.limit) qs.set("limit", String(params.limit));
    if (params.after) qs.set("after", params.after);
    const suffix = qs.toString() ? `?${qs.toString()}` : "";
    return this.fetch<Collection>(
      `${APIClient.ENDPOINTS.objects}/${kind}${suffix}`,
      null,
      options,
    );
  }

  /* — Safe Apply / config transaction — */
  fetchConfigStatus(options: FetchOptions = {}) {
    return this.fetch<ConfigStatus>(
      APIClient.ENDPOINTS.configStatus,
      null,
      options,
    );
  }
  fetchConfig(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.config, null, options);
  }
  configDiff(intent: ConfigIntent, options: FetchOptions = {}) {
    return this.fetch<ConfigDiffResponse>(
      APIClient.ENDPOINTS.configDiff,
      null,
      { ...options, method: "POST", body: { desired: intent } },
    );
  }
  configApply(
    diffHash: string,
    intent: ConfigIntent,
    opts?: { confirmWindowSeconds?: number; note?: string },
    options: FetchOptions = {},
  ) {
    return this.fetch<ConfigApplyResponse>(
      APIClient.ENDPOINTS.configApply,
      null,
      {
        ...options,
        method: "POST",
        body: {
          diffHash,
          desired: intent,
          confirmWindowSeconds: opts?.confirmWindowSeconds,
          note: opts?.note,
        },
      },
    );
  }
  configConfirm(transactionId: string, options: FetchOptions = {}) {
    return this.fetch<ConfigConfirmResponse>(
      `${APIClient.ENDPOINTS.configConfirm}/${encodeURIComponent(transactionId)}`,
      null,
      { ...options, method: "POST" },
    );
  }
  configRollback(
    transactionId: string,
    reason?: string,
    options: FetchOptions = {},
  ) {
    return this.fetch<ConfigRollbackResponse>(
      `${APIClient.ENDPOINTS.configRollback}/${encodeURIComponent(transactionId)}`,
      null,
      { ...options, method: "POST", body: reason ? { reason } : {} },
    );
  }

  /* — Network planner — */
  networkPreview(desired: ConfigIntent, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.networkPreview, null, {
      ...options,
      method: "POST",
      body: { desired },
    });
  }
  networkSimulate(desired: ConfigIntent, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.networkSimulate, null, {
      ...options,
      method: "POST",
      body: { desired },
    });
  }

  /* — QoS / FRR apply — */
  qosApply(plan: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.qosApply, null, {
      ...options,
      method: "POST",
      body: { plan },
    });
  }
  qosRollback(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.qosRollback, null, {
      ...options,
      method: "POST",
    });
  }
  frrApply(plan: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.frrApply, null, {
      ...options,
      method: "POST",
      body: { plan },
    });
  }
  frrRollback(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.frrRollback, null, {
      ...options,
      method: "POST",
    });
  }

  /* — Upgrade lifecycle — */
  upgradeStage(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.upgradeStage, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }
  upgradeActivate(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.upgradeActivate, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }
  upgradeRollback(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.upgradeRollback, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }

  /* — Backup restore — */
  backupRestore(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.backupRestore, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }

  /* — UPS probe — */
  upsProbe(options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.upsProbe, null, {
      ...options,
      method: "POST",
    });
  }

  /* — Migration — */
  migrate(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.migrate, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }

  /* — ZTP — */
  ztpIssue(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.ztpIssue, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }
  ztpRedeem(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.ztpRedeem, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }

  /* — WireGuard lifecycle — */
  wireguardServer(serverId: string, options: FetchOptions = {}) {
    return this.fetch<WireguardServerView>(
      `${APIClient.ENDPOINTS.wireguard}/servers/${encodeURIComponent(serverId)}`,
      null,
      options,
    );
  }
  wireguardCreateServer(server: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(
      `${APIClient.ENDPOINTS.wireguard}/servers`,
      null,
      { ...options, method: "POST", body: server },
    );
  }
  wireguardDeleteServer(serverId: string, options: FetchOptions = {}) {
    return this.fetch<unknown>(
      `${APIClient.ENDPOINTS.wireguard}/servers/${encodeURIComponent(serverId)}`,
      null,
      { ...options, method: "DELETE" },
    );
  }
  wireguardPeer(peerId: string, options: FetchOptions = {}) {
    return this.fetch<WireguardPeerView>(
      `${APIClient.ENDPOINTS.wireguard}/peers/${encodeURIComponent(peerId)}`,
      null,
      options,
    );
  }
  wireguardCreatePeer(peer: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(`${APIClient.ENDPOINTS.wireguard}/peers`, null, {
      ...options,
      method: "POST",
      body: peer,
    });
  }
  wireguardDeletePeer(peerId: string, options: FetchOptions = {}) {
    return this.fetch<unknown>(
      `${APIClient.ENDPOINTS.wireguard}/peers/${encodeURIComponent(peerId)}`,
      null,
      { ...options, method: "DELETE" },
    );
  }

  /* — Xray lifecycle — */
  xrayCreateServer(server: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(`${APIClient.ENDPOINTS.xray}/servers`, null, {
      ...options,
      method: "POST",
      body: server,
    });
  }
  xrayDeleteServer(serverId: string, options: FetchOptions = {}) {
    return this.fetch<unknown>(
      `${APIClient.ENDPOINTS.xray}/servers/${encodeURIComponent(serverId)}`,
      null,
      { ...options, method: "DELETE" },
    );
  }
  xrayCreateUser(user: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(`${APIClient.ENDPOINTS.xray}/users`, null, {
      ...options,
      method: "POST",
      body: user,
    });
  }
  xrayDeleteUser(userId: string, options: FetchOptions = {}) {
    return this.fetch<unknown>(
      `${APIClient.ENDPOINTS.xray}/users/${encodeURIComponent(userId)}`,
      null,
      { ...options, method: "DELETE" },
    );
  }

  /* — Clients export (admin-initiated, never in-app store) — */
  clientsExport(payload: unknown, options: FetchOptions = {}) {
    return this.fetch<unknown>(APIClient.ENDPOINTS.clientsExport, null, {
      ...options,
      method: "POST",
      body: payload,
    });
  }
}

/** Health payload — what `/api/v1/health` returns. Used by the System
 * Overview, Easy home, Easy care, and the inspector. Kept in sync with
 * the backend `HealthResponse` JSON shape.
 *
 * Production contract is intentionally minimal:
 *   { "status": "ok", "schemaVersion": "1" }
 *
 * Component-level health, time trust, and findings come from their own
 * dedicated endpoints (`/api/v1/overview`, `/api/v1/doctor`,
 * `/api/v1/topology`). The Easy Care page reads component state from
 * `useDoctor()` and never from this payload. */
export type HealthPayload = {
  status: "ok" | "degraded" | "unavailable" | string;
  schemaVersion: string;
};

export function toneFromResult(
  reason?: "missing" | "unauthorized" | "forbidden" | "failed" | "network",
): HealthTone {
  if (!reason) return "good";
  if (reason === "unauthorized" || reason === "forbidden") return "warn";
  return "bad";
}

export function inferTimeTrust(payload: unknown): TimeTrust {
  if (typeof payload !== "object" || payload === null) return "unknown";
  const p = payload as Record<string, unknown>;
  const t = p.timeTrust ?? p.clockTrusted ?? p.time ?? p.clock;
  if (t === "trusted" || t === true) return "trusted";
  if (t === "untrusted" || t === false) return "untrusted";
  return "unknown";
}

export function countFindings(payload: unknown): number {
  if (typeof payload !== "object" || payload === null) return 0;
  const p = payload as Record<string, unknown>;
  if (Array.isArray(p.findings)) return p.findings.length;
  if (Array.isArray(p.issues)) return p.issues.length;
  return 0;
}

export function countPending(payload: unknown): number {
  if (typeof payload !== "object" || payload === null) return 0;
  const p = payload as Record<string, unknown>;
  if (Array.isArray(p.pending)) return p.pending.length;
  if (Array.isArray(p.candidates)) return p.candidates.length;
  if (Array.isArray(p.changes)) return p.changes.length;
  return 0;
}

export function emptyHealth(): HealthSummary {
  return {
    overview: "unknown",
    readiness: "unknown",
    doctor: "unknown",
    topology: "unknown",
    reachable: false,
    lastObservedAt: null,
    timeTrust: "unknown",
    activeIncidents: 0,
    pendingChanges: 0,
  };
}

/** Internal helpers pages may use. */
export function asUplinks(payload: unknown): UplinkStatus[] {
  if (!payload || typeof payload !== "object") return [];
  const p = payload as {
    uplinks?: Array<Record<string, unknown>>;
    topology?: Array<Record<string, unknown>>;
  };
  const list = p.uplinks ?? p.topology ?? [];
  return list.map((u) => ({
    name: String(u.name ?? u.iface ?? u.interface ?? "—"),
    type: u.type ? String(u.type) : undefined,
    role: u.role ? String(u.role) : undefined,
    state: u.state ? String(u.state) : undefined,
    health: u.health ? String(u.health) : undefined,
    gateway: u.gateway ? String(u.gateway) : undefined,
    ipv4: u.ipv4 ? String(u.ipv4) : undefined,
    ipv6: u.ipv6 ? String(u.ipv6) : undefined,
    mtu: typeof u.mtu === "number" ? u.mtu : undefined,
  }));
}

export function asDoctorFindings(payload: unknown): DoctorFinding[] {
  if (Array.isArray(payload)) return payload as DoctorFinding[];
  if (!payload || typeof payload !== "object") return [];
  const p = payload as { findings?: unknown; issues?: unknown };
  const list = Array.isArray(p.findings)
    ? p.findings
    : Array.isArray(p.issues)
      ? p.issues
      : [];
  return list as DoctorFinding[];
}

export function asTopology(payload: unknown): {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
} {
  if (!payload || typeof payload !== "object") return { nodes: [], edges: [] };
  const p = payload as { nodes?: unknown; edges?: unknown; links?: unknown };
  const nodes = (Array.isArray(p.nodes) ? p.nodes : []) as TopologyNode[];
  const edges = (
    Array.isArray(p.edges) ? p.edges : Array.isArray(p.links) ? p.links : []
  ) as TopologyEdge[];
  return { nodes, edges };
}
