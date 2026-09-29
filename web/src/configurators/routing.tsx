/**
 * DuckOS V1 — Routing area configurators (P027–P040, P067).
 *
 * Live read-only: P027 RIB (via /overview.routes), P067 QoS stub.
 * Typed stubs for the rest until /api/v1/frr surface is published with
 * typed BGP/OSPF/BFD/... payloads.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable, FilterBar, type DataColumn } from "../components/forms/Table";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import { routeToInspector } from "../components/inspector/selectors";
import { usePrefs } from "../state/store";
import type { Overview } from "../../../shared/api/types";

export function RoutingPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<Overview> | null>(null);
  const [query, setQuery] = useState("");
  const [family, setFamily] = useState<"all" | "ipv4" | "ipv6">("all");
  const [protocol, setProtocol] = useState<"all" | string>("all");
  const { state, dispatch } = usePrefs();
  const selectedKey =
    state.inspectorObject && state.inspectorObject.pageId === page.id
      ? state.inspectorObject.ref
      : null;
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );

  const data = result.data;
  const protocols = Array.from(new Set(data.routes.map((r) => r.protocol))).sort();
  const q = query.toLowerCase();
  const filtered = data.routes.filter((r) => {
    if (family !== "all" && r.family !== family) return false;
    if (protocol !== "all" && r.protocol !== protocol) return false;
    if (!query) return true;
    return `${r.destination} ${r.gateway} ${r.device} ${r.table}`.toLowerCase().includes(q);
  });

  const columns: DataColumn<Overview["routes"][number]>[] = [
    { key: "destination", label: "Destination", mono: true, render: (r) => r.destination || "default" },
    { key: "gateway", label: "Gateway", mono: true, render: (r) => r.gateway || "—" },
    {
      key: "device",
      label: "Interface",
      render: (r) => <span className="v1-pill">{r.device || "—"}</span>,
    },
    { key: "family", label: "Family", render: (r) => (r.family === "ipv4" ? "IPv4" : "IPv6") },
    { key: "table", label: "Table", mono: true, render: (r) => r.table || "—" },
    { key: "metric", label: "Metric", mono: true, numeric: true },
    {
      key: "protocol",
      label: "Protocol",
      render: (r) => <span className="v1-pill">{r.protocol || "—"}</span>,
    },
    { key: "scope", label: "Scope", mono: true },
  ];

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        title="RIB summary"
        rows={[
          { key: "routes", value: data.routes.length },
          { key: "tables", value: new Set(data.routes.map((r) => r.table)).size },
          { key: "protocols", value: protocols.length },
        ]}
        observedAt={data.observedAt}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Find destination, gateway, or device"
        right={
          <>
            <select
              className="v1-select"
              value={family}
              onChange={(e) => setFamily(e.target.value as typeof family)}
              aria-label="Family"
            >
              <option value="all">All families</option>
              <option value="ipv4">IPv4</option>
              <option value="ipv6">IPv6</option>
            </select>
            <select
              className="v1-select"
              value={protocol}
              onChange={(e) => setProtocol(e.target.value)}
              aria-label="Protocol"
            >
              <option value="all">All protocols</option>
              {protocols.map((p) => (
                <option key={p} value={p}>{p}</option>
              ))}
            </select>
            <span className="v1-subtle v1-fs-xs">
              {filtered.length} of {data.routes.length}
            </span>
          </>
        }
      />
      <DataTable
        rows={filtered}
        rowKey={(r, i) => `${r.family}-${r.destination}-${r.device}-${r.table}-${i}`}
        pageSize={25}
        initialSort={{ key: "metric", direction: "asc" }}
        emptyState="No routes match this view."
        columns={columns}
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: routeToInspector(page.id, row),
          });
        }}
      />
    </PageFrame>
  );
}

export function BgpPage({
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
    client.fetchFrr({ signal: ctl.signal, fallbackToMissing: true }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );
  return (
    <PageFrame page={page} bundle={bundle}>
      <p className="v1-mute v1-fs-sm">
        /api/v1/frr exposes FRR summary state. A typed BGP-neighbor schema is
        scheduled and tracked in docs/ui/BACKEND_INTEGRATION_GAPS.md. Until
        then we render the live payload and flag it.
      </p>
      <pre className="v1-code">{JSON.stringify(result.data, null, 2)}</pre>
    </PageFrame>
  );
}

export const RoutingSubareaSpecs: Record<string, { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string }> = {
  P028: {
    endpoint: "/api/v1/routing/policy",
    columns: [
      { key: "selector", label: "Selector" },
      { key: "table", label: "Table" },
      { key: "priority", label: "Priority" },
    ],
  },
  P029: {
    endpoint: "/api/v1/routing/vrf",
    columns: [
      { key: "vrf", label: "VRF" },
      { key: "rt_import", label: "RT import" },
      { key: "rt_export", label: "RT export" },
    ],
  },
  P030: {
    endpoint: "/api/v1/frr (typed BGP section)",
    columns: [
      { key: "neighbor", label: "Neighbor" },
      { key: "state", label: "State" },
      { key: "uptime", label: "Uptime" },
      { key: "rx", label: "Rx" },
      { key: "tx", label: "Tx" },
    ],
  },
  P031: {
    endpoint: "/api/v1/frr (typed OSPF section)",
    columns: [
      { key: "area", label: "Area" },
      { key: "neighbor", label: "Neighbor" },
      { key: "state", label: "State" },
      { key: "cost", label: "Cost" },
    ],
  },
  P032: {
    endpoint: "/api/v1/frr (typed IS-IS/RIP section)",
    columns: [
      { key: "proto", label: "Protocol" },
      { key: "neighbor", label: "Neighbor" },
      { key: "state", label: "State" },
    ],
  },
  P033: {
    endpoint: "/api/v1/frr (typed BFD section)",
    columns: [
      { key: "peer", label: "Peer" },
      { key: "state", label: "State" },
      { key: "flap_count", label: "Flap count" },
    ],
  },
  P034: {
    endpoint: "/api/v1/frr (typed filters section)",
    columns: [
      { key: "map", label: "Map" },
      { key: "match", label: "Match" },
      { key: "action", label: "Action" },
    ],
  },
  P035: {
    endpoint: "/api/v1/frr (typed RPKI section)",
    columns: [
      { key: "target", label: "Target" },
      { key: "result", label: "Result" },
    ],
  },
  P036: {
    endpoint: "/api/v1/frr (typed MPLS section)",
    columns: [
      { key: "label", label: "Label" },
      { key: "fec", label: "FEC" },
      { key: "next_hop", label: "Next-hop" },
    ],
  },
  P037: {
    endpoint: "/api/v1/frr (typed EVPN section)",
    columns: [
      { key: "vni", label: "VNI" },
      { key: "type", label: "Type" },
      { key: "originator", label: "Originator" },
    ],
  },
  P038: {
    endpoint: "/api/v1/frr (typed multicast section)",
    columns: [
      { key: "group", label: "Group" },
      { key: "source", label: "Source" },
      { key: "iface", label: "Interface" },
    ],
  },
  P039: {
    endpoint: "/api/v1/network/nat64",
    columns: [
      { key: "type", label: "Type" },
      { key: "pool", label: "Pool" },
      { key: "state", label: "State" },
    ],
  },
  P040: {
    endpoint: "/api/v1/keepalived",
    columns: [
      { key: "group", label: "Group" },
      { key: "state", label: "State" },
      { key: "priority", label: "Priority" },
    ],
  },
  P067: {
    endpoint: "/api/v1/qos",
    columns: [
      { key: "class", label: "Class" },
      { key: "rate", label: "Rate" },
      { key: "priority", label: "Priority" },
    ],
  },
};

export function RoutingSubareaStub({
  page,
  bundle,
  spec,
}: {
  page: PageMeta;
  bundle: Messages;
  spec: { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string };
}) {
  return (
    <UnsupportedFeature
      page={page}
      bundle={bundle}
      contract={spec.endpoint}
      blocker="This routing area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}