/**
 * DuckOS V1 — explainability & evidence panels.
 *
 * Show CLI / Show API / Explain Why / Runtime Evidence / Risk Panel /
 * Validation Summary — all panels the contextual inspector and
 * configurators use.
 */
import { type ReactNode } from "react";
import { t, type Messages } from "../../i18n/messages";

/* — Show CLI — */
export function ShowCliPanel({
  bundle,
  command,
  hint,
}: {
  bundle?: Messages;
  command: string;
  hint?: ReactNode;
}) {
  return (
    <div className="v1-show">
      <div
        className="v1-row"
        style={{ alignItems: "center", gap: "var(--v1-s2)" }}
      >
        <strong>{bundle ? t(bundle, "inspector.cliLabel") : "Duck CLI"}</strong>
        <span className="v1-subtle v1-fs-xs">read-only — does not execute</span>
      </div>
      {hint && <div className="v1-subtle v1-fs-xs">{hint}</div>}
      <pre>{command}</pre>
    </div>
  );
}

/* — Show API — */
export function ShowApiPanel({
  bundle,
  request,
  response,
  hint,
}: {
  bundle?: Messages;
  request: string;
  response?: string;
  hint?: ReactNode;
}) {
  return (
    <div className="v1-show">
      <div
        className="v1-row"
        style={{ alignItems: "center", gap: "var(--v1-s2)" }}
      >
        <strong>
          {bundle ? t(bundle, "inspector.apiLabel") : "Engine / API"}
        </strong>
        <span className="v1-subtle v1-fs-xs">read-only — does not execute</span>
      </div>
      {hint && <div className="v1-subtle v1-fs-xs">{hint}</div>}
      <pre>{request}</pre>
      {response !== undefined && (
        <>
          <div className="v1-subtle v1-fs-xs">response</div>
          <pre>{response}</pre>
        </>
      )}
    </div>
  );
}

/* — Explain Why — */
export function ExplainWhyPanel({
  title = "Explain why",
  children,
}: {
  title?: string;
  children: ReactNode;
}) {
  return (
    <div className="v1-explain-why">
      <h4>{title}</h4>
      <div>{children}</div>
    </div>
  );
}

/* — Runtime evidence — generic key/value with explicit freshness. */
export interface EvidenceRow {
  key: string;
  value: ReactNode;
  mono?: boolean;
  observed?: string;
}

export function RuntimeEvidencePanel({
  title,
  rows,
  observedAt,
}: {
  title?: string;
  rows: EvidenceRow[];
  observedAt?: string;
}) {
  return (
    <div className="v1-evidence">
      {title && <strong className="v1-fs-sm">{title}</strong>}
      {rows.map((r) => (
        <div className="ev-row" key={r.key}>
          <span className="ev-key">{r.key}</span>
          <span className={`ev-val ${r.mono ? "v1-mono" : ""}`}>
            {r.value}
            {r.observed && (
              <span
                className="v1-subtle v1-fs-xs"
                style={{ marginLeft: "var(--v1-s2)" }}
              >
                · {r.observed}
              </span>
            )}
          </span>
        </div>
      ))}
      {observedAt && (
        <div className="ev-row">
          <span className="ev-key">observed</span>
          <span className="ev-val v1-mono v1-fs-xs">{observedAt}</span>
        </div>
      )}
    </div>
  );
}

/* — Risk panel — */
export function RiskPanel({
  risk,
  bundle,
}: {
  risk: "R" | "C" | "I";
  bundle?: Messages;
}) {
  const title =
    risk === "R"
      ? "Read-only observation"
      : risk === "C"
        ? "Configurable change"
        : "Irreversible";
  const body =
    risk === "R"
      ? "This page only reads system state. No configuration is changed."
      : risk === "C"
        ? "Changes follow the Global Safe Apply workflow. You will see them in the dock before they are applied."
        : bundle
          ? t(bundle, "candidate.noConfirmSupport")
          : "No commit-confirmed window — apply is irreversible.";
  return (
    <div className="v1-risk-panel">
      <div className="v1-risk big-risk" data-risk={risk}>
        {risk}
      </div>
      <div>
        <strong>{title}</strong>
        <p>{body}</p>
      </div>
    </div>
  );
}

/* — Validation summary — */
export function ValidationSummary({
  issues,
}: {
  issues: Array<{ key: string; reason: string }>;
}) {
  if (issues.length === 0) return null;
  return (
    <div className="v1-validation" role="alert">
      <h4>
        <span className="v1-dot" data-state="warn" />
        Validation
      </h4>
      <ul>
        {issues.map((i, idx) => (
          <li key={idx}>
            <strong>{i.key}:</strong> {i.reason}
          </li>
        ))}
      </ul>
    </div>
  );
}

/* — compose validators into a summary — */
export function summaryFromValidations(
  errors: Record<string, string | undefined>,
): Array<{ key: string; reason: string }> {
  const out: Array<{ key: string; reason: string }> = [];
  for (const [key, value] of Object.entries(errors)) {
    if (value) out.push({ key, reason: value });
  }
  return out;
}
