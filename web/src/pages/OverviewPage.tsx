/**
 * Pages that wrap live API data into the standard frame.
 *
 * Each of these pages connects to the production API via APIClient. When
 * the API is reachable they render real observation data; when it is not
 * they render an honest degraded/error state. Pages never invent data
 * that the API has not returned.
 */
import { useEffect, useMemo, useState } from "react";
import { APIClient, countFindings, countPending, inferTimeTrust, type FetchResult } from "../api/client";
import { type Messages, t } from "../i18n/messages";
import { usePrefs } from "../state/store";
import { formatBytes, formatUptime } from "../format";
import { PageFrame, PageState, StatPanel, Toolbar, SearchField, ColumnsPanel, MissingContractNotice } from "../components/PageFrame";
import { PAGES_BY_ID, type PageMeta } from "../pages/registry";
import type { Overview } from "../../../shared/api/types";

/* — Overview (P001) — */
export function OverviewPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<Overview> | null>(null);
  const { dispatch } = usePrefs();
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then((r) => {
      setResult(r);
      if (r.ok) {
        // Overview success with issues → warn. Overview success
        // without issues → unknown (recent + clean is NOT
        // verified healthy). Overview failure → bad (handled
        // below). The legacy `HealthSummary.overview` field still
        // records this for legacy consumers, but the centralized
        // derivation in web/src/state/health.ts owns the verdict.
        const issues = r.data.issues.length;
        dispatch({
          type: "setHealth",
          patch: {
            overview: issues > 0 ? "warn" : "unknown",
            lastObservedAt: Date.now(),
            activeIncidents: issues,
          },
        });
      } else if (r.reason !== "missing") {
        dispatch({ type: "setHealth", patch: { overview: "bad" } });
      }
    });
    return () => ctl.abort();
  }, [client, dispatch]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState kind="loading" bundle={bundle} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState
          kind={result.reason === "missing" ? "contract-missing" : "error"}
          bundle={bundle}
          body={result.message}
        />
      </PageFrame>
    );
  }
  const data = result.data;
  const root = data.storage.find((s) => s.mountpoint === "/");
  const cpuPercent = data.cpu.usagePercent;
  const memPercent =
    data.memory.totalBytes > 0
      ? (data.memory.usedBytes / data.memory.totalBytes) * 100
      : null;
  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v1-card" data-tight="true">
        <div className="card-head">
          <h3>{t(bundle, "overview.system")}</h3>
          <span className="meta">{data.environment}</span>
        </div>
        <div className="card-grid">
          <StatPanel
            label={t(bundle, "overview.cpu")}
            value={cpuPercent === null ? "—" : `${cpuPercent.toFixed(1)}%`}
            state={cpuPercent === null ? "unknown" : cpuPercent > 90 ? "bad" : cpuPercent > 70 ? "warn" : "good"}
            hint={`${data.cpu.logicalCores} cores`}
          />
          <StatPanel
            label={t(bundle, "overview.memory")}
            value={memPercent === null ? "—" : formatBytes(data.memory.usedBytes)}
            state={memPercent === null ? "unknown" : memPercent > 90 ? "bad" : memPercent > 75 ? "warn" : "good"}
            hint={`${formatBytes(data.memory.totalBytes)} total`}
          />
          <StatPanel
            label={t(bundle, "overview.storage")}
            value={root ? formatBytes(root.usedBytes) : "—"}
            state={!root ? "unknown" : (root.usedBytes / root.totalBytes) > 0.9 ? "bad" : "good"}
            hint={root ? `${formatBytes(root.availableBytes)} free` : "/"}
          />
          <StatPanel
            label={t(bundle, "overview.uptime")}
            value={formatUptime(data.system.uptimeSeconds)}
            state="good"
            hint={t(bundle, "overview.observed")}
          />
        </div>
      </div>

      <div className="v1-card" data-tight="true">
        <div className="card-head">
          <h3>{t(bundle, "overview.system")}</h3>
        </div>
        <dl className="v1-dl">
          <dt>{t(bundle, "overview.hostname")}</dt>
          <dd>{data.system.hostname || "—"}</dd>
          <dt>{t(bundle, "overview.os")}</dt>
          <dd>{data.system.os || "—"}</dd>
          <dt>{t(bundle, "overview.kernel")}</dt>
          <dd className="v1-mono">{data.system.kernel || "—"}</dd>
          <dt>{t(bundle, "overview.arch")}</dt>
          <dd className="v1-mono">{data.system.architecture || "—"}</dd>
        </dl>
      </div>

      <div className="v1-card">
        <div className="card-head">
          <h3>{t(bundle, "overview.issues")}</h3>
          <span className="meta">{data.issues.length}</span>
        </div>
        {data.issues.length === 0 ? (
          <p className="v1-mute">{t(bundle, "overview.noIssues")}</p>
        ) : (
          <div className="v1-table-scroll">
            <table className="v1-table">
              <thead>
                <tr>
                  <th>Component</th>
                  <th>Code</th>
                  <th>Message</th>
                </tr>
              </thead>
              <tbody>
                {data.issues.map((issue, i) => (
                  <tr key={i}>
                    <td>{issue.component}</td>
                    <td className="v1-mono">{issue.code}</td>
                    <td>{issue.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="v1-card">
        <div className="card-head">
          <h3>{t(bundle, "overview.internetEvidence")}</h3>
        </div>
        <p className="v1-mute">{t(bundle, "overview.noInternetEvidence")}</p>
        <p className="v1-subtle v1-fs-xs">
          The /api/v1/health endpoint is a local health signal, not Internet reachability.
        </p>
      </div>
    </PageFrame>
  );
}

/* — Generic data page that fetches JSON, shows table — */
export function GenericDataPage({
  page,
  bundle,
  client,
  fetch,
  columns,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
  fetch: (c: APIClient, signal: AbortSignal) => Promise<FetchResult<unknown>>;
  columns: Array<{ key: string; label: string; mono?: boolean; render?: (row: Record<string, unknown>) => React.ReactNode }>;
}) {
  const [result, setResult] = useState<FetchResult<unknown> | null>(null);
  const [query, setQuery] = useState("");
  useEffect(() => {
    const ctl = new AbortController();
    fetch(client, ctl.signal).then(setResult);
    return () => ctl.abort();
  }, [client, fetch]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState kind="loading" bundle={bundle} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState
          kind={result.reason === "missing" ? "contract-missing" : result.reason === "unauthorized" ? "permission" : "error"}
          bundle={bundle}
          body={result.message}
        />
      </PageFrame>
    );
  }
  const data = result.data as unknown;
  const rows = useMemo(() => coerceRows(data, columns), [data, columns]);
  const filtered = useMemo(
    () => filterRows(rows, query),
    [rows, query],
  );
  return (
    <PageFrame page={page} bundle={bundle}>
      <Toolbar
        left={<SearchField value={query} onChange={setQuery} placeholder={`Filter ${page.name.toLowerCase()}…`} />}
        right={
          <span className="v1-subtle v1-fs-xs">
            {filtered.length} {filtered.length === 1 ? "row" : "rows"}
          </span>
        }
      />
      <div className="v1-table-wrap">
        <div className="v1-table-scroll">
          <table className="v1-table">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c.key}>{c.label}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={columns.length} className="table-empty">
                    {query ? `No rows match "${query}".` : "No rows."}
                  </td>
                </tr>
              ) : (
                filtered.map((row, idx) => (
                  <tr key={idx}>
                    {columns.map((c) => (
                      <td key={c.key} className={c.mono ? "v1-mono" : undefined}>
                        {c.render ? c.render(row) : String(row[c.key] ?? "—")}
                      </td>
                    ))}
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </PageFrame>
  );
}

function coerceRows(
  data: unknown,
  _columns: Array<{ key: string }>,
): Array<Record<string, unknown>> {
  if (Array.isArray(data)) {
    return data as Array<Record<string, unknown>>;
  }
  if (data && typeof data === "object") {
    const obj = data as Record<string, unknown>;
    for (const key of Object.keys(obj)) {
      if (Array.isArray(obj[key])) {
        return obj[key] as Array<Record<string, unknown>>;
      }
    }
  }
  return [];
}

function filterRows(
  rows: Array<Record<string, unknown>>,
  query: string,
): Array<Record<string, unknown>> {
  if (!query) return rows;
  const q = query.toLowerCase();
  return rows.filter((r) =>
    Object.values(r)
      .filter((v) => v !== null && v !== undefined)
      .map((v) => String(v).toLowerCase())
      .join(" ")
      .includes(q),
  );
}

/* — Doctor page (P091) — */
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
  const { dispatch } = usePrefs();
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchDoctor({ signal: ctl.signal }).then((r) => {
      setResult(r);
      if (r.ok) {
        // Doctor success with findings → warn. Doctor success with
        // empty findings → unknown (not verified healthy). Doctor
        // request failed → bad (handled below). The legacy
        // `HealthSummary.doctor` field still records this so legacy
        // consumers keep working, but the centralized health
        // derivation in web/src/state/health.ts owns the verdict.
        const findings = countFindings(r.data);
        dispatch({
          type: "setHealth",
          patch: {
            doctor: findings > 0 ? "warn" : "unknown",
            timeTrust: inferTimeTrust(r.data),
            activeIncidents: findings,
          },
        });
      } else if (r.reason !== "missing") {
        dispatch({ type: "setHealth", patch: { doctor: "bad" } });
      }
    });
    return () => ctl.abort();
  }, [client, dispatch]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState kind="loading" bundle={bundle} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState
          kind={result.reason === "missing" ? "contract-missing" : "error"}
          bundle={bundle}
          body={result.message}
        />
      </PageFrame>
    );
  }
  const findings = (pickArray(result.data, ["findings", "issues", "results"]) ?? []) as Array<Record<string, unknown>>;
  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v1-card" data-tight="true">
        <div className="card-head">
          <h3>{t(bundle, "health.overall")}</h3>
          <span className="meta">{findings.length} findings</span>
        </div>
        {findings.length === 0 ? (
          <p className="v1-mute">No findings. Doctor considers the system healthy.</p>
        ) : (
          <div className="v1-table-scroll">
            <table className="v1-table">
              <thead>
                <tr>
                  <th>Component</th>
                  <th>Severity</th>
                  <th>Code</th>
                  <th>Message</th>
                </tr>
              </thead>
              <tbody>
                {findings.map((f, i) => (
                  <tr key={i}>
                    <td>{String(f.component ?? "—")}</td>
                    <td>
                      <span className="v1-pill" data-state={severityToState(String(f.severity ?? ""))}>
                        {String(f.severity ?? "info")}
                      </span>
                    </td>
                    <td className="v1-mono">{String(f.code ?? "—")}</td>
                    <td>{String(f.message ?? f.description ?? "—")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </PageFrame>
  );
}

function severityToState(s: string): "good" | "warn" | "bad" | "unknown" {
  const k = s.toLowerCase();
  if (k === "ok" || k === "good" || k === "info") return "good";
  if (k === "warn" || k === "warning") return "warn";
  if (k === "error" || k === "critical" || k === "fatal") return "bad";
  return "unknown";
}

function pickArray(data: unknown, keys: string[]): unknown[] | null {
  if (!data || typeof data !== "object") return null;
  for (const k of keys) {
    const v = (data as Record<string, unknown>)[k];
    if (Array.isArray(v)) return v;
  }
  return null;
}

/* — Multi-WAN (P019) — */
export function MultiwanPage({
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
    client.fetchMultiwan({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState kind="loading" bundle={bundle} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState
          kind={result.reason === "missing" ? "contract-missing" : "error"}
          bundle={bundle}
          body={result.message}
        />
      </PageFrame>
    );
  }
  const uplinks = (pickArray(result.data, ["uplinks", "links", "interfaces"]) ?? []) as Array<Record<string, unknown>>;
  return (
    <PageFrame page={page} bundle={bundle}>
      <ColumnsPanel page={page} />
      <div className="v1-card">
        <div className="card-head">
          <h3>Uplinks</h3>
          <span className="meta">{uplinks.length}</span>
        </div>
        {uplinks.length === 0 ? (
          <p className="v1-mute">No uplinks configured.</p>
        ) : (
          <div className="v1-table-scroll">
            <table className="v1-table">
              <thead>
                <tr>
                  <th>Interface</th>
                  <th>Role</th>
                  <th>State</th>
                  <th>Health</th>
                </tr>
              </thead>
              <tbody>
                {uplinks.map((u, i) => (
                  <tr key={i}>
                    <td className="v1-mono">{String(u.name ?? u.iface ?? "—")}</td>
                    <td>{String(u.role ?? "primary")}</td>
                    <td>
                      <span className="v1-pill" data-state={severityToState(String(u.state ?? "unknown"))}>
                        {String(u.state ?? "unknown")}
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
    </PageFrame>
  );
}

/* — Wi-Fi (P025) — */
export function WifiPage({
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
    client.fetchWifi({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState kind="loading" bundle={bundle} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState
          kind={result.reason === "missing" ? "contract-missing" : "error"}
          bundle={bundle}
          body={result.message}
        />
      </PageFrame>
    );
  }
  const ssids = (pickArray(result.data, ["ssids", "networks", "aps"]) ?? []) as Array<Record<string, unknown>>;
  return (
    <PageFrame page={page} bundle={bundle}>
      <ColumnsPanel page={page} />
      <div className="v1-card">
        <div className="card-head">
          <h3>SSIDs</h3>
          <span className="meta">{ssids.length}</span>
        </div>
        {ssids.length === 0 ? (
          <p className="v1-mute">No Wi-Fi networks configured.</p>
        ) : (
          <div className="v1-table-scroll">
            <table className="v1-table">
              <thead>
                <tr>
                  <th>SSID</th>
                  <th>Band</th>
                  <th>Channel</th>
                  <th>Security</th>
                  <th>Clients</th>
                </tr>
              </thead>
              <tbody>
                {ssids.map((s, i) => (
                  <tr key={i}>
                    <td>{String(s.ssid ?? "—")}</td>
                    <td>{String(s.band ?? "—")}</td>
                    <td className="v1-mono">{String(s.channel ?? "—")}</td>
                    <td>{String(s.security ?? s.auth ?? "—")}</td>
                    <td className="v1-mono">{String(s.clients ?? s.clientCount ?? "0")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </PageFrame>
  );
}

/* — Sites (P073) — */
export function SitesPage({
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
    client.fetchSites({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState kind="loading" bundle={bundle} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <PageState
          kind={result.reason === "missing" ? "contract-missing" : "error"}
          bundle={bundle}
          body={result.message}
        />
      </PageFrame>
    );
  }
  const sites = (pickArray(result.data, ["sites", "peers"]) ?? []) as Array<Record<string, unknown>>;
  return (
    <PageFrame page={page} bundle={bundle}>
      <ColumnsPanel page={page} />
      <div className="v1-card">
        <div className="card-head">
          <h3>Sites</h3>
          <span className="meta">{sites.length}</span>
        </div>
        {sites.length === 0 ? (
          <p className="v1-mute">No sites configured.</p>
        ) : (
          <div className="v1-table-scroll">
            <table className="v1-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Role</th>
                  <th>State</th>
                  <th>Lag</th>
                </tr>
              </thead>
              <tbody>
                {sites.map((s, i) => (
                  <tr key={i}>
                    <td>{String(s.name ?? s.id ?? "—")}</td>
                    <td>{String(s.role ?? "—")}</td>
                    <td>
                      <span className="v1-pill" data-state={severityToState(String(s.state ?? "unknown"))}>
                        {String(s.state ?? "unknown")}
                      </span>
                    </td>
                    <td className="v1-mono">{String(s.lag ?? "—")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </PageFrame>
  );
}

/* — About (P081) — */
export function AboutPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [overview, setOverview] = useState<FetchResult<Overview> | null>(null);
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setOverview);
    return () => ctl.abort();
  }, [client]);
  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v1-card">
        <div className="card-head">
          <h3>System</h3>
        </div>
        {overview?.ok ? (
          <dl className="v1-dl">
            <dt>Hostname</dt>
            <dd>{overview.data.system.hostname}</dd>
            <dt>OS</dt>
            <dd>{overview.data.system.os}</dd>
            <dt>Kernel</dt>
            <dd className="v1-mono">{overview.data.system.kernel}</dd>
            <dt>Architecture</dt>
            <dd className="v1-mono">{overview.data.system.architecture}</dd>
            <dt>Environment</dt>
            <dd className="v1-mono">{overview.data.environment}</dd>
            <dt>Observed</dt>
            <dd className="v1-mono">{overview.data.observedAt}</dd>
          </dl>
        ) : (
          <p className="v1-mute">No system observation available.</p>
        )}
      </div>
      <div className="v1-card">
        <div className="card-head">
          <h3>Build</h3>
        </div>
        <p className="v1-mute">
          DuckOS V1 dashboard. Schema version is what the running API reports;
          a mismatch here means the installed agent needs an update.
        </p>
      </div>
    </PageFrame>
  );
}

/* — Candidate (P002) — */
export function CandidatePage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<unknown> | null>(null);
  const { state, dispatch } = usePrefs();
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchConfig({ signal: ctl.signal, fallbackToMissing: true }).then((r) => {
      setResult(r);
      if (r.ok) {
        dispatch({
          type: "setHealth",
          patch: { pendingChanges: countPending(r.data) },
        });
      }
    });
    return () => ctl.abort();
  }, [client, dispatch]);

  const pending = state.pending;

  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v1-card">
        <div className="card-head">
          <h3>{t(bundle, "candidate.title")}</h3>
          <span className="meta">{pending.length}</span>
        </div>
        {pending.length === 0 && (
          <p className="v1-mute">{t(bundle, "candidate.empty")}</p>
        )}
        {pending.length > 0 && (
          <div className="v1-table-scroll">
            <table className="v1-table">
              <thead>
                <tr>
                  <th>Object</th>
                  <th>Operation</th>
                  <th>Before</th>
                  <th>After</th>
                  <th>Risk</th>
                  <th>Confirm</th>
                </tr>
              </thead>
              <tbody>
                {pending.map((c) => (
                  <tr key={c.id}>
                    <td>{c.title}</td>
                    <td>{c.owner}</td>
                    <td className="v1-mono">{c.before}</td>
                    <td className="v1-mono">{c.after}</td>
                    <td>
                      <span className="v1-risk" data-risk={c.risk}>{c.risk}</span>
                    </td>
                    <td>{c.confirmWindowSec === null ? "— (irreversible)" : `${c.confirmWindowSec}s`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div className="v1-card">
        <div className="card-head">
          <h3>{t(bundle, "shell.diagnostics")}</h3>
        </div>
        {result?.ok ? (
          <p className="v1-mute">Live candidate transaction visible — review before apply.</p>
        ) : result && !result.ok ? (
          <p className="v1-mute">{result.message}</p>
        ) : (
          <p className="v1-mute">Loading…</p>
        )}
      </div>
    </PageFrame>
  );
}

/* — Easy Home (P094) — */
export function EasyHomePage({
  page,
  bundle,
  navigate,
}: {
  page: PageMeta;
  bundle: Messages;
  navigate: (route: string) => void;
}) {
  const tiles = ["P095", "P096", "P097", "P098", "P099"];
  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v1-card" data-tone="easy">
        <div className="card-head">
          <h3>{bundle.easy.question5Title}</h3>
        </div>
        <p className="v1-mute">{bundle.easy.question5Body}</p>
      </div>
      <div className="v1-easy-grid">
        {tiles.map((id) => {
          const p = PAGES_BY_ID.get(id);
          if (!p) return null;
          return (
            <button
              key={p.id}
              type="button"
              className="v1-easy-tile"
              onClick={() => navigate(p.route)}
              aria-label={`${p.easyName ?? p.name} — ${p.easyHint ?? ""}`}
            >
              <div className="v1-easy-tile-id">{p.id}</div>
              <h3>{p.easyName ?? p.name}</h3>
              <p>{p.easyHint}</p>
              <div className="tile-foot">{t(bundle, "shell.easyHint")}</div>
            </button>
          );
        })}
      </div>
    </PageFrame>
  );
}

/* — Generic "design-only" page shell for unimplemented pages.
 * Renders ColumnsPanel + MissingContractNotice + a "tracked" badge so the
 * operator knows the page exists in the spec but not in production. */
export function DesignOnlyPage({
  page,
  bundle,
}: {
  page: PageMeta;
  bundle: Messages;
}) {
  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v1-card" data-tight="true">
        <div className="card-head">
          <h3>Design</h3>
          <span className="v1-pill" data-state="warn">design only</span>
        </div>
        <p className="v1-mute">
          The UX specification for this page is in <code className="v1-mono">docs/ux-v1/page-catalog.json</code> as <code className="v1-mono">{page.id}</code>. The production API does not yet expose the data needed to populate this view.
        </p>
        <div className="v1-row" style={{ gap: "var(--v1-s2)", marginTop: "var(--v1-s2)" }}>
          {page.risk === "C" && <span className="v1-pill">configurable when shipped</span>}
          {page.risk === "I" && <span className="v1-pill" data-state="warn">will be irreversible</span>}
          <span className="v1-pill">{page.columns.split(";").length} columns</span>
          <span className="v1-pill">{page.filters.split(";").length} filters</span>
        </div>
      </div>
      <ColumnsPanel page={page} />
      <MissingContractNotice page={page} bundle={bundle} />
    </PageFrame>
  );
}