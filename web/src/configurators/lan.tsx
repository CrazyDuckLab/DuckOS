/**
 * DuckOS V1 — LAN / VLAN area configurators (P022–P026).
 *
 * The public surface today exposes interfaces via /api/v1/overview but
 * does not yet expose a typed VLAN / IPAM view. We render typed stubs
 * for P023/P026 and a live read-only summary for P024 that derives
 * device-per-VLAN groupings from /overview.interfaces.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable } from "../components/forms/Table";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { FilterBar } from "../components/forms/Table";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import { networkToInspector } from "../components/inspector/selectors";
import { usePrefs } from "../state/store";
import type { Overview } from "../../../shared/api/types";

/* — P022 Networks / LAN / VLAN (live read-only, derived) — */
export function NetworksLivePage({
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
        <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
      </PageFrame>
    );
  }

  const data = result.data;
  const subnets = data.interfaces
    .flatMap((iface) =>
      iface.addresses.map((a) => ({
        iface: iface.name,
        address: a.address,
        prefix: a.prefixLength,
        family: a.family,
        cidr: `${a.address}/${a.prefixLength}`,
        state: iface.state,
        mac: iface.mac,
      })),
    )
    .filter((s) => query === "" || `${s.iface} ${s.cidr} ${s.mac}`.toLowerCase().includes(query.toLowerCase()));

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "interfaces", value: data.interfaces.length },
          { key: "ipv4 subnets", value: subnets.filter((s) => s.family === "ipv4").length },
          { key: "ipv6 subnets", value: subnets.filter((s) => s.family === "ipv6").length },
        ]}
        observedAt={data.observedAt}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Find an interface or subnet"
        right={
          <span className="v1-subtle v1-fs-xs">
            {subnets.length} addresses
          </span>
        }
      />
      <DataTable
        rows={subnets}
        rowKey={(s, i) => `${s.iface}-${s.family}-${s.address}-${i}`}
        pageSize={25}
        initialSort={{ key: "iface", direction: "asc" }}
        emptyState="No subnets observed."
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          const iface = result.data.interfaces.find((i) => i.name === row.iface);
          if (!iface) return;
          dispatch({
            type: "selectObject",
            object: networkToInspector(page.id, iface),
          });
        }}
        columns={[
          { key: "iface", label: "Interface", render: (s) => <strong>{s.iface}</strong> },
          { key: "family", label: "Family", render: (s) => s.family.toUpperCase() },
          { key: "cidr", label: "Subnet", mono: true, render: (s) => s.cidr },
          {
            key: "state",
            label: "Admin state",
            render: (s) => (
              <span className="v1-pill" data-state={s.state.toLowerCase() === "up" ? "good" : "warn"}>
                {s.state}
              </span>
            ),
          },
          { key: "mac", label: "MAC", mono: true, render: (s) => s.mac || "—" },
        ]}
      />
    </PageFrame>
  );
}

/* — P024 Clients / Devices (live read-only, derived) — */
export function ClientsPage({
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
        <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
      </PageFrame>
    );
  }
  // Each interface counts as one observed device until DHCP leases are
  // available; we surface that derivation honestly to the operator.
  const interfaces = result.data.interfaces;
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "interfaces observed", value: interfaces.length },
          { key: "ipv4", value: interfaces.flatMap((i) => i.addresses.filter((a) => a.family === "ipv4")).length },
          { key: "ipv6", value: interfaces.flatMap((i) => i.addresses.filter((a) => a.family === "ipv6")).length },
        ]}
        observedAt={result.data.observedAt}
      />
      <p className="v1-mute v1-fs-sm">
        Per-device lease information comes from /api/v1/dhcp/leases, which
        is not exposed today. Until then, this view shows the interfaces
        the kernel knows about, with their admin state and addresses.
      </p>
      <DataTable
        rows={interfaces}
        rowKey={(i) => `i-${i.index}-${i.name}`}
        pageSize={25}
        initialSort={{ key: "name", direction: "asc" }}
        emptyState="No devices observed."
        columns={[
          { key: "name", label: "Interface", render: (i) => <strong>{i.name}</strong> },
          {
            key: "state",
            label: "Admin state",
            render: (i) => (
              <span className="v1-pill" data-state={i.state.toLowerCase() === "up" ? "good" : "warn"}>
                {i.state}
              </span>
            ),
          },
          { key: "mac", label: "MAC", mono: true, render: (i) => i.mac || "—" },
          {
            key: "ipv4",
            label: "IPv4",
            mono: true,
            render: (i) => i.addresses.find((a) => a.family === "ipv4")?.address ?? "—",
          },
          {
            key: "ipv6",
            label: "IPv6",
            mono: true,
            render: (i) => i.addresses.find((a) => a.family === "ipv6")?.address ?? "—",
          },
        ]}
      />
    </PageFrame>
  );
}

export const LanSubareaSpecs: Record<string, { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string }> = {
  P023: {
    endpoint: "/api/v1/ipam",
    columns: [
      { key: "block", label: "Block" },
      { key: "purpose", label: "Purpose" },
      { key: "utilization", label: "Utilization" },
    ],
  },
  P025: {
    endpoint: "/api/v1/wifi",
    columns: [
      { key: "ssid", label: "SSID" },
      { key: "band", label: "Band" },
      { key: "channel", label: "Channel" },
      { key: "security", label: "Security" },
      { key: "clients", label: "Clients" },
    ],
  },
  P026: {
    endpoint: "/api/v1/lan/ipv6",
    columns: [
      { key: "source", label: "Source" },
      { key: "prefix", label: "Prefix" },
      { key: "lifetime", label: "Lifetime" },
    ],
  },
};

export function LanSubareaStub({
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
      blocker="This LAN/VLAN area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}