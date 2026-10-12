/**
 * DuckOS V1 — WAN area configurators (P017–P021).
 *
 * P017 WAN Connections and P019 Multi-WAN/Failover are wired to
 * /api/v1/multiwan and /api/v1/overview.interfaces. P018, P020, P021
 * render typed stubs that explain exactly what API surface they need.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable } from "../components/forms/Table";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import type { Overview } from "../../../shared/api/types";

interface Uplink {
  name: string;
  type?: string;
  ipv4?: string;
  gateway?: string;
  state?: string;
  role?: string;
  health?: string;
}

function asUplink(obj: Record<string, unknown>): Uplink {
  return {
    name: String(obj.name ?? obj.iface ?? obj.interface ?? "—"),
    type: obj.type ? String(obj.type) : undefined,
    ipv4: obj.ipv4 ? String(obj.ipv4) : undefined,
    gateway: obj.gateway ? String(obj.gateway) : undefined,
    state: obj.state ? String(obj.state) : undefined,
    role: obj.role ? String(obj.role) : "primary",
    health: obj.health ? String(obj.health) : undefined,
  };
}

/* — P017 WAN Connections — */
export function WanConnectionsPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [overview, setOverview] = useState<FetchResult<Overview> | null>(null);
  const [multiwan, setMultiwan] = useState<FetchResult<unknown> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setOverview);
    client
      .fetchMultiwan({ signal: ctl.signal, fallbackToMissing: true })
      .then(setMultiwan);
    return () => ctl.abort();
  }, [client]);

  if (!overview) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  }
  if (!overview.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={overview.reason === "missing" ? "missing-contract" : "error"}
          body={overview.message}
        />
      </PageFrame>
    );
  }

  const uplinks = multiwan?.ok
    ? (
        (multiwan.data as { uplinks?: Array<Record<string, unknown>> })
          .uplinks ?? []
      ).map(asUplink)
    : [];
  const defaultRoute = overview.data.routes.find(
    (r) => r.destination === "default" || r.destination === "0.0.0.0/0",
  );

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          {
            key: "uplinks",
            value: uplinks.length || "—",
            observed: multiwan?.ok ? undefined : "via /overview",
          },
          { key: "default gateway", value: defaultRoute?.gateway ?? "—" },
          { key: "default route device", value: defaultRoute?.device ?? "—" },
        ]}
        observedAt={overview.data.observedAt}
      />
      <p className="v1-mute v1-fs-sm">
        Read-only observation. Edit endpoints for uplink configuration are
        scheduled behind a typed contract — see
        docs/ui/BACKEND_INTEGRATION_GAPS.md.
      </p>
      <DataTable<Uplink>
        rows={uplinks}
        rowKey={(u) => u.name}
        pageSize={20}
        initialSort={{ key: "name", direction: "asc" }}
        emptyState={
          multiwan?.ok
            ? "No uplinks configured."
            : "/api/v1/multiwan is not exposed — falling back to /overview.interfaces."
        }
        columns={[
          {
            key: "name",
            label: "Interface",
            render: (u) => <strong>{u.name}</strong>,
          },
          { key: "type", label: "Type", render: (u) => u.type ?? "—" },
          {
            key: "role",
            label: "Role",
            render: (u) => (
              <span className="v1-pill">{u.role ?? "primary"}</span>
            ),
          },
          {
            key: "ipv4",
            label: "IPv4",
            mono: true,
            render: (u) => u.ipv4 ?? "—",
          },
          {
            key: "gateway",
            label: "Gateway",
            mono: true,
            render: (u) => u.gateway ?? "—",
          },
          {
            key: "state",
            label: "State",
            render: (u) => (
              <span
                className="v1-pill"
                data-state={
                  (u.state ?? "").toLowerCase() === "up"
                    ? "good"
                    : (u.state ?? "").toLowerCase() === "down"
                      ? "bad"
                      : "unknown"
                }
              >
                {u.state ?? "—"}
              </span>
            ),
          },
          {
            key: "health",
            label: "Health",
            render: (u) => u.health ?? "—",
          },
        ]}
      />
    </PageFrame>
  );
}

/* — P019 Multi-WAN / Failover — */
export function MultiwanPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<unknown> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchMultiwan({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );
  }

  const data = result.data as {
    uplinks?: Array<Record<string, unknown>>;
    active?: string;
    mode?: string;
  };
  const uplinks = (data.uplinks ?? []).map(asUplink);

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        title="Failover state"
        rows={[
          { key: "active uplink", value: data.active ?? "—" },
          { key: "mode", value: data.mode ?? "—" },
          { key: "uplinks", value: uplinks.length },
        ]}
      />
      <p className="v1-mute v1-fs-sm">
        The active uplink is per the most recent probe. Edit endpoints for
        failover order and policy rules are part of the design and tracked under
        docs/ui/BACKEND_INTEGRATION_GAPS.md.
      </p>
      <DataTable<Uplink>
        rows={uplinks}
        rowKey={(u) => u.name}
        pageSize={20}
        initialSort={{ key: "name", direction: "asc" }}
        emptyState="No uplinks configured."
        columns={[
          {
            key: "name",
            label: "Interface",
            render: (u) => <strong>{u.name}</strong>,
          },
          { key: "role", label: "Role", render: (u) => u.role ?? "primary" },
          {
            key: "state",
            label: "State",
            render: (u) => (
              <span
                className="v1-pill"
                data-state={
                  u.state?.toLowerCase() === "active"
                    ? "good"
                    : u.state?.toLowerCase() === "failed"
                      ? "bad"
                      : "warn"
                }
              >
                {u.state ?? "unknown"}
              </span>
            ),
          },
          { key: "health", label: "Health", render: (u) => u.health ?? "—" },
        ]}
      />
    </PageFrame>
  );
}

/* — P018/P020/P021 stubs — typed contract-gap pages — */
export const WanSubareaSpecs: Record<
  string,
  {
    columns: ReadonlyArray<{
      key: string;
      label: string;
      description?: string;
    }>;
    endpoint: string;
  }
> = {
  P018: {
    endpoint: "/api/v1/wan/modems",
    columns: [
      { key: "modem", label: "Modem" },
      { key: "signal", label: "Signal" },
      { key: "sim", label: "SIM" },
      { key: "apn", label: "APN" },
    ],
  },
  P020: {
    endpoint: "/api/v1/multiwan/path",
    columns: [
      { key: "target", label: "Target" },
      { key: "latency", label: "Latency" },
      { key: "loss", label: "Loss" },
      { key: "jitter", label: "Jitter" },
      { key: "mos", label: "MOS" },
    ],
  },
  P021: {
    endpoint: "/api/v1/wan/ddns",
    columns: [
      { key: "provider", label: "Provider" },
      { key: "hostname", label: "Hostname" },
      { key: "last_update", label: "Last update" },
      { key: "result", label: "Result" },
    ],
  },
};

export function WanSubareaStub({
  page,
  bundle,
  spec,
}: {
  page: PageMeta;
  bundle: Messages;
  spec: {
    columns: ReadonlyArray<{
      key: string;
      label: string;
      description?: string;
    }>;
    endpoint: string;
  };
}) {
  return (
    <UnsupportedFeature
      page={page}
      bundle={bundle}
      contract={spec.endpoint}
      blocker="This WAN area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}
