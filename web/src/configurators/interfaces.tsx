/**
 * DuckOS V1 — Interfaces area configurators (P007–P016).
 *
 * Per the spec the backend exposes the kernel interfaces via
 * /api/v1/overview. Edit/create APIs are not part of the public
 * surface today, so we render rich read-only views with full
 * columns, filters, and state markers. P008–P016 each get their own
 * data shape via state.* namespaces inside the same Overview payload
 * once the agent exposes them; until then they render a typed
 * "contract gap" stub that explains exactly which API call would be
 * needed.
 */
import { useEffect, useMemo, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame, Toolbar, Segmented, SearchField } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable, type DataColumn } from "../components/forms/Table";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import { interfaceToInspector } from "../components/inspector/selectors";
import { usePrefs } from "../state/store";
import type { Overview } from "../../../shared/api/types";

export function InterfacesLivePage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<Overview> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setResult);
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
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "interfaces", value: result.data.interfaces.length },
          {
            key: "administratively up",
            value: result.data.interfaces.filter((i) => i.state.toLowerCase() === "up").length,
          },
          { key: "ipv4 addresses", value: result.data.interfaces.flatMap((i) => i.addresses.filter((a) => a.family === "ipv4")).length },
          { key: "ipv6 addresses", value: result.data.interfaces.flatMap((i) => i.addresses.filter((a) => a.family === "ipv6")).length },
        ]}
        observedAt={result.data.observedAt}
      />
      <InterfacesTable bundle={bundle} interfaces={result.data.interfaces} pageId={page.id} />
    </PageFrame>
  );
}

export function InterfacesTable({
  bundle,
  interfaces,
  pageSize,
  pageId,
}: {
  bundle: Messages;
  interfaces: Overview["interfaces"];
  pageSize?: number;
  pageId?: string;
}) {
  const [query, setQuery] = useState("");
  const [family, setFamily] = useState<"all" | "ipv4" | "ipv6">("all");
  const [stateFilter, setStateFilter] = useState<"all" | "up" | "down" | "unknown">("all");
  const { state, dispatch } = usePrefs();
  const selectedKey =
    state.inspectorObject && pageId && state.inspectorObject.pageId === pageId
      ? state.inspectorObject.ref
      : null;

  const filtered = useMemo(() => {
    const q = query.toLowerCase();
    return interfaces.filter((iface) => {
      if (stateFilter !== "all" && iface.state.toLowerCase() !== stateFilter)
        return false;
      if (family !== "all" && !iface.addresses.some((a) => a.family === family))
        return false;
      if (!q) return true;
      return (
        iface.name.toLowerCase().includes(q) ||
        iface.mac.toLowerCase().includes(q) ||
        iface.addresses.some((a) => a.address.toLowerCase().includes(q))
      );
    });
  }, [interfaces, query, family, stateFilter]);

  const columns: DataColumn<Overview["interfaces"][number]>[] = [
    {
      key: "name",
      label: "Name",
      render: (i) => <strong>{i.name}</strong>,
      width: "120px",
    },
    {
      key: "state",
      label: "Admin state",
      render: (i) => (
        <span
          className="v1-pill"
          data-state={
            i.state.toLowerCase() === "up"
              ? "good"
              : i.state.toLowerCase() === "down"
                ? "bad"
                : "unknown"
          }
        >
          {i.state}
        </span>
      ),
      width: "100px",
    },
    { key: "mac", label: "MAC", mono: true, sortValue: (i) => i.mac, width: "160px" },
    { key: "mtu", label: "MTU", numeric: true, mono: true, width: "70px" },
    {
      key: "ipv4",
      label: "IPv4",
      mono: true,
      sortValue: (i) => i.addresses.find((a) => a.family === "ipv4")?.address ?? "",
      render: (i) =>
        i.addresses
          .filter((a) => a.family === "ipv4")
          .map((a) => `${a.address}/${a.prefixLength}`)
          .join(", ") || "—",
    },
    {
      key: "ipv6",
      label: "IPv6",
      mono: true,
      sortValue: (i) => i.addresses.find((a) => a.family === "ipv6")?.address ?? "",
      render: (i) =>
        i.addresses
          .filter((a) => a.family === "ipv6")
          .map((a) => `${a.address}/${a.prefixLength}`)
          .join(", ") || "—",
    },
    { key: "index", label: "Index", mono: true, numeric: true, width: "70px" },
  ];

  return (
    <>
      <Toolbar
        left={
          <SearchField
            value={query}
            onChange={setQuery}
            placeholder="Find an interface or address"
          />
        }
        right={
          <>
            <Segmented
              label="Family"
              value={family}
              onChange={setFamily}
              options={[
                { value: "all", label: "All" },
                { value: "ipv4", label: "IPv4" },
                { value: "ipv6", label: "IPv6" },
              ]}
            />
            <Segmented
              label="State"
              value={stateFilter}
              onChange={setStateFilter}
              options={[
                { value: "all", label: "All" },
                { value: "up", label: "Up" },
                { value: "down", label: "Down" },
                { value: "unknown", label: "Unknown" },
              ]}
            />
          </>
        }
      />
      <DataTable
        rows={filtered}
        columns={columns}
        rowKey={(r) => `${r.index}-${r.name}`}
        pageSize={pageSize ?? 25}
        initialSort={{ key: "name", direction: "asc" }}
        emptyState="No interfaces match this view."
        bundle={bundle}
        selectedRowKey={selectedKey}
        onRowClick={
          pageId
            ? (row) => {
                dispatch({
                  type: "selectObject",
                  object: interfaceToInspector(pageId, row),
                });
              }
            : undefined
        }
      />
    </>
  );
}

/* — Generic "design + contract gap" page for P008–P016.
 * Renders the catalog columns the page would have, plus an explicit
 * "API contract not implemented yet" notice. Pages can be promoted
 * to live read-only by adding a typed fetcher. */
export function InterfaceSubareaPage({
  page,
  bundle,
}: {
  page: PageMeta;
  bundle: Messages;
  columns: ReadonlyArray<{ key: string; label: string; description?: string }>;
}) {
  return (
    <UnsupportedFeature
      page={page}
      bundle={bundle}
      contract={`/api/v1/network/${page.id.toLowerCase()}`}
      blocker="This interfaces subarea is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}

export const InterfacesSubareaSpecs: Record<string, ReadonlyArray<{ key: string; label: string; description?: string }>> = {
  P008: [
    { key: "name", label: "Bridge" },
    { key: "members", label: "Member ports", description: "ports in the bridge" },
    { key: "fdb", label: "FDB size", description: "learned MAC count" },
    { key: "vlan_filter", label: "VLAN filter" },
  ],
  P009: [
    { key: "name", label: "Bond" },
    { key: "mode", label: "Mode" },
    { key: "members", label: "Members" },
    { key: "lacp_state", label: "LACP state" },
  ],
  P010: [
    { key: "port", label: "Port" },
    { key: "state", label: "State" },
    { key: "reason", label: "Reason" },
  ],
  P011: [
    { key: "local_port", label: "Local port" },
    { key: "chassis", label: "Chassis id" },
    { key: "remote_port", label: "Remote port" },
    { key: "ttl", label: "TTL" },
  ],
  P012: [
    { key: "port", label: "Port" },
    { key: "feature", label: "Feature" },
    { key: "mode", label: "Mode" },
    { key: "violations", label: "Violations" },
  ],
  P013: [
    { key: "session", label: "Session" },
    { key: "source", label: "Source" },
    { key: "destination", label: "Destination" },
    { key: "direction", label: "Direction" },
  ],
  P014: [
    { key: "interface", label: "Interface" },
    { key: "cipher", label: "Cipher" },
    { key: "key_id", label: "Key id" },
    { key: "replay", label: "Replay window" },
  ],
  P015: [
    { key: "interface", label: "Interface" },
    { key: "offloads", label: "Offloads" },
    { key: "vfs", label: "SR-IOV VFs" },
    { key: "queues", label: "Queues" },
  ],
  P016: [
    { key: "interface", label: "Interface" },
    { key: "oam_state", label: "OAM state" },
    { key: "remote_status", label: "Remote status" },
  ],
};