/**
 * DuckOS V1 — Logs / Diagnostics area configurators (P082–P092).
 *
 * Live:
 *  - P087 Topology (existing Topology.tsx)
 *  - P089 Monitoring / History (via /overview)
 *  - P091 Doctor / Support (via /doctor)
 *
 * Stubs: incidents, logs, events, audit, explain, tools, speedlab, retention
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
import type { Overview } from "../../../shared/api/types";

export function MonitoringPage({
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
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );

  const cpu = result.data.cpu;
  const mem = result.data.memory;
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "logical cores", value: cpu.logicalCores },
          { key: "load 1m", value: cpu.loadAverage[0]?.toFixed(2) ?? "—" },
          { key: "load 5m", value: cpu.loadAverage[1]?.toFixed(2) ?? "—" },
          { key: "load 15m", value: cpu.loadAverage[2]?.toFixed(2) ?? "—" },
          { key: "memory used", value: mem.usedBytes },
          { key: "memory total", value: mem.totalBytes },
        ]}
        observedAt={result.data.observedAt}
      />
      <DataTable
        rows={result.data.storage}
        rowKey={(s) => s.mountpoint}
        pageSize={20}
        emptyState="No storage mounts observed."
        columns={[
          { key: "mountpoint", label: "Mount", mono: true, render: (s) => <strong>{s.mountpoint}</strong> },
          { key: "filesystem", label: "Filesystem" },
          { key: "usedBytes", label: "Used", mono: true, numeric: true, render: (s) => s.usedBytes },
          { key: "availableBytes", label: "Available", mono: true, numeric: true, render: (s) => s.availableBytes },
          { key: "totalBytes", label: "Total", mono: true, numeric: true, render: (s) => s.totalBytes },
        ]}
      />
      <p className="v1-subtle v1-fs-xs">
        Time-series for traffic, latency, and errors require a typed
        /api/v1/monitoring/series endpoint, tracked under
        docs/ui/BACKEND_INTEGRATION_GAPS.md.
      </p>
    </PageFrame>
  );
}

export function DoctorPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<unknown> | null>(null);
  const [severity, setSeverity] = useState<"all" | "warn" | "critical" | "info">("all");
  const [query, setQuery] = useState("");
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchDoctor({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );

  const findings = (Array.isArray(result.data) ? result.data : (result.data as { findings?: Array<Record<string, unknown>> }).findings ?? []) as Array<Record<string, unknown>>;
  const filtered = findings.filter((f) => {
    const sv = String(f.severity ?? "").toLowerCase();
    if (severity === "warn" && !(sv === "warn" || sv === "warning")) return false;
    if (severity === "critical" && !(sv === "critical" || sv === "error" || sv === "fatal")) return false;
    if (severity === "info" && !(sv === "info" || sv === "ok" || sv === "good")) return false;
    if (!query) return true;
    return `${f.component ?? ""} ${f.code ?? ""} ${f.message ?? ""}`.toLowerCase().includes(query.toLowerCase());
  });

  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "findings", value: findings.length },
          { key: "warnings", value: findings.filter((f) => ["warn", "warning"].includes(String(f.severity ?? "").toLowerCase())).length },
          { key: "critical", value: findings.filter((f) => ["critical", "error", "fatal"].includes(String(f.severity ?? "").toLowerCase())).length },
        ]}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Filter findings"
        right={
          <select
            className="v1-select"
            value={severity}
            onChange={(e) => setSeverity(e.target.value as typeof severity)}
            aria-label="Severity"
          >
            <option value="all">All severities</option>
            <option value="critical">Critical</option>
            <option value="warn">Warning</option>
            <option value="info">Info</option>
          </select>
        }
      />
      <DataTable
        rows={filtered}
        rowKey={(f, i) => String(f.code ?? f.component ?? i)}
        pageSize={20}
        emptyState="No findings."
        columns={[
          {
            key: "severity",
            label: "Severity",
            render: (f) => {
              const sv = String(f.severity ?? "info").toLowerCase();
              const tone = ["critical", "error", "fatal"].includes(sv)
                ? "bad"
                : ["warn", "warning"].includes(sv)
                  ? "warn"
                  : "good";
              return <span className="v1-pill" data-state={tone}>{String(f.severity ?? "info")}</span>;
            },
          },
          { key: "component", label: "Component", render: (f) => <strong>{String(f.component ?? "—")}</strong> },
          { key: "code", label: "Code", mono: true, render: (f) => String(f.code ?? "—") },
          { key: "message", label: "Message" },
        ]}
      />
    </PageFrame>
  );
}

export const LogsSubareaSpecs: Record<string, { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string }> = {
  P082: { endpoint: "/api/v1/incidents", columns: [
    { key: "component", label: "Component" },
    { key: "severity", label: "Severity" },
    { key: "age", label: "Age" },
    { key: "evidence", label: "Evidence" },
  ] },
  P083: { endpoint: "/api/v1/logs", columns: [
    { key: "source", label: "Source" },
    { key: "severity", label: "Severity" },
    { key: "last_line", label: "Last line" },
  ] },
  P084: { endpoint: "/api/v1/events", columns: [
    { key: "time", label: "Time" },
    { key: "component", label: "Component" },
    { key: "severity", label: "Severity" },
    { key: "message", label: "Message" },
  ] },
  P085: { endpoint: "/api/v1/auth/audit", columns: [
    { key: "time", label: "Time" },
    { key: "actor", label: "Actor" },
    { key: "action", label: "Action" },
    { key: "result", label: "Result" },
  ] },
  P086: { endpoint: "/api/v1/engine/explain", columns: [
    { key: "question", label: "Question" },
    { key: "result", label: "Result" },
  ] },
  P088: { endpoint: "/api/v1/tools", columns: [
    { key: "tool", label: "Tool" },
    { key: "status", label: "Status" },
  ] },
  P090: { endpoint: "/api/v1/speedlab", columns: [
    { key: "direction", label: "Direction" },
    { key: "speed", label: "Speed" },
    { key: "latency", label: "Latency" },
  ] },
  P092: { endpoint: "/api/v1/retention", columns: [
    { key: "source", label: "Source" },
    { key: "retention", label: "Retention" },
    { key: "used", label: "Used" },
  ] },
};

export function LogsSubareaStub({
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
      blocker="This diagnostics area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}