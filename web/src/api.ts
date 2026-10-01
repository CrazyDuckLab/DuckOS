import type { Overview } from "../../shared/api/types";

export class APIError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "APIError";
  }
}

export async function fetchOverview(
  token: string,
  signal: AbortSignal,
): Promise<Overview> {
  const response = await fetch("/api/v1/overview", {
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    if (response.status === 401 || response.status === 403) {
      throw new APIError(
        response.status,
        "Access was denied. Check your administrator token and reconnect.",
      );
    }
    throw new APIError(
      response.status,
      `The system could not be observed (HTTP ${response.status}). Check the API and Agent services.`,
    );
  }
  const result: Overview = await response.json();
  if (result.schemaVersion !== "1")
    throw new Error(
      "This API schema is not supported by the installed dashboard.",
    );
  return result;
}

// Speed Lab API client. The mirror of internal/speedlab's
// Request / ActiveRef / Summary shapes.
export type SpeedLabKind = "latency" | "http" | "throughput";

export interface SpeedLabRequest {
  id?: string;
  kind: SpeedLabKind;
  target: {
    host?: string;
    port?: number;
    url?: string;
    label?: string;
  };
  duration: number; // milliseconds, since JSON has no Duration
  maxSamples?: number;
  operator?: string;
  reason?: string;
}

export interface SpeedLabActiveRef {
  id: string;
  state: "pending" | "running" | "done" | "failed";
  startedAt?: string;
}

export interface SpeedLabLatencyStats {
  count: number;
  minMicros: number;
  avgMicros: number;
  maxMicros: number;
  p50Micros: number;
  p95Micros: number;
  p99Micros: number;
}

export interface SpeedLabThroughStats {
  bytes: number;
  seconds: number;
  bytesPerSecond: number;
}

export interface SpeedLabSummary {
  id: string;
  kind: SpeedLabKind;
  target: SpeedLabRequest["target"];
  startedAt: string;
  finishedAt?: string;
  samples?: number;
  duration?: number;
  latency?: SpeedLabLatencyStats;
  throughput?: SpeedLabThroughStats;
  status: string;
  error?: string;
}

export async function startSpeedLabProbe(
  token: string,
  body: SpeedLabRequest,
  signal: AbortSignal,
): Promise<SpeedLabActiveRef> {
  const response = await fetch("/api/v1/speedlab/probes", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    throw new APIError(
      response.status,
      `Speed Lab probe start failed (HTTP ${response.status}).`,
    );
  }
  return (await response.json()) as SpeedLabActiveRef;
}

export async function listSpeedLabProbes(
  token: string,
  signal: AbortSignal,
): Promise<SpeedLabActiveRef[]> {
  const response = await fetch("/api/v1/speedlab/probes", {
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    throw new APIError(
      response.status,
      `Speed Lab list failed (HTTP ${response.status}).`,
    );
  }
  return (await response.json()) as SpeedLabActiveRef[];
}

export async function getSpeedLabProbe(
  token: string,
  id: string,
  signal: AbortSignal,
): Promise<{ state: string; summary: SpeedLabSummary }> {
  const response = await fetch(`/api/v1/speedlab/probes/${id}`, {
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    throw new APIError(
      response.status,
      `Speed Lab probe fetch failed (HTTP ${response.status}).`,
    );
  }
  return (await response.json()) as { state: string; summary: SpeedLabSummary };
}

export async function cancelSpeedLabProbe(
  token: string,
  id: string,
  signal: AbortSignal,
): Promise<void> {
  const response = await fetch(`/api/v1/speedlab/probes/${id}`, {
    method: "DELETE",
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok && response.status !== 404) {
    throw new APIError(
      response.status,
      `Speed Lab probe cancel failed (HTTP ${response.status}).`,
    );
  }
}

// UPnP / NAT-PMP / PCP / Wake types — P065 surface.

export type UPnPProtocol = "upnp" | "nat-pmp" | "pcp";

export interface UPnPNetwork {
  networkRef: string;
  protocols: UPnPProtocol[];
  ttlSec: number;
  allowedClients?: string[];
}

export interface UPnPMapping {
  id: string;
  networkRef: string;
  protocol: UPnPProtocol;
  clientIp: string;
  externalPort: number;
  internalPort: number;
  internalHost: string;
  expiresAt: string;
  createdAt: string;
  source: string;
}

export interface UPnPStats {
  networks: number;
  mappings: number;
  observedAt: string;
  stopped: boolean;
}

export interface UPnPState {
  stats: UPnPStats;
  networks: UPnPNetwork[];
  mappings: UPnPMapping[];
}

export interface WakeResult {
  target: string;
  broadcast: string;
  port: number;
  packets: number;
  bytes: number;
  sentAt: string;
  duration: string;
  status: string;
  observation: string;
}

export async function listUPnPState(
  token: string,
  signal: AbortSignal,
): Promise<UPnPState> {
  const response = await fetch("/api/v1/upnp/state", {
    headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    throw new APIError(
      response.status,
      `UPnP state failed (HTTP ${response.status}).`,
    );
  }
  return (await response.json()) as UPnPState;
}

export async function enableUPnPNetwork(
  token: string,
  body: UPnPNetwork,
  iface: string,
  signal: AbortSignal,
): Promise<void> {
  const response = await fetch(
    `/api/v1/upnp/networks/${encodeURIComponent(body.networkRef)}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ network: body, iface }),
      cache: "no-store",
      credentials: "omit",
      signal,
    },
  );
  if (!response.ok) {
    throw new APIError(
      response.status,
      `UPnP enable failed (HTTP ${response.status}).`,
    );
  }
}

export async function disableUPnPNetwork(
  token: string,
  ref: string,
  signal: AbortSignal,
): Promise<void> {
  const response = await fetch(
    `/api/v1/upnp/networks/${encodeURIComponent(ref)}`,
    {
      method: "DELETE",
      headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
      cache: "no-store",
      credentials: "omit",
      signal,
    },
  );
  if (!response.ok) {
    throw new APIError(
      response.status,
      `UPnP disable failed (HTTP ${response.status}).`,
    );
  }
}

export async function revokeUPnPMapping(
  token: string,
  id: string,
  signal: AbortSignal,
): Promise<void> {
  const response = await fetch(
    `/api/v1/upnp/mappings/${encodeURIComponent(id)}`,
    {
      method: "DELETE",
      headers: { Accept: "application/json", Authorization: `Bearer ${token}` },
      cache: "no-store",
      credentials: "omit",
      signal,
    },
  );
  if (!response.ok) {
    throw new APIError(
      response.status,
      `UPnP revoke failed (HTTP ${response.status}).`,
    );
  }
}

export async function sendWake(
  token: string,
  body: {
    target: string;
    broadcast?: string;
    port?: number;
    packets?: number;
    source?: string;
    reason?: string;
  },
  signal: AbortSignal,
): Promise<WakeResult> {
  const response = await fetch("/api/v1/wake/send", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
    cache: "no-store",
    credentials: "omit",
    signal,
  });
  if (!response.ok) {
    throw new APIError(
      response.status,
      `Wake send failed (HTTP ${response.status}).`,
    );
  }
  return (await response.json()) as WakeResult;
}
