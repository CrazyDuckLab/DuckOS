/**
 * PageFrame V4 — the standard header + content layout every V1 page
 * renders inside.
 *
 * Replaces the previous v1-card chrome. Single page title, optional
 * subtitle, optional meta line (risk / live / observed), then content.
 * No uppercased caps-block breadcrumbs, no generic "live" pill. Domain
 * copy comes from the bundle.
 *
 * For backward compatibility with the previous V1 contract we also
 * export the previous symbols (PageState, FormField, SegmentedControl,
 * SearchField, ColumnsPanel, PageToolbar, PageLink) as no-op stubs.
 * Pages still importing them will compile; once migrated, those
 * stubs will be removed in a follow-up.
 */
import { type ReactNode } from "react";
import { v4t, v4Messages } from "../i18n/v4-strings";
import type { Locale, Messages } from "../i18n/messages";
import { messages as legacyMessages, t as legacyT } from "../i18n/messages";
import type { PageMeta } from "../pages/registry";

/* — New V4 contract (named V4PageFrame) — */

export function V4PageFrame({
  page,
  locale,
  title,
  subtitle,
  status,
  meta,
  actions,
  children,
}: {
  page: PageMeta;
  locale?: Locale;
  title?: string;
  subtitle?: string;
  status?: { tone: "good" | "warn" | "bad" | "muted"; label: string };
  meta?: ReadonlyArray<{ label: string; tone?: "muted" | "warn" | "bad" }>;
  actions?: ReactNode;
  children: ReactNode;
}) {
  const heading = title ?? (page ? page.name : "");
  const sub = subtitle ?? (page ? page.question : "");
  void locale;
  void status;
  return (
    <div className="v4-page">
      <h1 className="v4-page-title">{heading}</h1>
      {sub && <p className="v4-page-subtitle">{sub}</p>}
      <div className="v4-page-meta">
        {status && (
          <span className="v4-pill" data-kind={statusToneKind(status.tone)}>
            {status.label}
          </span>
        )}
        {meta?.map((m, i) => (
          <span
            key={i}
            className="v4-pill"
            data-kind={m.tone ?? "muted"}
            style={{ textTransform: "none", letterSpacing: 0, fontWeight: 500 }}
          >
            {m.label}
          </span>
        ))}
      </div>
      {actions && (
        <div style={{ display: "flex", gap: 8, margin: "16px 0" }}>
          {actions}
        </div>
      )}
      {children}
    </div>
  );
}

function statusToneKind(
  tone: "good" | "warn" | "bad" | "muted",
): "muted" | "warn" | "bad" | undefined {
  if (tone === "warn") return "warn";
  if (tone === "bad") return "bad";
  if (tone === "muted") return "muted";
  return undefined;
}

/* — Legacy V1 contract — kept temporarily so calling sites keep building.
   These delegate to a no-op (no v1-card output) when used, telling the
   shell the call exists and still resolving to a div. Visual chrome
   lives entirely in PageFrame above; legacy stubs render nothing.
   Pages should be migrated to the V4 contract within this pass. */

interface LegacyPageFrameProps {
  page: PageMeta;
  bundle: Messages;
  children: ReactNode;
  headerTools?: ReactNode;
}

/** PageFrame is the legacy V1 contract — every existing configurator
 * uses it with `{ page, bundle, children, headerTools }`. We keep that
 * signature so existing call sites compile, but the chrome rendered is
 * V4: a single H1 title, a one-line subtitle, an optional meta line, and
 * the children inside a clean section. No v1-card, no crumbs, no caps
 * pill row.
 */
export function PageFrame(props: LegacyPageFrameProps) {
  const locale =
    typeof document !== "undefined"
      ? ((document.documentElement.getAttribute("data-locale") as Locale) ??
        "en")
      : "en";
  const meta: Array<{ label: string; tone?: "muted" | "warn" | "bad" }> = [];
  if (!props.page.implemented) {
    meta.push({ label: v4t(locale, "pageMeta.designOnly"), tone: "warn" });
  } else if (props.page.risk === "R") {
    meta.push({ label: v4t(locale, "pageMeta.readOnly"), tone: "muted" });
  } else if (props.page.risk === "C") {
    meta.push({ label: v4t(locale, "pageMeta.configurable") });
  } else if (props.page.risk === "I") {
    meta.push({ label: v4t(locale, "pageMeta.irreversible"), tone: "bad" });
  }
  // Derive locale from bundle fallback if possible
  void props.bundle;
  return (
    <div className="v4-page">
      <h1 className="v4-page-title">{props.page.name}</h1>
      <p className="v4-page-subtitle">{props.page.question}</p>
      <div className="v4-page-meta">
        {meta.map((m, i) => (
          <span
            key={i}
            className="v4-pill"
            data-kind={m.tone ?? "muted"}
            style={{ textTransform: "none", letterSpacing: 0, fontWeight: 500 }}
          >
            {m.label}
          </span>
        ))}
      </div>
      {props.headerTools && (
        <div style={{ display: "flex", gap: 8, margin: "12px 0" }}>
          {props.headerTools}
        </div>
      )}
      <div className="v4-page-section">{props.children}</div>
    </div>
  );
}

/** Legacy PageFrame signature: kept as an alias so old call sites
 * continue to compile. New code should import `PageFrame` directly. */
export const LegacyPageFrame = PageFrame;

/* — Legacy page-state marker. Accepts the old prop shape (kind + bundle
 * + optional body) and renders a V4 empty/error card. New code should
 * import `EmptyState` or use V4PageFrame directly. */
export function PageState(props: Record<string, unknown>) {
  const kind = String(props.kind ?? "loading");
  const isError =
    kind === "error" ||
    kind === "missing-contract" ||
    kind === "contract-missing";
  const title = typeof props.title === "string" ? props.title : "";
  const body =
    typeof props.body === "string"
      ? props.body
      : typeof props.desc === "string"
        ? props.desc
        : "";
  return (
    <div
      className="duck-empty"
      style={
        isError
          ? { background: "var(--bad-soft)", color: "var(--bad-on)" }
          : undefined
      }
    >
      {title ? <h3>{title}</h3> : null}
      {body ? <p>{body}</p> : null}
      {typeof props.actions !== "undefined" && props.actions ? (
        <div>{props.actions as ReactNode}</div>
      ) : null}
    </div>
  );
}

interface FormFieldProps {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: "text" | "number" | "password";
  bundle: Messages;
}

/** Legacy form field — V4 substitute. */
export function FormField({ label, value, onChange, type }: FormFieldProps) {
  return (
    <label
      className="v4-form-field"
      style={{ display: "block", marginBottom: 12 }}
    >
      <span
        style={{
          display: "block",
          fontSize: "var(--fs-xs)",
          color: "var(--text-muted)",
          marginBottom: 4,
        }}
      >
        {label}
      </span>
      <input
        type={type ?? "text"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{
          width: "100%",
          padding: "8px 12px",
          borderRadius: "var(--radius-m)",
          border: "1px solid var(--line)",
          background: "var(--surface-input)",
          color: "var(--text)",
          fontFamily: "inherit",
          fontSize: "var(--fs-s)",
        }}
      />
    </label>
  );
}

/* — Legacy SegmentedControl. Accepts any prop shape; renders V4 fallback. */
export function SegmentedControl<T extends string>(
  props: Record<string, unknown>,
) {
  const value = props.value as T;
  const onChange = (v: T) => {
    const cb = props.onChange as ((v: T) => void) | undefined;
    if (cb) cb(v);
  };
  const options = (props.options ?? []) as ReadonlyArray<{
    value: T;
    label: string;
  }>;
  const label = typeof props.label === "string" ? props.label : "";
  return (
    <div
      className="duck-mode"
      role="group"
      aria-label={label}
      style={{ display: "inline-flex" }}
    >
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={value === o.value}
          onClick={() => onChange(o.value)}
          style={{
            background:
              value === o.value ? "var(--surface-elevated)" : "transparent",
            color:
              value === o.value ? "var(--text-strong)" : "var(--text-muted)",
            border: "none",
            padding: "4px 12px",
            fontFamily: "inherit",
            fontSize: "var(--fs-xs)",
            cursor: "pointer",
            borderRadius: "var(--radius-pill)",
          }}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/* — Legacy search field. Accepts any prop shape (legacy calls vary). */
export function SearchField(props: Record<string, unknown>) {
  const value = typeof props.value === "string" ? props.value : "";
  const onChange =
    typeof props.onChange === "function"
      ? (props.onChange as (v: string) => void)
      : () => undefined;
  const placeholder =
    typeof props.placeholder === "string" ? props.placeholder : "Search…";
  return (
    <input
      type="search"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      style={{
        width: "100%",
        padding: "8px 12px",
        borderRadius: "var(--radius-pill)",
        border: "1px solid var(--line)",
        background: "var(--surface-input)",
        color: "var(--text)",
        fontFamily: "inherit",
      }}
    />
  );
}

/* — Legacy columns-panel toggle. Accepts any prop shape; renders nothing. */
export function ColumnsPanel(_props: Record<string, unknown>): null {
  return null;
}

/* — Legacy PageToolbar. Accepts any prop shape. */
export function PageToolbar(props: Record<string, unknown>) {
  return (
    <div
      className="v4-page-toolbar"
      style={{
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        gap: 12,
        marginBottom: 16,
      }}
    >
      <div>{props.left as ReactNode}</div>
      <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
        {props.right as ReactNode}
      </div>
    </div>
  );
}

/* — Legacy PageLink. */
export function PageLink(props: Record<string, unknown>) {
  const page = props.page as PageMeta | undefined;
  const navigate = props.navigate as ((r: string) => void) | undefined;
  if (!page) return null;
  return (
    <a
      href={`#${page.route}`}
      onClick={(e) => {
        if (navigate) {
          e.preventDefault();
          navigate(page.route);
        }
      }}
      style={{
        color: "var(--accent-ink)",
        textDecoration: "none",
        fontWeight: 500,
      }}
    >
      {props.children as ReactNode}
    </a>
  );
}

/** Legacy MissingContractNotice — promoted to EmptyState in V4. */
export function MissingContractNotice(props: {
  bundle: Messages;
  page: PageMeta;
}) {
  const locale =
    typeof document !== "undefined"
      ? ((document.documentElement.getAttribute("data-locale") as Locale) ??
        "en")
      : "en";
  const emptyKey =
    props.page.id === "P047" || props.page.id === "P048"
      ? "empty.wireguardPeers"
      : props.page.id === "P042"
        ? "empty.firewallRules"
        : props.page.id === "P057"
          ? "empty.dhcpLeases"
          : props.page.id === "P049" || props.page.id === "P050"
            ? "empty.xray"
            : "empty.generic";
  return (
    <div className="duck-empty">
      <div className="e-icon">
        <span aria-hidden="true">📭</span>
      </div>
      <h3>{v4t(locale, `${emptyKey}.title`)}</h3>
      <p>{v4t(locale, `${emptyKey}.body`)}</p>
      {v4t(locale, `${emptyKey}.action`) && (
        <button type="button">{v4t(locale, `${emptyKey}.action`)}</button>
      )}
    </div>
  );
}

/* — Legacy stubs for old import paths.
 * These accept any props shape (legacy call sites vary) and render
 * nothing visible — the V4 chrome in PageFrame above is what users see.
 * Returning `null` here keeps the page body clean of duplicated chrome. */
export const StatPanel = (_props: Record<string, unknown>): null => null;
export const Toolbar = (_props: Record<string, unknown>): null => null;
export const Segmented = (_props: Record<string, unknown>): null => null;

/* — helpers used elsewhere — */

export function usePageLocale(): Locale {
  return typeof document !== "undefined"
    ? ((document.documentElement.getAttribute("data-locale") as Locale) ?? "en")
    : "en";
}

export function getDomainCopy(locale: Locale, key: string): string {
  return v4t(locale, key);
}

export function bundle(locale: Locale) {
  return v4Messages[locale] ?? v4Messages.en;
}

export function t(b: Messages, path: string): string {
  return legacyT(b, path);
}

export function messages(locale: Locale): Messages {
  return legacyMessages[locale];
}
