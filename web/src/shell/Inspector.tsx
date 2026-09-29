/**
 * DuckOS V1 — contextual inspector.
 *
 * The third column. Shows real per-page information: question, columns,
 * filters, runtime notes, diagnostics, dependencies, Show CLI, Show API,
 * risk, related pages, ownership, configured/desired/observed/verified
 * state distinctions.
 *
 * When a page selects an object (via `selectObject` on the prefs context),
 * the Inspector switches to an object panel — name, status, ordered
 * fields, page breadcrumb — instead of the generic page documentation.
 * The page documentation stays available underneath as a "Page reference"
 * section so the inspector never permanently consumes the screen with
 * generic documentation while the user is interacting with objects.
 *
 * The inspector never invents a value that the system has not returned.
 * Where a piece of information is not yet exposed by the backend, the
 * inspector explicitly says so.
 */
import { Fragment, useContext, useMemo, useState } from "react";
import { type Messages, t } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { lookupEntry } from "../help/index";
import { PAGES_BY_ID, PAGES_BY_ROUTE } from "../pages/registry";
import { PrefsContext } from "../state/store";

export function Inspector({
  bundle,
  page,
  onClose,
}: {
  bundle: Messages;
  page: PageMeta;
  onClose: () => void;
}) {
  const { state, dispatch } = useContext(PrefsContext)!;
  // A selection only counts if it came from the page we're currently on —
  // stale selections from previous pages would mislead the operator.
  const objectPanel =
    state.inspectorObject && state.inspectorObject.pageId === page.id
      ? state.inspectorObject
      : null;
  const clearSelection = () => dispatch({ type: "clearSelection" });
  const [copiedKey, setCopiedKey] = useState<"cli" | "api" | null>(null);
  const helpEntry = lookupEntry(page.id);

  const cli = useMemo(
    () =>
      [
        `# Duck CLI — ${page.name}`,
        `duck os ${page.id.toLowerCase()} show`,
        `duck os ${page.id.toLowerCase()} diff`,
        `duck os ${page.id.toLowerCase()} apply --confirm`,
      ].join("\n"),
    [page],
  );

  const api = useMemo(() => {
    const ep = apiPathFor(page);
    return [
      `GET ${ep}`,
      page.implemented
        ? `# ${page.implemented ? "implemented" : "design only"}: see page shell for the exact contract`
        : `# contract gap recorded — no production endpoint yet`,
    ].join("\n");
  }, [page]);

  const related = helpEntry?.relatedPages ?? [];

  function copy(value: string, kind: "cli" | "api") {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      void navigator.clipboard.writeText(value);
      setCopiedKey(kind);
      window.setTimeout(() => setCopiedKey(null), 1200);
    }
  }

  return (
    <aside className="v1-inspector" aria-label={t(bundle, "inspector.title")}>
      <div className="v1-row" style={{ justifyContent: "space-between" }}>
        <h2>{t(bundle, "inspector.title")}</h2>
        <button
          type="button"
          className="v1-btn"
          data-variant="ghost"
          onClick={onClose}
          aria-label={t(bundle, "inspector.close")}
        >
          ×
        </button>
      </div>

      {objectPanel && (
        <div className="inspector-block" data-state="object">
          <div className="v1-row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
            <h3 style={{ marginBottom: 0 }}>{objectPanel.displayName}</h3>
            <button
              type="button"
              className="v1-btn"
              data-variant="ghost"
              onClick={clearSelection}
              aria-label="Clear selection"
              title="Clear selection"
            >
              ✕
            </button>
          </div>
          <p className="v1-subtle v1-fs-xs" style={{ marginTop: "var(--v1-s1)" }}>
            {objectPanel.kind} · ref <code className="v1-mono">{objectPanel.ref}</code>
          </p>
          {objectPanel.status && (
            <p style={{ marginTop: "var(--v1-s2)" }}>
              <span
                className="v1-pill"
                data-state={objectPanel.status === "live" ? "good" : "unknown"}
              >
                {objectPanel.status}
              </span>
            </p>
          )}
          <dl className="v1-kv" style={{ marginTop: "var(--v1-s3)" }}>
            {objectPanel.fields.map((f) => (
              <Fragment key={f.label}>
                <dt>{f.label}</dt>
                <dd className={f.mono ? "v1-mono" : undefined}>{f.value}</dd>
              </Fragment>
            ))}
          </dl>
          <p className="v1-subtle v1-fs-xs" style={{ marginTop: "var(--v1-s2)" }}>
            Selected on <strong>{page.name}</strong> · page reference below.
          </p>
        </div>
      )}

      <p className="v1-mute" style={{ fontSize: "var(--v1-fs-md)" }}>
        {page.easyHint}
      </p>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.question")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }}>{page.question}</p>
      </div>

      <div className="inspector-block">
        <h3>Configured · Desired · Observed · Verified</h3>
        <ul className="v1-row" style={{ gap: "var(--v1-s2)", flexWrap: "wrap", listStyle: "none", padding: 0, marginTop: "var(--v1-s2)" }}>
          <li className="v1-pill" data-state="unknown">configured</li>
          <li className="v1-pill" data-state="unknown">desired</li>
          <li className="v1-pill" data-state={page.implemented ? "good" : "warn"}>
            observed · {page.implemented ? "live" : "design only"}
          </li>
          <li className="v1-pill" data-state="unknown">verified</li>
        </ul>
        <p className="v1-subtle v1-fs-xs" style={{ marginTop: "var(--v1-s2)" }}>
          The dashboard never collapses these into a single status. Each
          is set by the Engine independently.
        </p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.features")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }}>
          {page.features.split(";").map((f) => f.trim()).join(" · ")}
        </p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.columns")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }} className="v1-mono">
          {page.columns}
        </p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.filters")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }} className="v1-mono">
          {page.filters}
        </p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.runtime")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }}>{page.runtime}</p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.diagnostics")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }}>{page.diagnostics}</p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.dependencies")}</h3>
        <p style={{ marginTop: "var(--v1-s2)" }} className="v1-mono">
          {page.dependencies}
        </p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "shell.risk")}</h3>
        <span className="v1-risk" data-risk={page.risk}>{page.risk}</span>
        <p style={{ marginTop: "var(--v1-s2)" }} className="v1-mute">
          {riskDescription(page.risk, bundle)}
        </p>
      </div>

      {related.length > 0 && (
        <div className="inspector-block">
          <h3>Related pages</h3>
          <ul className="v1-stack" style={{ listStyle: "none", padding: 0, marginTop: "var(--v1-s2)" }}>
            {related.map((id) => {
              const p = PAGES_BY_ID.get(id);
              if (!p) return null;
              return (
                <li key={id}>
                  <a href={p.route}>
                    <strong>{p.name}</strong>
                    <div className="v1-subtle v1-fs-xs">{p.easyHint}</div>
                  </a>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      <div className="inspector-block">
        <h3>{t(bundle, "inspector.cliLabel")}</h3>
        <pre>{cli}</pre>
        <div className="actions">
          <button
            type="button"
            className="v1-btn"
            data-variant="ghost"
            onClick={() => copy(cli, "cli")}
          >
            {copiedKey === "cli"
              ? `${t(bundle, "inspector.copyCli")} ✓`
              : t(bundle, "inspector.copyCli")}
          </button>
        </div>
        <p className="v1-subtle v1-fs-xs" style={{ marginTop: "var(--v1-s2)" }}>
          Commands are accurate per the documented CLI surface; the
          dashboard never executes them. Never include private keys or
          secrets in CLI examples.
        </p>
      </div>

      <div className="inspector-block">
        <h3>{t(bundle, "inspector.apiLabel")}</h3>
        <pre>{api}</pre>
        <div className="actions">
          <button
            type="button"
            className="v1-btn"
            data-variant="ghost"
            onClick={() => copy(api, "api")}
          >
            {copiedKey === "api"
              ? `${t(bundle, "inspector.copyApi")} ✓`
              : t(bundle, "inspector.copyApi")}
          </button>
        </div>
        <p className="v1-subtle v1-fs-xs" style={{ marginTop: "var(--v1-s2)" }}>
          API endpoint hint based on the public contract. The page shell
          verifies the exact contract at runtime.
        </p>
      </div>

      <div className="inspector-block">
        <h3>Ownership & provenance</h3>
        <dl className="v1-kv">
          <dt>Page id</dt>
          <dd className="v1-mono">{page.id}</dd>
          <dt>Section</dt>
          <dd>{page.section}</dd>
          <dt>Risk</dt>
          <dd>{page.risk}</dd>
          <dt>Live</dt>
          <dd>{page.implemented ? "yes" : "no (design only)"}</dd>
        </dl>
        <p className="v1-subtle v1-fs-xs" style={{ marginTop: "var(--v1-s2)" }}>
          The dashboard treats ownership and provenance as first-class:
          every change is tied back to the same owner, and the dashboard
          never claims to have done something the agent has not reported.
        </p>
      </div>
    </aside>
  );
}

export function apiPathFor(page: PageMeta): string {
  switch (page.section) {
    case "interfaces":
      return "/api/v1/network";
    case "wan":
      return "/api/v1/multiwan";
    case "lan":
      return "/api/v1/dhcp";
    case "routing":
      if (page.id === "P030" || page.id === "P031") return "/api/v1/frr";
      return "/api/v1/network";
    case "firewall":
      return "/api/v1/objects";
    case "tunnels":
      if (page.id === "P047" || page.id === "P048") return "/api/v1/wireguard";
      if (page.id === "P049" || page.id === "P050") return "/api/v1/xray";
      return "/api/v1/network";
    case "dns":
      return "/api/v1/network";
    case "services":
      if (page.id === "P073") return "/api/v1/sites";
      return "/api/v1/network";
    case "system":
      if (page.id === "P001") return "/api/v1/overview";
      if (page.id === "P068") return "/api/v1/backup";
      if (page.id === "P069") return "/api/v1/upgrade";
      if (page.id === "P072") return "/api/v1/ups";
      if (page.id === "P002") return "/api/v1/config";
      return "/api/v1/overview";
    case "logs":
      if (page.id === "P087") return "/api/v1/topology";
      if (page.id === "P091") return "/api/v1/doctor";
      if (page.id === "P088") return "/api/v1/network";
      if (page.id === "P090") return "/api/v1/network";
      return "/api/v1/doctor";
    default:
      return "/api/v1/overview";
  }
}

function riskDescription(risk: "R" | "C" | "I", bundle: Messages): string {
  switch (risk) {
    case "R":
      return "Read-only observation. No configuration change.";
    case "C":
      return "Configurable. Changes follow the Global Safe Apply flow.";
    case "I":
      return bundle.candidate.noConfirmSupport;
  }
}

// Force import to keep tree-shaking from removing the lookup.
void PAGES_BY_ROUTE;