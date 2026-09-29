/**
 * Concrete V1 page shells for the pages backed by /api/v1/overview today.
 *
 * This file is intentionally minimal: Router only needs TopologyPage
 * from this module. The other legacy concrete pages (Interfaces,
 * Routing, Dns, Services, Monitoring, Clients) are wired through other
 * components or rendered as design-only stubs from the Router, so the
 * concrete implementations were deleted along with their obsolete
 * legacy chrome (Toolbar / SearchField / Segmented / PageState) calls.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { V4PageFrame } from "../components/PageFrame";
import { v4t } from "../i18n/v4-strings";
import type { Overview } from "../../../shared/api/types";
import type { PageMeta } from "../pages/registry";
import { Topology } from "../Topology";
import { usePrefs } from "../state/store";

function useLocale(): import("../i18n/messages").Locale {
  if (typeof document === "undefined") return "en";
  const html = document.documentElement.getAttribute("data-locale");
  if (html === "ru" || html === "de" || html === "es" || html === "en") return html;
  const shell = document.querySelector(".duck-shell")?.getAttribute("data-locale") ?? null;
  if (shell === "ru" || shell === "de" || shell === "es" || shell === "en") return shell;
  return "en";
}

/* — Topology (P087) — */
export function TopologyPage({
  page,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const locale = useLocale();
  const [overview, setOverview] = useState<FetchResult<Overview> | null>(null);
  // usePrefs (which internally calls useContext) must be hoisted above
  // every early return so React sees the same hook order on every render.
  // We only USE dispatch in the success branch; calling the hook
  // unconditionally here is correct and standard.
  const { dispatch } = usePrefs();
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setOverview);
    return () => ctl.abort();
  }, [client]);
  if (!overview) {
    return (
      <V4PageFrame page={page} locale={locale} title={page.name} subtitle={page.question}>
        <div className="duck-empty">
          <h3>{v4t(locale, "sys.loading")}</h3>
        </div>
      </V4PageFrame>
    );
  }
  if (!overview.ok) {
    return (
      <V4PageFrame page={page} locale={locale} title={page.name} subtitle={page.question}>
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
  return (
    <V4PageFrame page={page} locale={locale}>
      <div className="v4-page-section">
        <Topology
          data={overview.data}
          stale={false}
          onSelectObject={(sel) => {
            dispatch({
              type: "selectObject",
              object: { pageId: page.id, ...sel },
            });
          }}
        />
      </div>
    </V4PageFrame>
  );
}
