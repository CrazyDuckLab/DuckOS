/**
 * DuckOS V1 — System area configurators (P001–P006, P068–P081).
 *
 * Live:
 *  - P001 System Overview (summary, in Easy/Easy)
 *  - P002 Candidate (in Easy via store + Global Safe Apply dock)
 *  - P081 About / Build
 *
 * Stubs for the rest: history, dependencies, templates, users,
 * backup, updates, license, hardware, power, sites (covered in
 * services), provisioning, migrations, time, api, help, privacy,
 * webhooks.
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

export function SitesLivePage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<unknown> | null>(null);
  const [query, setQuery] = useState("");
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchSites({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );

  const sites = ((result.data as { sites?: Array<Record<string, unknown>> }).sites ?? []) as Array<Record<string, unknown>>;
  const filtered = sites.filter((s) => !query || `${s.name ?? s.hostname ?? ""}`.toLowerCase().includes(query.toLowerCase()));
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel rows={[{ key: "sites", value: sites.length }]} />
      <FilterBar value={query} onChange={setQuery} placeholder="Filter sites" />
      <DataTable
        rows={filtered}
        rowKey={(s, i) => String(s.name ?? s.id ?? i)}
        pageSize={20}
        emptyState="No sites configured."
        columns={[
          { key: "name", label: "Site", render: (s) => <strong>{String(s.name ?? s.id ?? "—")}</strong> },
          { key: "role", label: "Role", render: (s) => String(s.role ?? "—") },
          {
            key: "state",
            label: "State",
            render: (s) => (
              <span
                className="v1-pill"
                data-state={String(s.state ?? "").toLowerCase() === "active" ? "good" : "warn"}
              >
                {String(s.state ?? "—")}
              </span>
            ),
          },
          { key: "lag", label: "Lag", mono: true, render: (s) => String(s.lag ?? "—") },
        ]}
      />
    </PageFrame>
  );
}

export function BackupPage({
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
    client.fetchBackup({ signal: ctl.signal, fallbackToMissing: true }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );

  const backups = ((result.data as { backups?: Array<Record<string, unknown>> }).backups ?? []) as Array<Record<string, unknown>>;
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel rows={[{ key: "backups", value: backups.length }]} />
      <DataTable
        rows={backups}
        rowKey={(b, i) => String(b.id ?? b.createdAt ?? i)}
        pageSize={20}
        emptyState="No backups on file."
        columns={[
          { key: "id", label: "Backup", mono: true, render: (b) => <strong>{String(b.id ?? "—")}</strong> },
          { key: "createdAt", label: "Created", render: (b) => String(b.createdAt ?? "—") },
          {
            key: "verified",
            label: "Verified",
            render: (b) => (
              <span className="v1-pill" data-state={b.verified ? "good" : "warn"}>
                {b.verified ? "verified" : "unverified"}
              </span>
            ),
          },
          { key: "size", label: "Size", mono: true, numeric: true, render: (b) => String(b.size ?? "—") },
        ]}
      />
      <p className="v1-subtle v1-fs-xs">
        Restores are routed through the privileged agent only. The dashboard
        surfaces the existing public API and never invents restore actions.
      </p>
    </PageFrame>
  );
}

export function UpsPage({
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
    client.fetchUps({ signal: ctl.signal, fallbackToMissing: true }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );
  const ups = (result.data && typeof result.data === "object" ? result.data : {}) as Record<string, unknown>;
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "mains", value: String(ups.mains ?? "—") },
          { key: "battery", value: String(ups.battery ?? "—") },
          { key: "runtime", value: String(ups.runtime ?? "—") },
        ]}
      />
      <pre className="v1-code">{JSON.stringify(ups, null, 2)}</pre>
    </PageFrame>
  );
}

export function UsersPage({
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
    client.fetchAuthMe({ signal: ctl.signal, fallbackToMissing: true }).then(setResult);
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
        The Users and Roles view shows the currently-authenticated user via
        /api/v1/auth/me. A typed users list endpoint is part of the design
        and tracked under docs/ui/BACKEND_INTEGRATION_GAPS.md.
      </p>
      <pre className="v1-code">{JSON.stringify(result.data, null, 2)}</pre>
    </PageFrame>
  );
}

export const SystemSubareaSpecs: Record<string, { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string }> = {
  P003: { endpoint: "/api/v1/revision", columns: [
    { key: "revision", label: "Revision" },
    { key: "actor", label: "Actor" },
    { key: "changed", label: "Changed" },
    { key: "result", label: "Result" },
  ] },
  P004: { endpoint: "/api/v1/objects", columns: [
    { key: "object", label: "Object" },
    { key: "owner", label: "Owner" },
    { key: "dependents", label: "Dependents" },
  ] },
  P005: { endpoint: "/api/v1/templates", columns: [
    { key: "template", label: "Template" },
    { key: "target", label: "Target" },
    { key: "parameters", label: "Parameters" },
  ] },
  P068: { endpoint: "/api/v1/backup", columns: [
    { key: "id", label: "Backup" },
    { key: "size", label: "Size" },
    { key: "verified", label: "Verified" },
  ] },
  P069: { endpoint: "/api/v1/upgrade", columns: [
    { key: "release", label: "Release" },
    { key: "size", label: "Size" },
    { key: "impact", label: "Impact" },
  ] },
  P070: { endpoint: "/api/v1/license", columns: [
    { key: "field", label: "Field" },
    { key: "value", label: "Value" },
  ] },
  P071: { endpoint: "/api/v1/hardware", columns: [
    { key: "component", label: "Component" },
    { key: "value", label: "Value" },
    { key: "state", label: "State" },
  ] },
  P072: { endpoint: "/api/v1/ups", columns: [
    { key: "property", label: "Property" },
    { key: "value", label: "Value" },
    { key: "state", label: "State" },
  ] },
  P074: { endpoint: "/api/v1/ztp", columns: [
    { key: "device", label: "Device" },
    { key: "state", label: "State" },
    { key: "result", label: "Result" },
  ] },
  P075: { endpoint: "/api/v1/migrate", columns: [
    { key: "migration", label: "Migration" },
    { key: "stage", label: "Stage" },
    { key: "result", label: "Result" },
  ] },
  P076: { endpoint: "/api/v1/time", columns: [
    { key: "source", label: "Source" },
    { key: "offset", label: "Offset" },
    { key: "trust", label: "Trust" },
  ] },
  P077: { endpoint: "/api/v1/api-surface", columns: [
    { key: "endpoint", label: "Endpoint" },
    { key: "method", label: "Method" },
    { key: "schema", label: "Schema" },
  ] },
  P078: { endpoint: "/api/v1/migration-help", columns: [
    { key: "migration", label: "Migration" },
    { key: "recommendation", label: "Recommendation" },
    { key: "risk", label: "Risk" },
  ] },
  P079: { endpoint: "/api/v1/privacy", columns: [
    { key: "destination", label: "Destination" },
    { key: "purpose", label: "Purpose" },
    { key: "status", label: "Status" },
  ] },
  P080: { endpoint: "/api/v1/webhooks", columns: [
    { key: "target", label: "Target" },
    { key: "events", label: "Events" },
    { key: "last_delivery", label: "Last delivery" },
  ] },
};

export function SystemSubareaStub({
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
      blocker="This system area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}