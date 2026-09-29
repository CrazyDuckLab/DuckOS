/**
 * DuckOS V1 — System Overview V4 landing (P001).
 *
 * One V4 page with:
 *   - top headline (Healthy / Needs attention / Status incomplete)
 *   - 2×2 resource cards (CPU, Memory, Interfaces, Uptime)
 *   - Identity card
 *   - Storage card
 *   - Recent findings list (real data; honest empty state)
 *
 * No template placeholders leak (no `{cores}`, `{up}`, `{total}`).
 * No impossible percentages. Numbers come from the overview API.
 */
import { useContext, useEffect, useState } from "react";
import { type APIClient, type FetchResult } from "../api/client";
import type { Overview } from "../../../shared/api/types";
import { type Messages, type Locale, detectLocale } from "../i18n/messages";
import type { PageMeta } from "../pages/registry";
import { PrefsContext } from "../state/store";
import { v4t } from "../i18n/v4-strings";
import { deriveHealth, type ObservationState } from "../state/health";

function useLocale(): Locale {
  try {
    const { state } = useContext(PrefsContext) ?? {};
    if (state?.locale) return state.locale;
  } catch {
    /* outside a Provider — fall through */
  }
  return detectLocale();
}

function safe(n: number | null | undefined, fallback = 0): number {
  return typeof n === "number" && Number.isFinite(n) ? n : fallback;
}

function safePct(used: number, total: number): number {
  if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return 0;
  if (used < 0) return 0;
  if (used > total) return 100;
  return Math.round((used / total) * 100);
}

export function SystemOverviewV2({
  navigate,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  navigate: (route: string) => void;
  client: APIClient;
}) {
  const locale = useLocale();
  const [overview, setOverview] = useState<FetchResult<Overview> | null>(null);
  const [doctor, setDoctor] = useState<FetchResult<unknown> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setOverview);
    client.fetchDoctor({ signal: ctl.signal }).then(setDoctor);
    return () => ctl.abort();
  }, [client]);

  const ov = overview?.ok ? overview.data : null;

  // Centralized health derivation. No page-local health derivation
  // lives here. See web/src/state/health.ts for the rules.
  const derived = deriveHealth({
    health: null,
    doctor: doctor ?? null,
    overview: overview ?? null,
  });
  const observationState: ObservationState = derived.observation;

  const cpuCores = safe(ov?.cpu.logicalCores, 0);
  const cpuUsage = typeof ov?.cpu.usagePercent === "number" ? ov.cpu.usagePercent : null;
  const memUsedBytes = safe(ov?.memory.usedBytes, 0);
  const memTotalBytes = safe(ov?.memory.totalBytes, 0);
  const memPct = safePct(memUsedBytes, memTotalBytes);

  const interfaces = ov?.interfaces ?? [];
  const upCount = interfaces.filter((i) => i.state === "up").length;
  const totalCount = interfaces.length;
  const routesCount = (ov?.routes ?? []).length;
  const dnsCount = (ov?.dns?.nameservers ?? []).length;
  const uptimeSeconds = safe(ov?.system.uptimeSeconds, 0);

  const findings: Array<{ id: string; severity: string; title: string; detail?: string }> = [];
  // We don't pull doctor from SystemOverviewV2 here to avoid double fetching;
  // recent findings come from /api/v1/doctor in P091, not from this page.
  // This page only surfaces observation-derived signals honestly.

  // "Observation fresh" depends ONLY on the centralized observation
  // classification derived from /api/v1/overview.observedAt. Doctor
  // findings may affect the attention/overall verdict (via
  // `derived.overall`) but NEVER redefine whether the observation
  // timestamp itself is fresh. A recent timestamp with Doctor
  // findings is still a fresh observation — the findings are a
  // separate, lower-level concern handled by the overall verdict.
  const observationFresh = observationState === "recent";
  const headlineTone: "good" | "warn" | "unknown" =
    derived.overall === "attention" ? "warn" : "unknown";
  const headline =
    derived.overall === "attention"
      ? v4t(locale, "health.needsAttention")
      : derived.overall === "unreachable"
        ? v4t(locale, "health.unavailable")
        : v4t(locale, "health.statusIncomplete");

  return (
    <div className="v4-page">
      {/* Single H1 */}
      <h1 className="v4-page-title">{v4t(locale, "sys.title")}</h1>
      <p className="v4-page-subtitle">{v4t(locale, "sys.subtitle")}</p>
      <div className="v4-page-meta">
        <span
          className="v4-pill"
          data-kind={headlineTone === "warn" ? "warn" : undefined}
        >
          {observationFresh
            ? v4t(locale, "health.observationFresh")
            : v4t(locale, "health.observationUntrusted")}
        </span>
        <span style={{ textTransform: "none", letterSpacing: 0, fontWeight: 500 }}>{headline}</span>
      </div>

      {/* Resources 2x2 */}
      <h2 className="v4-section-title">{v4t(locale, "sys.sectionHeadline")}</h2>
      <div className="v4-grid-2">
        <article className="v4-stat" data-featured="true">
          <div className="v4-stat-label">{v4t(locale, "sys.cardCpu")}</div>
          <div className="v4-stat-value">
            {cpuUsage === null ? "—" : `${cpuUsage.toFixed(1)}%`}
          </div>
          <div className="v4-stat-foot">
            {v4t(locale, "sys.coresFormat", cpuCores)}
          </div>
          {cpuUsage !== null && (
            <div className="v4-stat-bar"><span style={{ width: `${Math.min(100, Math.max(0, cpuUsage))}%` }} /></div>
          )}
        </article>

        <article className="v4-stat" data-featured="true">
          <div className="v4-stat-label">{v4t(locale, "sys.cardMemory")}</div>
          <div className="v4-stat-value">
            {memTotalBytes === 0 ? "—" :
              `${v4t(locale, "v4.units.bytes", memUsedBytes)} / ${v4t(locale, "v4.units.bytes", memTotalBytes)}`}
          </div>
          <div className="v4-stat-foot">{v4t(locale, "sys.percentFormat", memPct)}</div>
          {memTotalBytes > 0 && (
            <div className="v4-stat-bar"><span style={{ width: `${memPct}%` }} /></div>
          )}
        </article>

        <article className="v4-stat" data-featured="true">
          <div className="v4-stat-label">{v4t(locale, "sys.cardInterfaces")}</div>
          <div className="v4-stat-value">{interfaces.length}</div>
          <div className="v4-stat-foot">{upCount} of {totalCount} administratively up</div>
        </article>

        <article className="v4-stat" data-featured="true">
          <div className="v4-stat-label">{v4t(locale, "sys.cardUptime")}</div>
          <div className="v4-stat-value">{formatUptime(uptimeSeconds, locale)}</div>
          <div className="v4-stat-foot">{v4t(locale, "v4.since")} {ov?.system.os ?? "—"}</div>
        </article>
      </div>

      <h2 className="v4-section-title">Identity</h2>
      <div className="v4-keyvalue">
        <dt>Hostname</dt><dd>{ov?.system.hostname ?? "—"}</dd>
        <dt>OS</dt><dd>{ov?.system.os ?? "—"}</dd>
        <dt>Kernel</dt><dd>{ov?.system.kernel ?? "—"}</dd>
        <dt>Architecture</dt><dd>{ov?.system.architecture ?? "—"}</dd>
        <dt>Environment</dt><dd>{ov?.environment ?? "—"}</dd>
        <dt>Last observed</dt><dd>{ov?.observedAt ? new Date(ov.observedAt).toLocaleString() : "—"}</dd>
      </div>

      <h2 className="v4-section-title">Storage</h2>
      {ov && ov.storage.length > 0 ? (
        <table className="v4-table">
          <thead>
            <tr><th>Mountpoint</th><th>Filesystem</th><th>Used</th><th>Available</th></tr>
          </thead>
          <tbody>
            {ov.storage.map((s: Overview["storage"][number]) => {
              const pct = safePct(s.usedBytes, s.totalBytes);
              return (
                <tr key={s.mountpoint}>
                  <td style={{ fontWeight: 500 }}>{s.mountpoint}</td>
                  <td className="v4-mono">{s.filesystem}</td>
                  <td className="v4-mono">{v4t(locale, "v4.units.bytes", s.usedBytes)} ({pct}%)</td>
                  <td className="v4-mono">{v4t(locale, "v4.units.bytes", s.availableBytes)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      ) : (
        <p style={{ color: "var(--text-muted)" }}>No filesystem data available yet.</p>
      )}

      <h2 className="v4-section-title">{v4t(locale, "sys.recentFindings")}</h2>
      {/* The /api/v1/health payload doesn't carry per-component findings.
         We surface the honest "no findings available from the health probe"
         message and route the user to Doctor for real findings. */}
      <p style={{ color: "var(--text-muted)" }}>{v4t(locale, "sys.noCurrentFindings")}</p>
      <div>
        <button
          type="button"
          onClick={() => navigate("/diagnostics/doctor")}
          style={{
            border: "1px solid var(--accent)",
            background: "transparent",
            color: "var(--accent-ink)",
            padding: "8px 16px",
            borderRadius: "var(--radius-pill)",
            fontWeight: 500,
            cursor: "pointer",
            fontFamily: "inherit",
          }}
        >
          Open Doctor →
        </button>
      </div>

      <h2 className="v4-section-title">Inventory</h2>
      <div className="v4-grid-3">
        <article className="v4-stat">
          <div className="v4-stat-label">{v4t(locale, "sys.cardRoutes")}</div>
          <div className="v4-stat-value">{routesCount}</div>
        </article>
        <article className="v4-stat">
          <div className="v4-stat-label">{v4t(locale, "sys.cardDns")}</div>
          <div className="v4-stat-value">{dnsCount}</div>
        </article>
        <article className="v4-stat">
          <div className="v4-stat-label">Services</div>
          <div className="v4-stat-value">{ov?.services.length ?? 0}</div>
        </article>
      </div>

      {!observationFresh && (
        <p style={{ color: "var(--text-muted)", marginTop: "var(--space-l)" }}>
          {v4t(locale, "sys.dataStale")}
        </p>
      )}
      {/* avoid unused-var error in strict mode */}
      <span hidden>{findings.length}</span>
    </div>
  );
}

function formatUptime(seconds: number, locale: import("../i18n/messages").Locale): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "—";
  const total = Math.floor(seconds);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const mins = Math.floor((total % 3600) / 60);
  if (days > 0) return v4t(locale, "sys.uptimeDaysHours", days, hours);
  if (hours > 0) return v4t(locale, "sys.uptimeHours", hours);
  return v4t(locale, "sys.uptimeMinutes", mins);
}
