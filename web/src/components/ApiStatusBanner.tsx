/**
 * API status banner — surfaces global health so the operator never
 * silently looks at a stale dashboard.
 *
 * Three modes:
 *   - "all-good"   — overview/doctor/topology all reachable
 *   - "degraded"   — at least one reachable, others are not
 *   - "unreachable" — overview unreachable
 *
 * Always dismissed by a refresh action; never auto-cleared (a fake
 * "all clear" would be the worst kind of success).
 */
import { useContext } from "react";
import { type Messages, t } from "../i18n/messages";
import { PrefsContext } from "../state/store";
import type { APIClient } from "../api/client";

export function ApiStatusBanner({
  bundle,
  client,
}: {
  bundle: Messages;
  client: APIClient;
}) {
  const { state, dispatch } = useContext(PrefsContext)!;
  const { health } = state;
  const known = [health.overview, health.doctor, health.topology, health.readiness];
  const good = known.filter((t) => t === "good").length;
  const bad = known.filter((t) => t === "bad").length;
  const unknown = known.length - good - bad;
  let tone: "all-good" | "degraded" | "unreachable" = "all-good";
  if (bad === known.length) tone = "unreachable";
  else if (bad > 0 || unknown >= 2) tone = "degraded";

  if (tone === "all-good") return null;

  function refresh() {
    void client.fetchOverview({ fallbackToMissing: true });
  }

  return (
    <div className="v1-banner" role="status" aria-live="polite">
      <span className="v1-dot" data-state={tone === "unreachable" ? "bad" : "warn"} />
      <span className="v1-strong">
        {tone === "unreachable"
          ? t(bundle, "health.apiUnreachable")
          : "Some components are stale."}
      </span>
      <span className="v1-subtle">
        {good} reachable · {bad} unreachable · {unknown} unknown
      </span>
      <button type="button" className="v1-btn" data-variant="ghost" onClick={refresh}>
        Refresh
      </button>
      <button
        type="button"
        className="v1-btn"
        data-variant="ghost"
        onClick={() =>
          dispatch({ type: "setHealth", patch: { lastObservedAt: null } })
        }
      >
        Dismiss
      </button>
    </div>
  );
}