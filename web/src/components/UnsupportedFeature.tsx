/**
 * DuckOS V1 — UnsupportedFeature.
 *
 * A single, intentional page surface used by every catalog entry whose
 * backend contract has not yet landed. Replaces the previous per-area
 * stub components (each of which rendered its own thin copy of the same
 * "Columns" + StatePanel pattern).
 *
 * The page is built from the registry's metadata, so it stays consistent
 * with the design source-of-truth (docs/ux-v1/page-catalog.json):
 *   - the question the page answers;
 *   - the easy-mode hint;
 *   - the columns the table will show when the contract lands;
 *   - the dependencies that block the contract;
 *   - the runtime / diagnostics pointers.
 *
 * The visual goal is "this feature is planned, here is what it would
 * answer and what would unblock it" — never a fake success, never an
 * obvious placeholder.
 */
import type { PageMeta } from "../pages/registry";
import { PageFrame } from "./PageFrame";
import { PageState } from "./PageFrame";
import type { Messages } from "../i18n/messages";

/**
 * Split a comma/semicolon-separated catalog field into trimmed, non-empty
 * entries. The catalog mixes comma and semicolon separators depending on
 * the field; this normalises both into a single shape.
 */
function splitList(raw: string | undefined | null): string[] {
  if (!raw) return [];
  return raw
    .split(/[,;]/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

export interface UnsupportedFeatureProps {
  page: PageMeta;
  bundle: Messages;
  /** Optional override for the "what would unblock this" description. */
  blocker?: string;
  /** Optional endpoint / contract name expected to back this surface. */
  contract?: string;
}

/**
 * Render an intentional "design-only" page for a catalog entry that has
 * no wired configurator. The page is honest: it shows the feature
 * question, the columns the table will eventually show, what is blocking
 * the contract, and the operator-friendly description used by Easy mode.
 */
export function UnsupportedFeature({
  page,
  bundle,
  blocker,
  contract,
}: UnsupportedFeatureProps) {
  const columns = splitList(page.columns);
  const filters = splitList(page.filters);
  const features = splitList(page.features);
  const packages = splitList(page.packages);
  const deps = splitList(page.dependencies);

  return (
    <PageFrame page={page} bundle={bundle}>
      <PageState
        kind="contract-missing"
        bundle={bundle}
        title={`${page.name} — not available in this build`}
        body={
          blocker ??
          "This feature is not available in this DuckOS version yet. The UI stays empty on purpose — no fake data, no fake success. Once the underlying API lands, the table renders the columns below."
        }
        actions={
          contract ? (
            <details className="unsupported-expert">
              <summary>Expert details</summary>
              <code className="v1-mono v1-fs-xs" aria-label="expected endpoint">
                {contract}
              </code>
            </details>
          ) : null
        }
      />

      <div className="v1-card">
        <div className="card-head">
          <h3>What this page answers</h3>
        </div>
        <p className="v1-mute">{page.question}</p>
        {page.easyHint && (
          <p className="v1-fs-sm v1-mute">
            <span className="eyebrow">Easy mode summary · </span>
            {page.easyHint}
          </p>
        )}
      </div>

      {columns.length > 0 && (
        <div className="v1-card">
          <div className="card-head">
            <h3>Planned table columns</h3>
            <span className="meta">{columns.length}</span>
          </div>
          <p className="v1-mute v1-fs-sm">
            When the API returns data, the table will render these columns in
            this order.
          </p>
          <ul className="v1-list-plain">
            {columns.map((c) => (
              <li key={c}>
                <span className="v1-fs-sm">{c}</span>
              </li>
            ))}
          </ul>
          {filters.length > 0 && (
            <>
              <div className="card-head" style={{ marginTop: "var(--v1-s3)" }}>
                <h4>Filters</h4>
                <span className="meta">{filters.length}</span>
              </div>
              <ul className="v1-list-plain">
                {filters.map((f) => (
                  <li key={f}>
                    <span className="v1-fs-sm">{f}</span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}

      <div className="v1-card">
        <div className="card-head">
          <h3>What would unblock this</h3>
        </div>
        {deps.length > 0 && (
          <>
            <p className="v1-mute v1-fs-sm">
              The following dependencies must land before this page can become
              live:
            </p>
            <ul className="v1-list-plain">
              {deps.map((d) => (
                <li key={d}>
                  <code className="v1-mono v1-fs-xs">{d}</code>
                </li>
              ))}
            </ul>
          </>
        )}
        {packages.length > 0 && (
          <p className="v1-fs-sm v1-mute">
            <span className="eyebrow">Backend packages · </span>
            {packages.join(" · ")}
          </p>
        )}
        {features.length > 0 && (
          <p className="v1-fs-sm v1-mute">
            <span className="eyebrow">Planned capabilities · </span>
            {features.join(" · ")}
          </p>
        )}
        {page.diagnostics && (
          <p className="v1-fs-sm v1-mute">
            <span className="eyebrow">Diagnostics · </span>
            {page.diagnostics}
          </p>
        )}
        {page.runtime && (
          <p className="v1-fs-sm v1-mute">
            <span className="eyebrow">Runtime notes · </span>
            {page.runtime}
          </p>
        )}
      </div>
    </PageFrame>
  );
}