/**
 * WAN pages — uplinks, mobile modems, failover, path quality, DDNS.
 * P017 (WAN Connections) and P019 (Multi-WAN / Failover) wired to the
 * real /overview and /multiwan APIs. Renders V4 chrome only.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Locale } from "../i18n/messages";
import { V4PageFrame } from "../components/PageFrame";
import { v4t } from "../i18n/v4-strings";
import type { Overview } from "../../../shared/api/types";
import type { PageMeta } from "../pages/registry";

function useLocale(): Locale {
  if (typeof document === "undefined") return "en";
  const html = document.documentElement.getAttribute("data-locale");
  if (html === "ru" || html === "de" || html === "es" || html === "en")
    return html;
  const shell = document
    .querySelector(".duck-shell")
    ?.getAttribute("data-locale");
  if (shell === "ru" || shell === "de" || shell === "es" || shell === "en")
    return shell;
  return "en";
}

/* — P017 WAN Connections — */
export function WanConnectionsPage({
  page,
  client,
}: {
  page: PageMeta;
  bundle?: unknown;
  client: APIClient;
}) {
  const locale = useLocale();
  const [overview, setOverview] = useState<FetchResult<Overview> | null>(null);
  const [query, setQuery] = useState("");
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setOverview);
    return () => ctl.abort();
  }, [client]);

  if (!overview) {
    return (
      <V4PageFrame
        page={page}
        locale={locale}
        title={page.name}
        subtitle={page.question}
      >
        <div className="duck-empty">
          <h3>{v4t(locale, "sys.loading")}</h3>
        </div>
      </V4PageFrame>
    );
  }
  if (!overview.ok) {
    return (
      <V4PageFrame
        page={page}
        locale={locale}
        title={page.name}
        subtitle={page.question}
      >
        <div className="duck-empty">
          <h3>
            {overview.reason === "missing"
              ? v4t(locale, "sys.contractMissing")
              : v4t(locale, "sys.error")}
          </h3>
          <p>{overview.message}</p>
        </div>
      </V4PageFrame>
    );
  }
  const data: Overview = overview.data;
  const uplinks: Overview["interfaces"] = data.interfaces;
  const filtered = uplinks.filter(
    (u) =>
      !query ||
      `${u.name} ${u.mac} ${u.state}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  const defaultRoute = data.routes.find(
    (r) => r.destination === "default" || r.destination === "0.0.0.0/0",
  );
  return (
    <V4PageFrame page={page} locale={locale}>
      <div className="v4-page-section">
        <div className="v4-toolbar">
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={v4t(locale, "pageWan.findUplink")}
            className="v4-input-search"
          />
          <span className="v4-subtle v4-fs-xs">
            {filtered.length}{" "}
            {v4t(
              locale,
              "chrome.ofTotal",
              filtered.length,
              uplinks.length,
            ).replace(/^\d+\s*/, "")}
          </span>
        </div>
        {defaultRoute && (
          <p className="v4-mute v4-fs-s">
            {v4t(locale, "pageWan.defaultGateway")}{" "}
            <span className="v4-mono">{defaultRoute.gateway || "—"}</span>{" "}
            {v4t(locale, "pageWan.via")}{" "}
            <span className="v4-mono">{defaultRoute.device || "—"}</span>
          </p>
        )}
        <div className="v4-table-wrap">
          <table className="v4-table">
            <thead>
              <tr>
                <th>{v4t(locale, "pageWan.colInterface")}</th>
                <th>{v4t(locale, "pageWan.colState")}</th>
                <th>{v4t(locale, "pageWan.colIPv4")}</th>
                <th>{v4t(locale, "pageWan.colIPv6")}</th>
                <th>{v4t(locale, "pageWan.colMTU")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={5} className="table-empty">
                    {v4t(locale, "pageWan.noMatch")}
                  </td>
                </tr>
              ) : (
                filtered.map((u) => (
                  <tr key={u.index}>
                    <td>{u.name}</td>
                    <td>
                      <span
                        className="v4-status-pill"
                        data-tone={
                          u.state.toLowerCase() === "up" ? "good" : "warn"
                        }
                      >
                        {u.state}
                      </span>
                    </td>
                    <td className="v4-mono">
                      {u.addresses.find((a) => a.family === "ipv4")?.address ??
                        "—"}
                    </td>
                    <td className="v4-mono">
                      {u.addresses.find((a) => a.family === "ipv6")?.address ??
                        "—"}
                    </td>
                    <td className="v4-mono">{u.mtu}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </V4PageFrame>
  );
}

/* — P019 Multi-WAN / Failover — uses /multiwan endpoint — */
export function MultiwanPage({
  page,
  client,
}: {
  page: PageMeta;
  bundle?: unknown;
  client: APIClient;
}) {
  const locale = useLocale();
  const [result, setResult] = useState<FetchResult<unknown> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchMultiwan({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);

  if (!result) {
    return (
      <V4PageFrame
        page={page}
        locale={locale}
        title={page.name}
        subtitle={page.question}
      >
        <div className="duck-empty">
          <h3>{v4t(locale, "sys.loading")}</h3>
        </div>
      </V4PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <V4PageFrame
        page={page}
        locale={locale}
        title={page.name}
        subtitle={page.question}
      >
        <div className="duck-empty">
          <h3>
            {result.reason === "missing"
              ? v4t(locale, "sys.contractMissing")
              : v4t(locale, "sys.error")}
          </h3>
          <p>{result.message}</p>
        </div>
      </V4PageFrame>
    );
  }
  const data = result.data as {
    uplinks?: Array<Record<string, unknown>>;
    active?: string;
  };
  const uplinks = data.uplinks ?? [];
  return (
    <V4PageFrame page={page} locale={locale}>
      <div className="v4-page-section">
        <div className="v4-card">
          <div className="v4-card-head">
            <h3>{v4t(locale, "pageWan.failoverOrder")}</h3>
            <span className="meta">{data.active ?? "—"}</span>
          </div>
          {uplinks.length === 0 ? (
            <p className="v4-mute">{v4t(locale, "pageWan.noUplinks")}</p>
          ) : (
            <div className="v4-table-wrap">
              <table className="v4-table">
                <thead>
                  <tr>
                    <th>{v4t(locale, "pageWan.colUplink")}</th>
                    <th>{v4t(locale, "pageWan.colRole")}</th>
                    <th>{v4t(locale, "pageWan.colState")}</th>
                    <th>{v4t(locale, "pageWan.colHealth")}</th>
                  </tr>
                </thead>
                <tbody>
                  {uplinks.map((u, i) => (
                    <tr key={i}>
                      <td className="v4-mono">{String(u.name ?? "—")}</td>
                      <td>{String(u.role ?? "—")}</td>
                      <td>
                        <span
                          className="v4-status-pill"
                          data-tone={
                            String(u.state ?? "unknown") === "active"
                              ? "good"
                              : "warn"
                          }
                        >
                          {String(u.state ?? "—")}
                        </span>
                      </td>
                      <td>{String(u.health ?? "—")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </V4PageFrame>
  );
}
