/**
 * DuckOS V1 — DNS / DHCP area configurators (P053–P057).
 *
 * P053 DNS Resolver is wired to /overview.dns. P057 DHCP Leases is
 * read-only via /overview.interfaces until /api/v1/dhcp/leases lands.
 * P054–P056 are typed stubs.
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
import {
  dnsServerToInspector,
  leaseToInspector,
} from "../components/inspector/selectors";
import { usePrefs } from "../state/store";
import type { Overview } from "../../../shared/api/types";

export function DnsResolverPage({
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

  if (!result)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  if (!result.ok)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );

  const dns = result.data.dns;
  const filtered = dns.nameservers.filter(
    (ns) => !query || ns.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "nameservers", value: dns.nameservers.length },
          { key: "search domains", value: dns.searchDomains.length },
          { key: "source", value: dns.source || "—" },
        ]}
        observedAt={result.data.observedAt}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Filter nameservers"
        right={
          <span className="v1-subtle v1-fs-xs">
            {filtered.length} of {dns.nameservers.length}
          </span>
        }
      />
      <DataTable
        rows={filtered.map((ns) => ({ ns, source: dns.source }))}
        rowKey={(r) => r.ns}
        pageSize={20}
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: dnsServerToInspector(page.id, {
              address: row.ns,
              port: 53,
              protocol: "udp",
              enabled: true,
            }),
          });
        }}
        columns={[
          {
            key: "ns",
            label: "Nameserver",
            mono: true,
            render: (r) => <strong>{r.ns}</strong>,
          },
          { key: "source", label: "Source" },
        ]}
        emptyState="No nameservers match."
      />
      <div className="v1-card">
        <div className="card-head">
          <h3>Search domains</h3>
          <span className="meta">{dns.searchDomains.length}</span>
        </div>
        {dns.searchDomains.length === 0 ? (
          <p className="v1-mute">No search domains configured.</p>
        ) : (
          <ul
            className="v1-row"
            style={{
              gap: "var(--v1-s2)",
              flexWrap: "wrap",
              listStyle: "none",
              padding: 0,
            }}
          >
            {dns.searchDomains.map((d) => (
              <li key={d} className="v1-pill">
                {d}
              </li>
            ))}
          </ul>
        )}
      </div>
      <p className="v1-subtle v1-fs-xs">
        Reachability is not tested. The /api/v1/health endpoint is a local
        signal, not Internet reachability.
      </p>
    </PageFrame>
  );
}

export function DhcpLeasesPage({
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
  if (!result)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  if (!result.ok)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );

  // We do not yet have a typed DHCP leases feed. Until /api/v1/dhcp/leases
  // is exposed, we present a derived view: each interface with an IPv4
  // address is reported as "1 observed lease". This is honest because we
  // say so explicitly.
  const observed = result.data.interfaces
    .filter((i) => i.addresses.some((a) => a.family === "ipv4"))
    .map((i) => ({
      iface: i.name,
      mac: i.mac,
      address: i.addresses.find((a) => a.family === "ipv4")?.address ?? "—",
      state: i.state,
    }))
    .filter(
      (r) =>
        !query ||
        `${r.iface} ${r.mac} ${r.address}`
          .toLowerCase()
          .includes(query.toLowerCase()),
    );

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "observed leases (derived)", value: observed.length },
          { key: "leases source", value: "derived from /overview.interfaces" },
        ]}
        observedAt={result.data.observedAt}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Filter derived leases"
        right={
          <span className="v1-subtle v1-fs-xs">{observed.length} rows</span>
        }
      />
      <DataTable
        rows={observed}
        rowKey={(r) => `${r.iface}-${r.mac}`}
        pageSize={25}
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: leaseToInspector(page.id, {
              ip: row.address,
              mac: row.mac,
              hostname: row.iface,
              iface: row.iface,
              expiresAt: null,
            }),
          });
        }}
        columns={[
          {
            key: "iface",
            label: "Interface",
            render: (r) => <strong>{r.iface}</strong>,
          },
          { key: "mac", label: "MAC", mono: true, render: (r) => r.mac || "—" },
          {
            key: "address",
            label: "Address",
            mono: true,
            render: (r) => r.address,
          },
          {
            key: "state",
            label: "Admin state",
            render: (r) => (
              <span
                className="v1-pill"
                data-state={r.state.toLowerCase() === "up" ? "good" : "warn"}
              >
                {r.state}
              </span>
            ),
          },
        ]}
        emptyState="No derived leases."
      />
      <p className="v1-subtle v1-fs-xs">
        A typed leases endpoint would expose MAC, hostname, lease age, and
        reservation status. Until then this view derives the same shape from
        /overview.interfaces and flags its derivation.
      </p>
    </PageFrame>
  );
}

export const DnsSubareaSpecs: Record<
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
  P054: {
    endpoint: "/api/v1/dns/zones",
    columns: [
      { key: "zone", label: "Zone" },
      { key: "type", label: "Record type" },
      { key: "value", label: "Value" },
    ],
  },
  P055: {
    endpoint: "/api/v1/dns/policies",
    columns: [
      { key: "policy", label: "Policy" },
      { key: "action", label: "Action" },
      { key: "target", label: "Target" },
    ],
  },
  P056: {
    endpoint: "/api/v1/dhcp/scopes",
    columns: [
      { key: "scope", label: "Scope" },
      { key: "pool", label: "Pool" },
      { key: "lease_count", label: "Lease count" },
    ],
  },
};

export function DnsSubareaStub({
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
      blocker="This DNS/DHCP area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}
