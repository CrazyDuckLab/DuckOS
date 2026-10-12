/**
 * DuckOS V1 — Services area configurators (P051, P058–P066).
 *
 * Live:
 *  - P058 Services / Listeners via /overview.services
 *  - P073 (handled by router — sites) via /sites
 *  - P025 (Wi-Fi) via /wifi
 *
 * Stubs:
 *  - P051 Subscriptions, P059 Sites (extended), P060 Certificates,
 *    P061 PKI, P062 AAA, P063 Time, P064 Telemetry, P065 UPnP, P066 Portal
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable, FilterBar } from "../components/forms/Table";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import { serviceToInspector } from "../components/inspector/selectors";
import { usePrefs } from "../state/store";
import type { Overview } from "../../../shared/api/types";

export function ServicesLivePage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<Overview> | null>(null);
  const [filter, setFilter] = useState<"all" | "active" | "failed">("all");
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

  const q = query.toLowerCase();
  const filtered = result.data.services.filter((s) => {
    if (filter === "active" && s.activeState !== "active") return false;
    if (
      filter === "failed" &&
      s.activeState !== "failed" &&
      s.subState !== "failed"
    )
      return false;
    if (!q) return true;
    return `${s.name} ${s.loadState} ${s.activeState} ${s.subState}`
      .toLowerCase()
      .includes(q);
  });

  const counts = {
    total: result.data.services.length,
    active: result.data.services.filter((s) => s.activeState === "active")
      .length,
    failed: result.data.services.filter(
      (s) => s.activeState === "failed" || s.subState === "failed",
    ).length,
  };

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "total", value: counts.total },
          { key: "active", value: counts.active },
          { key: "failed", value: counts.failed },
        ]}
        observedAt={result.data.observedAt}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Filter services"
        right={
          <select
            className="v1-select"
            value={filter}
            onChange={(e) => setFilter(e.target.value as typeof filter)}
            aria-label="State filter"
          >
            <option value="all">All</option>
            <option value="active">Active</option>
            <option value="failed">Failed</option>
          </select>
        }
      />
      <DataTable
        rows={filtered}
        rowKey={(s) => `${s.name}`}
        pageSize={25}
        initialSort={{ key: "name", direction: "asc" }}
        emptyState="No services match this view."
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: serviceToInspector(page.id, {
              name: row.name,
              kind: "systemd",
              listen: "—",
              port: 0,
              protocol: "—",
              state: row.activeState,
            }),
          });
        }}
        columns={[
          {
            key: "name",
            label: "Service",
            mono: true,
            render: (s) => <strong>{s.name}</strong>,
          },
          {
            key: "loadState",
            label: "Load",
            render: (s) => (
              <span
                className="v1-pill"
                data-state={s.loadState === "loaded" ? "good" : "warn"}
              >
                {s.loadState}
              </span>
            ),
          },
          {
            key: "activeState",
            label: "Active",
            render: (s) => (
              <span
                className="v1-pill"
                data-state={
                  s.activeState === "active"
                    ? "good"
                    : s.activeState === "failed"
                      ? "bad"
                      : "warn"
                }
              >
                {s.activeState}
              </span>
            ),
          },
          { key: "subState", label: "Sub", render: (s) => s.subState },
        ]}
      />
    </PageFrame>
  );
}

export function SubscriptionsPage({
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
    client
      .fetchConfig({ signal: ctl.signal, fallbackToMissing: true })
      .then(setResult);
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
  const subs =
    (result.data as { subscriptions?: Array<Record<string, unknown>> })
      .subscriptions ?? [];
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[{ key: "subscriptions", value: subs.length }]}
      />
      <DataTable
        rows={subs as Array<Record<string, unknown>>}
        rowKey={(s, i) => String(s.id ?? s.name ?? i)}
        pageSize={20}
        emptyState="No subscriptions on file."
        columns={[
          {
            key: "name",
            label: "Subscription",
            render: (s) => <strong>{String(s.name ?? s.id ?? "—")}</strong>,
          },
          {
            key: "status",
            label: "Status",
            render: (s) => String(s.status ?? "—"),
          },
          {
            key: "expires",
            label: "Expires",
            render: (s) => String(s.expires ?? "—"),
          },
        ]}
      />
      {subs.length === 0 && (
        <p className="v1-subtle v1-fs-xs">
          A typed subscriptions surface is not exposed by the public API today.
          The dashboard will populate this list as soon as the agent publishes
          it.
        </p>
      )}
    </PageFrame>
  );
}

export const ServicesSubareaSpecs: Record<
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
  P059: {
    endpoint: "/api/v1/sites",
    columns: [
      { key: "hostname", label: "Hostname" },
      { key: "backend", label: "Backend" },
      { key: "tls", label: "TLS" },
    ],
  },
  P060: {
    endpoint: "/api/v1/cert",
    columns: [
      { key: "domain", label: "Domain" },
      { key: "issuer", label: "Issuer" },
      { key: "expires", label: "Expires" },
    ],
  },
  P061: {
    endpoint: "/api/v1/pki",
    columns: [
      { key: "ca", label: "CA" },
      { key: "type", label: "Type" },
      { key: "issued", label: "Issued" },
    ],
  },
  P062: {
    endpoint: "/api/v1/aaa",
    columns: [
      { key: "server", label: "Server" },
      { key: "method", label: "Method" },
      { key: "state", label: "State" },
    ],
  },
  P063: {
    endpoint: "/api/v1/time",
    columns: [
      { key: "source", label: "Source" },
      { key: "offset", label: "Offset" },
      { key: "stratum", label: "Stratum" },
    ],
  },
  P064: {
    endpoint: "/api/v1/telemetry",
    columns: [
      { key: "target", label: "Target" },
      { key: "type", label: "Type" },
      { key: "last_seen", label: "Last seen" },
    ],
  },
  P065: {
    endpoint: "/api/v1/upnp",
    columns: [
      { key: "service", label: "Service" },
      { key: "rule", label: "Rule" },
      { key: "last_use", label: "Last use" },
    ],
  },
  P066: {
    endpoint: "/api/v1/portal",
    columns: [
      { key: "profile", label: "Profile" },
      { key: "method", label: "Method" },
      { key: "clients", label: "Clients" },
    ],
  },
};

export function ServicesSubareaStub({
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
      blocker="This services area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}
