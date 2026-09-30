/**
 * DuckOS V1 — Easy mode (V4 redesign).
 *
 * Single-file V4 rewrite. Six pages that read directly from real
 * backend state:
 *
 *   P094 Home         — "Is everything working?" dashboard
 *   P095 Internet     — connection / gateway / DNS
 *   P096 Networks     — LAN + Wi-Fi
 *   P097 Devices      — observed clients
 *   P098 Publish      — published services
 *   P099 Care         — Doctor findings + recovery pointers
 *
 * Easy mode never displays speed / latency / public IP / guest network
 * because those concepts are not exposed by the public API today.
 * Destructive actions always redirect to Full mode.
 */
import { useContext, useEffect, useState } from "react";
import { type APIClient, type FetchResult, type HealthPayload } from "../api/client";
import type { Overview } from "../../../shared/api/types";
import type { WireguardServerView, XrayServerView } from "../api/types";
import { type Messages, type Locale, detectLocale } from "../i18n/messages";
import type { PageMeta } from "../pages/registry";
import { PrefsContext } from "../state/store";
import { v4t } from "../i18n/v4-strings";
import { Icon } from "../icons";
import { EmptyState } from "./EmptyState";
import { deriveHealth } from "../state/health";

function useLocale(): Locale {
  // Prefer the data-locale attribute on the document or shell — it is
  // set by AppShell whenever the locale changes, including after
  // localStorage hydration, so it always reflects the user's pick.
  if (typeof document !== "undefined") {
    const html = document.documentElement.getAttribute("data-locale");
    if (html === "ru" || html === "de" || html === "es" || html === "en") return html;
    const shell = document.querySelector(".duck-shell")?.getAttribute("data-locale");
    if (shell === "ru" || shell === "de" || shell === "es" || shell === "en") return shell;
  }
  // Then the prefs context (covers components rendered before the
  // AppShell writes data-locale on first mount).
  try {
    const ctx = useContext(PrefsContext);
    if (ctx?.state?.locale) return ctx.state.locale;
  } catch {
    /* outside a Provider — fall through */
  }
  return detectLocale();
}

function useOverview(client: APIClient) {
  const [data, setData] = useState<FetchResult<Overview> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setData);
    return () => ctl.abort();
  }, [client]);
  return data;
}

function useHealth(client: APIClient) {
  const [data, setData] = useState<FetchResult<HealthPayload> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchHealth({}).then(setData);
    return () => ctl.abort();
  }, [client]);
  return data;
}

function useDoctor(client: APIClient) {
  const [data, setData] = useState<FetchResult<unknown> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchDoctor({}).then(setData);
    return () => ctl.abort();
  }, [client]);
  return data;
}

/* — Easy Home — */

export function EasyHomePage({
  navigate,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  navigate: (route: string) => void;
  client: APIClient;
}) {
  const locale = useLocale();
  const overview = useOverview(client);
  const doctor = useDoctor(client);
  const health = useHealth(client);
  const ov = overview?.ok ? overview.data : null;

  // Centralized health derivation. No page-local health derivation
  // lives here. See web/src/state/health.ts for the rules.
  const derived = deriveHealth({
    health: health ?? null,
    doctor: doctor ?? null,
    overview: overview ?? null,
  });

  const interfaces = ov?.interfaces ?? [];
  // Easy Mode hides the loopback interface. Full Mode still
  // exposes it (and the user can switch modes without losing
  // any actual device). The exclusion is local to Easy Mode
  // because the loopback is not a real "device" the operator
  // needs to act on.
  const realInterfaces = interfaces.filter((i) => i.name !== "lo");
  const upCount = realInterfaces.filter((i) => i.state === "up").length;
  const totalCount = realInterfaces.length;
  const dnsCount = ov?.dns?.nameservers?.length ?? 0;

  // Headline tone mirrors the centralized derived overall.
  //   - "attention" → warn (Doctor raised findings or overview has issues)
  //   - "unreachable" / "stale" / "incomplete" → unknown (honest, not "good")
  const headlineTone: "warn" | "unknown" =
    derived.overall === "attention" ? "warn" : "unknown";
  const headlineKey =
    derived.overall === "attention"
      ? "easyHome.statusAttention"
      : derived.overall === "unreachable"
        ? "easyHome.statusUnavailable"
        : "easyHome.statusIncomplete";
  const headlineLabel = v4t(locale, headlineKey);
  const headlineIcon =
    derived.overall === "attention" ? "warn" : "info";

  // System tile — STRICTLY consumed from the centralized verdict.
  // No local positive derivation here. The verdict is the verdict;
  // we don't compute any "everything is verified" condition from
  // individual signals because no production API provides the
  // verified system health evidence that would justify it. Today
  // the verdict enum has no "good" member, so the tile collapses
  // to "warn" only on real attention, else "muted" with the
  // honest incomplete label.
  const systemTileState: "good" | "warn" | "muted" =
    derived.overall === "attention"
      ? "warn"
      : "muted";
  const systemStateLabel =
    derived.overall === "attention"
      ? v4t(locale, "easyHome.statusAttention")
      : v4t(locale, "easyHome.statusIncomplete");

  // Internet tile — NO positive derivation. No production API
  // explicitly proves Internet connectivity today, so we MUST NOT
  // claim "Internet is working" / "Internet is verified" from any
  // combination of API reachability, observation freshness, or
  // Doctor success. We surface a neutral pending state and link
  // the user to the Easy Internet page where they can verify
  // connectivity themselves.
  const internetTileState: "warn" | "muted" =
    derived.overall === "attention" ? "warn" : "muted";
  const internetStateLabel =
    derived.overall === "attention"
      ? v4t(locale, "easyHome.statusAttention")
      : v4t(locale, "easyHome.statusInternetIncomplete");

  return (
    <div className="v4-page">
      <div
        className="easy-headline"
        data-tone={headlineTone === "warn" ? "warn" : undefined}
      >
        <div className="h-icon">
          <Icon name={headlineIcon} size={28} />
        </div>
        <div>
          <h2>{headlineLabel}</h2>
          <p className="h-sub">{v4t(locale, "easyHome.subtitle")}</p>
        </div>
        <div className="h-actions">
          <button
            type="button"
            onClick={() => navigate("/easy/care")}
            style={{
              background: "transparent",
              border: "1px solid var(--line)",
              color: "var(--text)",
              padding: "10px 16px",
              borderRadius: "var(--radius-pill)",
              fontWeight: 500,
              cursor: "pointer",
              display: "inline-flex", gap: 6, alignItems: "center",
              fontFamily: "inherit", fontSize: "var(--fs-s)",
            }}
          >
            <Icon name="doctor" size={16} /> {v4t(locale, "easyHome.systemOpen")}
          </button>
        </div>
      </div>

      <div className="easy-tiles">
        <EasyTile
          icon="internet"
          title={v4t(locale, "easyHome.internetCard")}
          body={v4t(locale, "easyHome.internetSub")}
          meta={`${upCount} of ${totalCount} up · ${dnsCount} DNS`}
          state={internetTileState}
          stateLabel={internetStateLabel}
          primaryLabel={v4t(locale, "easyHome.internetConfigure")}
          onPrimary={() => navigate("/easy/internet")}
        />
        <EasyTile
          icon="wifi"
          title={v4t(locale, "easyHome.wifiCard")}
          body={v4t(locale, "easyHome.wifiSub")}
          meta={v4t(locale, "easyHome.unknownValue")}
          state="muted"
          stateLabel={v4t(locale, "easyHome.details")}
          primaryLabel={v4t(locale, "easyHome.wifiConfigure")}
          onPrimary={() => navigate("/easy/networks")}
        />
        <EasyTile
          icon="devices"
          title={v4t(locale, "easyHome.devicesCard")}
          body={v4t(locale, "easyHome.devicesSub")}
          meta={`${realInterfaces.length} interfaces · ${realInterfaces.filter((i) => i.addresses.length > 0).length} with addresses`}
          state="muted"
          stateLabel={v4t(locale, "easyHome.details")}
          primaryLabel={v4t(locale, "easyHome.devicesOpen")}
          onPrimary={() => navigate("/easy/devices")}
        />
        <EasyTile
          icon="vpn"
          title={v4t(locale, "easyHome.vpnCard")}
          body={v4t(locale, "easyHome.vpnSub")}
          meta=""
          state="muted"
          stateLabel={v4t(locale, "easyHome.details")}
          primaryLabel={v4t(locale, "easyHome.vpnConfigure")}
          onPrimary={() => navigate("/easy/publish")}
        />
      </div>

      <div className="easy-tiles" style={{ gridTemplateColumns: "1fr" }}>
        <EasyTile
          icon="system"
          title={v4t(locale, "easyHome.systemCard")}
          body={v4t(locale, "easyHome.systemSub")}
          meta=""
          // Never claim "Healthy" on absence of findings alone — only
          // when we have actual freshness evidence from
          // /api/v1/overview AND verified system health. Otherwise
          // show muted / incomplete so the UI never lies about
          // verified health.
          state={systemTileState}
          stateLabel={systemStateLabel}
          primaryLabel={v4t(locale, "easyHome.systemOpen")}
          onPrimary={() => navigate("/easy/care")}
        />
      </div>

      <h2 className="v4-section-title">{v4t(locale, "easyHome.aboutTitle")}</h2>
      <dl className="easy-about">
        <div>
          <dt>{v4t(locale, "easyHome.aboutHostname")}</dt>
          <dd>{ov?.system.hostname ?? v4t(locale, "easyHome.unknownValue")}</dd>
        </div>
        <div>
          <dt>{v4t(locale, "easyHome.aboutUptime")}</dt>
          <dd>{formatUptime(ov?.system.uptimeSeconds ?? 0, locale)}</dd>
        </div>
        <div>
          <dt>{v4t(locale, "easyHome.aboutEnvironment")}</dt>
          <dd>{ov?.environment ?? v4t(locale, "easyHome.unknownValue")}</dd>
        </div>
        <div>
          <dt>{v4t(locale, "easyHome.aboutOs")}</dt>
          <dd>{ov?.system.os ?? v4t(locale, "easyHome.unknownValue")}</dd>
        </div>
        <div>
          <dt>{v4t(locale, "easyHome.aboutKernel")}</dt>
          <dd>{ov?.system.kernel ?? v4t(locale, "easyHome.unknownValue")}</dd>
        </div>
        <div>
          <dt>{v4t(locale, "easyHome.aboutArchitecture")}</dt>
          <dd>{ov?.system.architecture ?? v4t(locale, "easyHome.unknownValue")}</dd>
        </div>
      </dl>
    </div>
  );
}

function EasyTile(props: {
  icon: string;
  title: string;
  body: string;
  meta?: string;
  state: "good" | "warn" | "bad" | "muted";
  stateLabel: string;
  primaryLabel: string;
  onPrimary: () => void;
}) {
  return (
    <div className="easy-tile" data-tone={props.state}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <Icon name={props.icon} size={20} />
        <h3>{props.title}</h3>
      </div>
      <p style={{ margin: 0, color: "var(--text-muted)", fontSize: "var(--fs-s)" }}>
        {props.body}
      </p>
      {props.meta && (
        <div className="tile-meta" style={{ color: "var(--text)" }}>{props.meta}</div>
      )}
      <div className="tile-state">{props.stateLabel}</div>
      <div className="tile-actions">
        <button type="button" onClick={props.onPrimary}>{props.primaryLabel} →</button>
      </div>
    </div>
  );
}

function formatUptime(seconds: number, locale: Locale): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return v4t(locale, "easyHome.unknownValue");
  const total = Math.floor(seconds);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const mins = Math.floor((total % 3600) / 60);
  if (days > 0) return v4t(locale, "sys.uptimeDaysHours", days, hours);
  if (hours > 0) return v4t(locale, "sys.uptimeHours", hours);
  return v4t(locale, "sys.uptimeMinutes", mins);
}

/* — Easy Internet — */

export function EasyInternetPage({
  navigate,
  client,
  bundle: _bundle,
}: {
  page: PageMeta;
  bundle: Messages;
  navigate: (route: string) => void;
  client: APIClient;
}) {
  const locale = useLocale();
  const overview = useOverview(client);
  const ov = overview?.ok ? overview.data : null;
  const interfaces = (ov?.interfaces ?? []).filter((i) => i.name?.startsWith("wan"));
  const defaultRoute = (ov?.routes ?? []).find((r) => r.destination === "default");
  const gateway = defaultRoute?.gateway ?? "";
  const dns = ov?.dns?.nameservers ?? [];

  return (
    <div className="v4-page">
      <h1 className="v4-page-title">{v4t(locale, "easyHome.internetCard")}</h1>
      <p className="v4-page-subtitle">{v4t(locale, "easyHome.internetSub")}</p>
      <div className="v4-page-meta">
        <span className="v4-pill" data-kind="muted">live</span>
        <span>{v4t(locale, "sys.cardInterfaces")} {interfaces.length}</span>
      </div>

      <div className="v4-grid-2">
        <article className="v4-card">
          <div className="v4-card-head">
            <span className="v4-card-icon"><Icon name="router" size={18} /></span>
            <h3>Gateway</h3>
          </div>
          <div className="v4-mono" style={{ fontSize: "var(--fs-l)", color: "var(--text-strong)" }}>{gateway || v4t(locale, "easyHome.unknownValue")}</div>
          <div style={{ marginTop: 8, fontSize: "var(--fs-s)", color: "var(--text-muted)" }}>
            default route via {interfaces[0]?.name ?? "—"}
          </div>
        </article>
        <article className="v4-card">
          <div className="v4-card-head">
            <span className="v4-card-icon"><Icon name="dns" size={18} /></span>
            <h3>DNS</h3>
          </div>
          <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
            {dns.length === 0 && <li style={{ color: "var(--text-muted)" }}>{v4t(locale, "easyHome.unknownValue")}</li>}
            {dns.map((d) => (
              <li key={d} className="v4-mono" style={{ padding: "4px 0" }}>{d}</li>
            ))}
          </ul>
        </article>
      </div>

      <h2 className="v4-section-title">Uplinks</h2>
      {interfaces.length === 0 ? (
        <EmptyState
          icon="internet"
          title="No uplinks"
          body="No WAN interfaces are visible. Check that an external interface is administratively up."
          action="Configure in Full mode"
          onAction={() => navigate("/wan/connections")}
        />
      ) : (
        <table className="v4-table">
          <thead>
            <tr>
              <th>Interface</th>
              <th>Address</th>
              <th>State</th>
              <th>Gateway</th>
            </tr>
          </thead>
          <tbody>
            {interfaces.map((i) => (
              <tr key={i.name}>
                <td style={{ fontWeight: 500 }}>{i.name}</td>
                <td className="v4-mono">{i.addresses[0]?.address ?? "—"}</td>
                <td>
                  <span className="v4-status-pill" data-tone={i.state === "up" ? "good" : "bad"}>
                    {i.state}
                  </span>
                </td>
                <td className="v4-mono">{gateway || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/* — Easy Networks (Wi-Fi + LAN) — */

export function EasyNetworksPage({
  navigate,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  navigate: (route: string) => void;
  client: APIClient;
}) {
  const locale = useLocale();
  const overview = useOverview(client);
  const ov = overview?.ok ? overview.data : null;
  const lanIfaces = (ov?.interfaces ?? []).filter((i) => i.name?.startsWith("lan") || i.name?.startsWith("wlan"));
  return (
    <div className="v4-page">
      <h1 className="v4-page-title">Networks</h1>
      <p className="v4-page-subtitle">{v4t(locale, "easyHome.wifiSub")}</p>
      {lanIfaces.length === 0 ? (
        <EmptyState
          icon="wifi"
          title="Wi-Fi status unavailable"
          body="Wi-Fi details for this device are not currently visible. Advanced settings will open Full mode."
          action="Configure in Full mode"
          onAction={() => navigate("/lan/wifi")}
        />
      ) : (
        <table className="v4-table">
          <thead>
            <tr><th>Interface</th><th>Address</th><th>State</th></tr>
          </thead>
          <tbody>
            {lanIfaces.map((i) => (
              <tr key={i.name}>
                <td style={{ fontWeight: 500 }}>{i.name}</td>
                <td className="v4-mono">{i.addresses[0]?.address ?? "—"}</td>
                <td><span className="v4-status-pill" data-tone={i.state === "up" ? "good" : "bad"}>{i.state}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

/* — Easy Devices — */

export function EasyDevicesPage({ client }: { page: PageMeta; bundle: Messages; navigate: (r: string) => void; client: APIClient }) {
  const locale = useLocale();
  const overview = useOverview(client);
  const ov = overview?.ok ? overview.data : null;
  // Easy Mode hides the loopback interface. Full Mode still
  // exposes it. The exclusion is local because loopback is
  // not a real device the operator needs to act on.
  const interfaces = ov?.interfaces ?? [];
  const realInterfaces = interfaces.filter((i) => i.name !== "lo");
  return (
    <div className="v4-page">
      <h1 className="v4-page-title">{v4t(locale, "easyHome.devicesCard")}</h1>
      <p className="v4-page-subtitle">{v4t(locale, "easyHome.devicesSub")}</p>
      <div className="v4-page-meta">
        <span className="v4-pill" data-kind="muted">{realInterfaces.length} interfaces</span>
        <span>{realInterfaces.filter((i) => i.addresses.length > 0).length} with addresses</span>
      </div>
      <table className="v4-table">
        <thead>
          <tr><th>Interface</th><th>Address</th><th>State</th></tr>
        </thead>
        <tbody>
          {realInterfaces.map((i) => (
            <tr key={i.name}>
              <td style={{ fontWeight: 500 }}>{i.name}</td>
              <td className="v4-mono">{i.addresses[0]?.address ?? "—"}</td>
              <td><span className="v4-status-pill" data-tone={i.state === "up" ? "good" : "bad"}>{i.state}</span></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* — Easy Publish — */

export function EasyPublishPage({ client }: { page: PageMeta; bundle: Messages; navigate: (r: string) => void; client: APIClient }) {
  const locale = useLocale();
  const [wg, setWg] = useState<FetchResult<WireguardServerView[]> | null>(null);
  const [xray, setXray] = useState<FetchResult<XrayServerView[]> | null>(null);
  useEffect(() => {
    client.fetchWireguardServers({}).then(setWg);
    client.fetchXrayServers({}).then(setXray);
  }, [client]);
  return (
    <div className="v4-page">
      <h1 className="v4-page-title">Publish</h1>
      <p className="v4-page-subtitle">Tell this device which services are reachable from outside.</p>
      <div className="v4-page-meta">
        <span className="v4-pill" data-kind="muted">{wg?.ok ? wg.data.length : 0} VPN servers</span>
      </div>
      <div className="v4-grid-2">
        <article className="v4-card">
          <div className="v4-card-head">
            <span className="v4-card-icon"><Icon name="vpn" size={18} /></span>
            <h3>VPN</h3>
          </div>
          {(wg?.ok ? wg.data : []).length === 0 ? (
            <EmptyState
              icon="vpn"
              title={v4t(locale, "empty.vpn.title")}
              body={v4t(locale, "empty.vpn.body")}
              action={v4t(locale, "empty.vpn.body") ? "Configure VPN" : "Configure"}
              onAction={() => { window.location.hash = "#/tunnels/wireguard"; }}
            />
          ) : (
            <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
              {wg?.ok && wg.data.map((s) => (
                <li key={s.ref.id} style={{ display: "flex", justifyContent: "space-between", padding: "6px 0", borderBottom: "1px solid var(--line-soft)" }}>
                  <span>{s.displayName}</span>
                  <span className="v4-status-pill" data-tone={s.availability === "available" ? "good" : "muted"}>{s.availability}</span>
                </li>
              ))}
            </ul>
          )}
        </article>
        <article className="v4-card">
          <div className="v4-card-head">
            <span className="v4-card-icon"><Icon name="tunnel" size={18} /></span>
            <h3>Xray</h3>
          </div>
          {(xray?.ok ? xray.data : []).length === 0 ? (
            <EmptyState
              icon="tunnel"
              title={v4t(locale, "empty.xray.title")}
              body={v4t(locale, "empty.xray.body")}
              action={v4t(locale, "empty.xray.action")}
              onAction={() => { window.location.hash = "#/tunnels/xray"; }}
            />
          ) : (
            <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
              {xray?.ok && xray.data.map((s) => (
                <li key={s.ref.id} style={{ display: "flex", justifyContent: "space-between", padding: "6px 0", borderBottom: "1px solid var(--line-soft)" }}>
                  <span>{s.displayName}</span>
                  <span className="v4-status-pill" data-tone="muted">{s.availability}</span>
                </li>
              ))}
            </ul>
          )}
        </article>
      </div>
    </div>
  );
}

/* — Easy Care — */

export function EasyCarePage({
  navigate,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  navigate: (r: string) => void;
  client: APIClient;
}) {
  const locale = useLocale();
  const doctor = useDoctor(client);
  const health = useHealth(client);

  // Centralized health derivation. Doctor findings come from here
  // (null when Doctor failed) — we never auto-convert a Doctor
  // error to "no findings".
  const derived = deriveHealth({
    health: health ?? null,
    doctor: doctor ?? null,
    overview: null,
  });
  const doctorState = derived.doctor;
  // `findings` is `null` until Doctor has responded. We do NOT
  // substitute `[]` for `null` because `[]` semantically means
  // "Doctor reported zero findings" — that is only valid after a
  // successful Doctor response. Before that, the count line is
  // suppressed entirely (loading / error states do not show a
  // numeric count).
  const findings = derived.doctorFindings;
  // Loading != empty. Doctor findings may be [] ONLY after a
  // successful Doctor request that returned an empty findings
  // collection. Loading and error are distinct from empty:
  //   loading    — no fetch resolved yet; show a neutral state,
  //                do NOT claim "no current Doctor findings"
  //   error      — Doctor unreachable; show "Doctor unavailable"
  //   empty      — Doctor success + zero findings; show
  //                "No current Doctor findings"
  //   findings   — Doctor success + ≥1 finding; show the cards
  const doctorStatus: "loading" | "unavailable" | "empty" | "findings" =
    doctorState === "loading"
      ? "loading"
      : doctorState === "error"
        ? "unavailable"
        : findings && findings.length > 0
          ? "findings"
          : "empty";
  const healthReachable = derived.api === "reachable";
  return (
    <div className="v4-page">
      <h1 className="v4-page-title">{v4t(locale, "easyCare.title")}</h1>
      <p className="v4-page-subtitle">{v4t(locale, "easyCare.subtitle")}</p>
      <div className="v4-page-meta">
        <span className="v4-pill" data-kind={healthReachable ? undefined : "warn"}>
          {healthReachable
            ? v4t(locale, "easyCare.apiReachable")
            : v4t(locale, "easyCare.apiUnreachable")}
        </span>
        {/* Numeric count is only rendered when Doctor has
            successfully responded. Loading / error states
            intentionally omit the count line so they can never
            be confused with "0 findings". */}
        {findings !== null && (
          <span>
            {findings.length} {v4t(locale, "easyCare.findingsCount")}
          </span>
        )}
      </div>

      {doctorStatus === "loading" ? (
        <EmptyState
          icon="doctor"
          title={v4t(locale, "easyCare.doctorLoadingTitle")}
          body={v4t(locale, "easyCare.doctorLoadingBody")}
        />
      ) : doctorStatus === "unavailable" ? (
        <EmptyState
          icon="doctor"
          title={v4t(locale, "easyCare.doctorUnavailableTitle")}
          body={v4t(locale, "easyCare.doctorUnavailableBody")}
          action={v4t(locale, "easyCare.doctorUnavailableAction")}
          onAction={() => {
            // The page re-fetches on remount; the simplest "Retry"
            // is to navigate to the same route again.
            navigate("/easy/care");
          }}
        />
      ) : doctorStatus === "empty" ? (
        <EmptyState
          icon="doctor"
          title={v4t(locale, "easyCare.doctorEmptyTitle")}
          body={v4t(locale, "easyCare.doctorEmptyBody")}
          action={v4t(locale, "easyCare.doctorEmptyAction")}
          onAction={() => navigate("/diagnostics/doctor")}
        />
      ) : (
        <div className="v4-grid-2">
          {(findings ?? []).map((f, idx) => (
            <article key={`${f.component}-${f.code}-${idx}`} className="v4-card">
              <div className="v4-card-head">
                <span className="v4-card-icon"><Icon name="warn" size={18} /></span>
                <div>
                  <h3>{f.component} · {f.code}</h3>
                  <p style={{ margin: 0 }}>{f.message ?? f.description ?? ""}</p>
                </div>
              </div>
              <button
                type="button"
                style={{
                  border: "1px solid var(--accent)",
                  background: "transparent",
                  color: "var(--accent-ink)",
                  padding: "8px 14px",
                  borderRadius: "var(--radius-pill)",
                  fontWeight: 500,
                  cursor: "pointer",
                  fontFamily: "inherit",
                }}
                onClick={() => navigate("/diagnostics/doctor")}
              >
                {v4t(locale, "easyCare.findingOpenAction")} →
              </button>
            </article>
          ))}
        </div>
      )}

      {!healthReachable && (
        <p style={{ marginTop: "var(--space-l)", color: "var(--text-muted)" }}>
          {v4t(locale, "sys.dataStale")}
        </p>
      )}
    </div>
  );
}
