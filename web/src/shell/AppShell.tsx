/**
 * DuckOS V1 — AppShell V4.
 *
 * Layout:
 *   topbar     brand · search · tools · end session
 *   rail       primary nav (Easy = 6 plain-language items, Full = grouped by product area)
 *   main       page content
 *   actionbar  quiet strip; only loud when work pending
 *
 * Inspector lives in a right-side drawer, closed by default.
 *
 * Strings live in i18n/v4-strings.ts. Locale falls back to EN silently.
 */
import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import {
  type Locale,
  detectLocale,
  type Messages,
  LOCALES,
} from "../i18n/messages";
import type { InspectorObject } from "../state/store";
import { DuckMark } from "../brand/DuckMark";
import { Icon } from "../icons";
import {
  EASY_NAV,
  FULL_GROUPS,
  pagesBySection,
  PAGES_BY_ID,
  PAGES_BY_ROUTE,
  type PageMeta,
  v4GroupFor,
} from "../pages/registry";
import { searchHelp, type HelpHit } from "../help/index";
import { searchObjects, type ObjectHit } from "../search/objects";
import type { APIClient } from "../api/client";
import type { FetchResult } from "../api/client";
import type { HealthPayload } from "../api/client";
import type { Overview } from "../../../shared/api/types";
import {
  initialPrefs,
  PrefsContext,
  prefsReducer,
  type HealthSummary,
} from "../state/store";
import { v4t } from "../i18n/v4-strings";
import { chipTone, chipLabelKey, deriveHealth } from "../state/health";

export interface AppShellProps {
  token?: string;
  locale?: Locale;
  client: APIClient;
  page?: PageMeta;
  bundle?: Messages;
  route?: string;
  routeTrail?: PageMeta[];
  navigate?: (route: string) => void;
  reloadApi?: () => void;
  signOut?: () => void;
  initialRoute?: string;
  renderRoute?: (route: string, navigate: (r: string) => void) => React.ReactNode;
  children?: React.ReactNode;
}

interface AppState {
  health: HealthSummary;
  /** Latest successful Overview snapshot — fuels global search of real
   *  objects and the Easy-mode "About this device" section. */
  overview: Overview | null;
  /** Raw /api/v1/health fetch result — the topbar chip derives its
   *  tone from this through `deriveHealth`. */
  healthFetch: FetchResult<HealthPayload> | null;
  /** Raw /api/v1/doctor fetch result — needed by the centralized
   *  health derivation in the topbar chip. We keep it separate
   *  from `overview` so Doctor success/failure is distinguishable
   *  from overview success/failure. */
  doctor: FetchResult<unknown> | null;
  /** Raw /api/v1/overview fetch result (success or failure) so the
   *  chip can see observation freshness even when the overview
   *  snapshot itself is null. */
  overviewFetch: FetchResult<Overview> | null;
  pending: ReadonlyArray<{ id: string }>;
  pendingFailed: boolean;
  pendingApplying: boolean;
  pendingAwaitConfirm: boolean;
  searchOpen: boolean;
  searchQuery: string;
  searchFocus: number;
  inspection: boolean;
  inspectorTab: "what" | "why" | "risk" | "example";
  selection: InspectorObject | null;
  theme: "light" | "dark";
  locale: Locale;
}

type Action =
  | { type: "set"; part: Partial<AppState> }
  | { type: "setLocale"; locale: Locale }
  | { type: "setTheme"; theme: "light" | "dark" }
  | { type: "openInspector"; pageId: string }
  | { type: "toggleInspector"; pageId: string }
  | { type: "closeInspector" }
  | { type: "selectObject"; obj: InspectorObject | null }
  | { type: "openSearch" }
  | { type: "closeSearch" }
  | { type: "searchQuery"; q: string }
  | { type: "searchFocus"; i: number }
  | { type: "searchNext" }
  | { type: "searchPrev" }
  | { type: "searchPick"; hits: HelpHit[] }
  | { type: "setHealth"; patch: Partial<HealthSummary> }
  | { type: "setHealthFetch"; healthFetch: FetchResult<HealthPayload> }
  | { type: "setOverview"; overview: Overview }
  | { type: "setOverviewFetch"; overviewFetch: FetchResult<Overview> }
  | { type: "setDoctor"; doctor: FetchResult<unknown> };

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case "set":
      return { ...state, ...action.part };
    case "setLocale":
      return { ...state, locale: action.locale };
    case "setTheme":
      return { ...state, theme: action.theme };
    case "openInspector":
      return {
        ...state,
        inspection: true,
        inspectorTab: "what",
      };
    case "toggleInspector":
      // Toggle: open if closed or for a different page, close
      // if it is already showing.
      if (state.inspection) {
        return { ...state, inspection: false, selection: null };
      }
      return { ...state, inspection: true, inspectorTab: "what" };
    case "closeInspector":
      return { ...state, inspection: false, selection: null };
    case "selectObject":
      return {
        ...state,
        selection: action.obj,
        inspection: action.obj ? true : state.inspection,
      };
    case "openSearch":
      return { ...state, searchOpen: true, searchQuery: "", searchFocus: 0 };
    case "closeSearch":
      return { ...state, searchOpen: false, searchQuery: "", searchFocus: 0 };
    case "searchQuery":
      return { ...state, searchQuery: action.q, searchFocus: 0 };
    case "searchFocus":
      return { ...state, searchFocus: action.i };
    case "searchNext":
      return { ...state, searchFocus: state.searchFocus + 1 };
    case "searchPrev":
      return { ...state, searchFocus: Math.max(0, state.searchFocus - 1) };
    case "setHealth":
      return { ...state, health: { ...state.health, ...action.patch } };
    case "setOverview":
      return { ...state, overview: action.overview };
    case "setOverviewFetch":
      return { ...state, overviewFetch: action.overviewFetch };
    case "setHealthFetch":
      return { ...state, healthFetch: action.healthFetch };
    case "setDoctor":
      return { ...state, doctor: action.doctor };
    default:
      return state;
  }
}

const initial: AppState = {
  health: {
    overview: "unknown",
    readiness: "unknown",
    doctor: "unknown",
    topology: "unknown",
    reachable: false,
    lastObservedAt: null,
    timeTrust: "unknown",
    activeIncidents: 0,
    pendingChanges: 0,
  },
  overview: null,
  overviewFetch: null,
  healthFetch: null,
  doctor: null,
  pending: [],
  pendingFailed: false,
  pendingApplying: false,
  pendingAwaitConfirm: false,
  searchOpen: false,
  searchQuery: "",
  searchFocus: 0,
  inspection: false,
  inspectorTab: "what",
  selection: null,
  theme:
    typeof document !== "undefined" &&
    document.documentElement.getAttribute("data-theme") === "light"
      ? "light"
      : "dark",
  locale: detectLocale(),
};

function setHtmlTheme(t: "light" | "dark"): void {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-theme", t);
}

function setHtmlLocale(l: Locale): void {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-locale", l);
}

function persistPrefs(p: { theme: "light" | "dark"; locale: Locale }): void {
  try {
    window.localStorage.setItem(
      "duckos.v1.prefs.v1",
      JSON.stringify(p),
    );
  } catch {
    /* swallow */
  }
}

export function AppShell(props: AppShellProps) {
  const {
    client,
    page: pageProp,
    route: routeProp,
    navigate: navigateProp,
    reloadApi: _reloadApi = () => undefined,
    signOut = () => undefined,
    children,
    initialRoute,
    renderRoute,
  } = props;

  const [innerRoute, setInnerRoute] = useState<string>(
    initialRoute ?? routeProp ?? "/system/overview",
  );
  const navigate = navigateProp ?? ((r: string) => {
    setInnerRoute(r);
    if (typeof window !== "undefined") {
      window.location.hash = "#" + r;
    }
  });

  /* listen to hash changes when we're self-managing the route */
  useEffect(() => {
    if (navigateProp) return; // parent owns navigation
    function readHash() {
      const h = window.location.hash || "";
      const r = h.startsWith("#") ? h.slice(1) : "/system/overview";
      if (r !== innerRoute) setInnerRoute(r);
    }
    readHash();
    window.addEventListener("hashchange", readHash);
    return () => window.removeEventListener("hashchange", readHash);
  }, [navigateProp, innerRoute]);

  const route = routeProp ?? innerRoute;
  const page = pageProp ?? PAGES_BY_ROUTE.get(route) ?? PAGES_BY_ID.get("P001")!;
  // Navigation clears any Inspector selection: keeping a stale
  // reference would render the wrong object on the next page.
  // The Inspector itself stays open across pages (operator may
  // want to keep it pinned), but the row selection is dropped.
  const lastPageIdRef = useRef<string | null>(null);
  useEffect(() => {
    if (lastPageIdRef.current && lastPageIdRef.current !== page.id) {
      dispatch({ type: "selectObject", obj: null });
    }
    lastPageIdRef.current = page.id;
  }, [page.id]);

  const [state, dispatch] = useReducer(reducer, initial);

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem("duckos.v1.prefs.v1");
      if (!raw) return;
      const persisted = JSON.parse(raw) as {
        theme?: "light" | "dark";
        locale?: Locale;
      };
      if (persisted.theme === "light" || persisted.theme === "dark") {
        dispatch({ type: "setTheme", theme: persisted.theme });
        setHtmlTheme(persisted.theme);
      }
      if (
        persisted.locale === "en" ||
        persisted.locale === "ru" ||
        persisted.locale === "de" ||
        persisted.locale === "es"
      ) {
        dispatch({ type: "setLocale", locale: persisted.locale });
        setHtmlLocale(persisted.locale);
      }
    } catch {
      /* ignore corrupted prefs */
    }
  }, []);

  const locale = state.locale;
  const theme = state.theme;
  const mode: "easy" | "full" =
    page.section === "easy" ? "easy" : "full";

  /* health chip */
  useEffect(() => {
    let cancelled = false;
    // The production /api/v1/health payload is just
    // `{ status, schemaVersion }`. It proves only API reachability at
    // its own level — it does NOT carry overall tone, time trust,
    // observation freshness, or per-component health. We therefore do
    // NOT synthesize a "good" tone from a missing `overall` field.
    // The health chip stays in the neutral "incomplete" state until a
    // dedicated endpoint (overview / doctor) provides real evidence.
    //
    // We store BOTH the legacy HealthSummary patch (so legacy
    // consumers keep working) and the raw FetchResult (so the
    // centralized `deriveHealth` helper in `web/src/state/health.ts`
    // sees the truth and produces the chip tone).
    client.fetchHealth({}).then((r) => {
      if (cancelled) return;
      dispatch({ type: "setHealthFetch", healthFetch: r });
      if (!r.ok) {
        // Request failed: surface that the API is unreachable. Tone
        // stays "unknown" so the chip does not claim Healthy.
        dispatch({
          type: "setHealth",
          patch: { reachable: false, lastObservedAt: null },
        });
        return;
      }
      const reachable = r.data.status === "ok";
      dispatch({
        type: "setHealth",
        patch: {
          reachable,
          // Production payload carries no observedAt. We deliberately
          // leave lastObservedAt / timeTrust / tone as-is so the
          // HealthChip logic can decide whether evidence is enough to
          // claim anything other than "incomplete".
        },
      });
    });
    return () => {
      cancelled = true;
    };
  }, [client]);

  /* /overview feeds the global object search (real interfaces, IPs,
     services, hostname). Fetch once at boot — search is best-effort
     and silently degrades to page-search if the overview is missing. */
  useEffect(() => {
    let cancelled = false;
    client.fetchOverview({}).then((r) => {
      if (cancelled) return;
      // Store BOTH the parsed Overview (for search + About) and the
      // raw FetchResult (for the centralized health derivation in
      // the topbar chip — it needs to see success/failure
      // separately from the parsed payload).
      dispatch({ type: "setOverviewFetch", overviewFetch: r });
      if (r.ok) {
        dispatch({ type: "setOverview", overview: r.data });
      }
    });
    return () => {
      cancelled = true;
    };
  }, [client]);

  /* /doctor feeds the topbar chip's centralized health derivation.
     We store the RAW FetchResult so the chip can distinguish Doctor
     error from Doctor success-empty — Doctor error is NOT zero
     findings. */
  useEffect(() => {
    let cancelled = false;
    client.fetchDoctor({}).then((r) => {
      if (cancelled) return;
      dispatch({ type: "setDoctor", doctor: r });
    });
    return () => {
      cancelled = true;
    };
  }, [client]);

  /* keyboard shortcuts: ⌘K opens search, ? opens inspector when not in a field */
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null;
      const inField =
        !!target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable);
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        dispatch({ type: "openSearch" });
      } else if (e.key === "Escape") {
        // Priority: Inspector first (if open), then search.
        if (state.inspection) {
          dispatch({ type: "closeInspector" });
        } else {
          dispatch({ type: "closeSearch" });
        }
      } else if (e.key === "?" && !inField) {
        // Toggle: closing the Inspector via `?` matches the
        // same key the operator used to open it.
        dispatch({ type: "toggleInspector", pageId: page.id });
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [page.id, state.inspection]);

  const changeLocale = useCallback((next: Locale) => {
    dispatch({ type: "setLocale", locale: next });
    setHtmlLocale(next);
    persistPrefs({ theme: state.theme, locale: next });
  }, [state.theme]);

  const changeTheme = useCallback(() => {
    const next = theme === "dark" ? "light" : "dark";
    dispatch({ type: "setTheme", theme: next });
    setHtmlTheme(next);
    persistPrefs({ theme: next, locale });
  }, [theme, locale]);

  const onSelectRoute = useCallback(
    (r: string) => {
      navigate(r);
      dispatch({ type: "closeSearch" });
    },
    [navigate],
  );

  const prefsState = useReducer(prefsReducer, initialPrefs);

  return (
    <PrefsContext.Provider value={{ state: prefsState[0], dispatch: prefsState[1] }}>
    <div
      className="duck-shell"
      data-mode={mode}
      data-theme={theme}
      data-locale={locale}
    >
      <a href="#duck-main" className="duck-skip">{v4t(locale, "shell.skipToMain")}</a>

      <TopBar
        locale={locale}
        theme={theme}
        mode={mode}
        page={page}
        healthFetch={state.healthFetch}
        doctorFetch={state.doctor}
        overviewFetch={state.overviewFetch}
        onOpenSearch={() => dispatch({ type: "openSearch" })}
        onOpenInspector={() => dispatch({ type: "openInspector", pageId: page.id })}
        onToggleTheme={changeTheme}
        onChangeLocale={changeLocale}
        onSetMode={(m) => {
          const target =
            m === "easy" ? "/easy/home" : "/system/overview";
          onSelectRoute(target);
        }}
        onSignOut={signOut}
        onSelectRoute={onSelectRoute}
      />

      <SideRail
        locale={locale}
        mode={mode}
        page={page}
        onSelectRoute={onSelectRoute}
      />

      <main className="duck-main" id="duck-main" tabIndex={-1}>
        {renderRoute ? renderRoute(route, navigate) : children}
      </main>

      <ActionBar
        locale={locale}
        pending={state.pending}
        observationStale={state.health.timeTrust !== "trusted"}
        failed={state.pendingFailed}
        applying={state.pendingApplying}
        awaitConfirm={state.pendingAwaitConfirm}
        page={page}
        onApply={() => dispatch({ type: "set", part: {} })}
        onDiscard={() => dispatch({ type: "set", part: {} })}
      />

      {/* Easy-mode horizontal nav for ≤920px */}
      {mode === "easy" && (
        <nav className="duck-easytop" aria-label={v4t(locale, "shell.primaryNav")}>
          {EASY_NAV.map((item) => {
            // item.key is "easy-home", "easy-internet"... map to "easy.home", "easy.internet"
            const i18nKey = item.key.replace("-", ".");
            return (
              <a
                key={item.key}
                href={`#${item.route}`}
                className="duck-easytop-item"
                aria-current={page.route === item.route ? "page" : undefined}
                onClick={(e) => {
                  e.preventDefault();
                  onSelectRoute(item.route);
                }}
              >
                <Icon name={item.icon} size={16} />
                <span>{v4t(locale, i18nKey)}</span>
              </a>
            );
          })}
        </nav>
      )}

      <SearchOverlay
        locale={locale}
        open={state.searchOpen}
        query={state.searchQuery}
        focus={state.searchFocus}
        overview={state.overview}
        onQuery={(q) => dispatch({ type: "searchQuery", q })}
        onPick={(r) => onSelectRoute(r.route)}
        onPickObject={(o) => {
          // Selecting a real object routes to its page and primes
          // the Inspector with the row so the user lands on context.
          if (o.ref) {
            dispatch({ type: "selectObject", obj: {
              pageId: routeToPageId(o.route),
              kind: o.ref.kind,
              ref: o.ref.id,
              displayName: o.displayName,
              status: undefined,
              fields: [],
            } as InspectorObject });
          }
          dispatch({ type: "closeSearch" });
          onSelectRoute(o.route);
        }}
        onClose={() => dispatch({ type: "closeSearch" })}
        onNext={() => dispatch({ type: "searchNext" })}
        onPrev={() => dispatch({ type: "searchPrev" })}
      />

      <InspectorDrawer
        locale={locale}
        open={state.inspection}
        page={page}
        selection={state.selection}
        tab={state.inspectorTab}
        onTab={(t) => dispatch({ type: "set", part: { inspectorTab: t } })}
        onClose={() => dispatch({ type: "closeInspector" })}
        onClearSelection={() => dispatch({ type: "selectObject", obj: null })}
      />
    </div>
    </PrefsContext.Provider>
  );
}

/* — Topbar — */

function TopBar(props: {
  locale: Locale;
  theme: "light" | "dark";
  mode: "easy" | "full";
  page: PageMeta;
  healthFetch: FetchResult<HealthPayload> | null;
  doctorFetch: FetchResult<unknown> | null;
  overviewFetch: FetchResult<Overview> | null;
  onOpenSearch: () => void;
  onOpenInspector: () => void;
  onToggleTheme: () => void;
  onChangeLocale: (l: Locale) => void;
  onSetMode: (m: "easy" | "full") => void;
  onSignOut: () => void;
  onSelectRoute: (r: string) => void;
}) {
  const { locale } = props;
  return (
    <header className="duck-topbar" role="banner">
      <a
        className="duck-brand"
        href="#/system/overview"
        onClick={(e) => {
          e.preventDefault();
          props.onSelectRoute("/system/overview");
        }}
        aria-label="DuckOS home"
      >
        <span className="duck-mark" aria-hidden="true">
          <DuckMark size={30} />
        </span>
        <span>
          DuckOS
          <small>V1</small>
        </span>
      </a>

      <div className="duck-search-slot">
        <button
          type="button"
          className="duck-search"
          onClick={props.onOpenSearch}
          style={{ background: "transparent", border: "none", padding: 0, width: "min(560px, 100%)" }}
          aria-label={v4t(locale, "topbar.openSearchHint")}
        >
          <span
            style={{
              display: "flex",
              alignItems: "center",
              width: "100%",
              height: "40px",
              padding: "0 16px 0 40px",
              background: "var(--surface-input)",
              border: "1px solid var(--line)",
              borderRadius: "var(--radius-pill)",
              color: "var(--text-subtle)",
              fontSize: "var(--fs-s)",
              position: "relative",
            }}
          >
            <span className="search-icon" aria-hidden="true">
              <Icon name="search" size={16} />
            </span>
            <span style={{ marginLeft: 8 }}>{v4t(locale, "topbar.openSearchHint")}</span>
            <span className="kbd" aria-hidden="true">⌘K</span>
          </span>
        </button>
      </div>

      <div className="duck-tools" role="toolbar" aria-label={v4t(locale, "shell.primaryNav")}>
        <HealthChip
          locale={locale}
          health={props.healthFetch}
          doctor={props.doctorFetch}
          overview={props.overviewFetch}
        />

        <div className="duck-mode" role="group" aria-label={v4t(locale, "topbar.modeEasy")}>
          <button
            type="button"
            aria-pressed={props.mode === "easy"}
            onClick={() => props.onSetMode("easy")}
          >
            {v4t(locale, "topbar.modeEasy")}
          </button>
          <button
            type="button"
            aria-pressed={props.mode === "full"}
            onClick={() => props.onSetMode("full")}
          >
            {v4t(locale, "topbar.modeFull")}
          </button>
        </div>

        <button
          type="button"
          className="duck-tool"
          aria-label={v4t(locale, "topbar.openInspector")}
          onClick={props.onOpenInspector}
        >
          <Icon name="help" size={18} />
          <span className="duck-tooltip">{v4t(locale, "topbar.openInspector")}</span>
        </button>

        <button
          type="button"
          className="duck-tool"
          aria-label={v4t(locale, "topbar.themeToggle")}
          onClick={props.onToggleTheme}
        >
          <Icon name={props.theme === "dark" ? "sun" : "moon"} size={18} />
          <span className="duck-tooltip">{v4t(locale, "topbar.themeToggle")}</span>
        </button>

        <label className="duck-tool" aria-label={v4t(locale, "topbar.localeSelect")}>
          <Icon name="globe" size={18} />
          <span className="duck-tooltip">{v4t(locale, "topbar.localeSelect")}</span>
          <select
            aria-label={v4t(locale, "topbar.localeSelect")}
            value={props.locale}
            onChange={(e) => props.onChangeLocale(e.target.value as Locale)}
          >
            {LOCALES.map((l) => (
              <option key={l.id} value={l.id}>
                {l.label}
              </option>
            ))}
          </select>
        </label>

        <button type="button" className="duck-end-session" onClick={props.onSignOut}>
          {v4t(locale, "topbar.endSession")}
        </button>
      </div>
    </header>
  );
}

/* — Health chip — */

/**
 * Topbar status chip. The chip is the single most-visible health
 * surface in the product; its tone MUST come from the centralized
 * `deriveHealth` helper in `web/src/state/health.ts`. Pages must
 * NOT have their own verdict derivation — they all funnel through
 * this same helper, so a change to the rules lands everywhere.
 *
 * Inputs:
 *   - `health`      raw /api/v1/health FetchResult (or null while loading)
 *   - `doctor`      raw /api/v1/doctor FetchResult (or null while loading)
 *   - `overview`    raw /api/v1/overview FetchResult (or null while loading)
 */
export function HealthChip({
  locale,
  health,
  doctor,
  overview,
}: {
  locale: Locale;
  health: FetchResult<HealthPayload> | null;
  doctor: FetchResult<unknown> | null;
  overview: FetchResult<Overview> | null;
}) {
  // Single source of truth — same helper as Easy Home, Easy Care,
  // and System Overview. We never duplicate the derivation logic
  // here.
  const derived = deriveHealth({ health, doctor, overview });
  const tone = chipTone(derived);
  const label = v4t(locale, chipLabelKey(derived));
  return (
    <span className="duck-health-chip" data-tone={tone} title={label}>
      <span className="dot" aria-hidden="true" />
      {label}
    </span>
  );
}

/* — SideRail — */

function SideRail({
  locale,
  mode,
  page,
  onSelectRoute,
}: {
  locale: Locale;
  mode: "easy" | "full";
  page: PageMeta;
  onSelectRoute: (r: string) => void;
}) {
  if (mode === "easy") {
    return (
      <nav
        className="duck-rail"
        aria-label={v4t(locale, "shell.primaryNav")}
        style={{ display: "none" }}
      >
        <span aria-hidden="true" />
      </nav>
    );
  }

  return (
    <nav className="duck-rail" aria-label={v4t(locale, "shell.primaryNav")}>
      {FULL_GROUPS.map((g) => {
        const pages: PageMeta[] = [];
        for (const sid of g.sectionIds) {
          pages.push(...pagesBySection(sid));
        }
        if (pages.length === 0) return null;
        return (
          <div className="duck-rail-group" key={g.key}>
            <div className="duck-rail-group-label">{v4t(locale, `rail.${g.key}`)}</div>
            {pages.map((p) => (
              <button
                key={p.id}
                type="button"
                className="duck-rail-item"
                aria-current={page.id === p.id ? "page" : undefined}
                onClick={() => onSelectRoute(p.route)}
                title={p.name}
              >
                <span className="duck-rail-icon">
                  <Icon name={p.icon} size={18} />
                </span>
                <span className="duck-rail-name">{p.name}</span>
              </button>
            ))}
          </div>
        );
      })}
    </nav>
  );
}

/* — Search overlay — */

function SearchOverlay({
  locale,
  open,
  query,
  focus,
  overview,
  onQuery,
  onPick,
  onPickObject,
  onClose,
  onNext,
  onPrev,
}: {
  locale: Locale;
  open: boolean;
  query: string;
  focus: number;
  overview: Overview | null;
  onQuery: (q: string) => void;
  onPick: (p: PageMeta) => void;
  onPickObject: (h: ObjectHit) => void;
  onClose: () => void;
  onNext: () => void;
  onPrev: () => void;
}) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const results: HelpHit[] = useMemo(() => {
    if (!open) return [];
    if (query.trim().length < 2) return [];
    return searchHelp(query, 20);
  }, [open, query]);

  const objectResults: ObjectHit[] = useMemo(() => {
    if (!open) return [];
    if (query.trim().length < 2) return [];
    return searchObjects(query, overview);
  }, [open, query, overview]);

  // resolve to PageMeta for navigation
  const pages: PageMeta[] = useMemo(() => {
    return results
      .map((h) => PAGES_BY_ID.get(h.pageId))
      .filter((x): x is PageMeta => Boolean(x));
  }, [results]);

  const total = pages.length + objectResults.length;

  useEffect(() => {
    if (open) {
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  function onKey(e: React.KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      onNext();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      onPrev();
    } else if (e.key === "Enter") {
      // Pages first, then objects — most common workflow.
      if (focus < pages.length) {
        const p = pages[focus];
        if (p) onPick(p);
      } else {
        const o = objectResults[focus - pages.length];
        if (o) onPickObject(o);
      }
    }
  }

  if (!open) return null;
  return (
    <div className="duck-search-overlay" role="dialog" aria-modal="true">
      <div className="scrim" onClick={onClose} />
      <div className="panel" onKeyDown={onKey}>
        <div style={{ padding: "var(--space-s) var(--space-l) var(--space-2xs)" }}>
          <div style={{ position: "relative" }}>
            <span
              style={{
                position: "absolute",
                top: "50%",
                left: 12,
                transform: "translateY(-50%)",
                color: "var(--text-subtle)",
              }}
            >
              <Icon name="search" size={16} />
            </span>
            <input
              ref={inputRef}
              value={query}
              onChange={(e) => onQuery(e.target.value)}
              placeholder={v4t(locale, "search.placeholder")}
              style={{
                width: "100%",
                padding: "10px 36px 10px 36px",
                background: "var(--surface-input)",
                border: "1px solid var(--line)",
                borderRadius: "var(--radius-m)",
                color: "var(--text)",
                fontSize: "var(--fs-m)",
                outline: "none",
              }}
            />
          </div>
        </div>
        {query.trim().length < 2 && (
          <div style={{ padding: "var(--space-m) var(--space-l)", color: "var(--text-subtle)" }}>
            {v4t(locale, "search.tooShort")} — {v4t(locale, "search.tooShortHint")}
          </div>
        )}
        {query.trim().length >= 2 && total === 0 && (
          <div style={{ padding: "var(--space-m) var(--space-l)", color: "var(--text-subtle)" }}>
            <strong style={{ color: "var(--text)" }}>{v4t(locale, "search.empty")}</strong>
            <div style={{ marginTop: 4 }}>{v4t(locale, "search.emptyHint")}</div>
          </div>
        )}
        {query.trim().length >= 2 && objectResults.length > 0 && (
          <>
            <div className="group-head">{v4t(locale, "search.headingObjects")}</div>
            {objectResults.map((o, idx) => {
              const realIdx = pages.length + idx;
              return (
                <button
                  key={`${o.kind}:${o.id}`}
                  type="button"
                  className="result"
                  aria-selected={realIdx === focus}
                  onClick={() => onPickObject(o)}
                  onMouseEnter={() => onQuery(query)}
                >
                  <span className="r-icon">
                    <Icon name={iconForObjectKind(o.kind)} size={18} />
                  </span>
                  <span>
                    <div className="r-title">{o.displayName}</div>
                    <div className="r-meta">{o.context}</div>
                  </span>
                  <Icon name="arrow" size={14} />
                </button>
              );
            })}
          </>
        )}
        {query.trim().length >= 2 && pages.length > 0 && (
          <>
            <div className="group-head">{v4t(locale, "search.headingPages")}</div>
            {pages.map((p, idx) => {
              const sectionLabel = sectionBreadcrumbLabel(p, locale);
              return (
                <button
                  key={p.id}
                  type="button"
                  className="result"
                  aria-selected={idx === focus}
                  onClick={() => onPick(p)}
                  onMouseEnter={() => onQuery(query)}
                >
                  <span className="r-icon">
                    <Icon name={p.icon} size={18} />
                  </span>
                  <span>
                    <div className="r-title">{p.name}</div>
                    <div className="r-meta">{sectionLabel}</div>
                  </span>
                  <Icon name="arrow" size={14} />
                </button>
              );
            })}
          </>
        )}
        {query.trim().length >= 2 && total > 0 && (
          <div className="hint">
            {v4t(locale, "search.resultsHint", total)}
          </div>
        )}
      </div>
    </div>
  );
}

function iconForObjectKind(kind: "interface" | "address" | "vlan" | "device" | "service"): string {
  if (kind === "interface") return "ethernet";
  if (kind === "address") return "resolver";
  if (kind === "vlan") return "vlan";
  if (kind === "device") return "system";
  return "services";
}

/** Map a route (e.g. "/interfaces") to its PageMeta id (e.g. "P007").
 *  Used to set Inspector pageId so the Inspector can render the right
 *  per-page contextual blocks when the user selects an object hit. */
function routeToPageId(route: string): string {
  for (const p of PAGES_BY_ROUTE.values()) {
    if (p.route === route) return p.id;
  }
  return "";
}

function sectionBreadcrumbLabel(p: PageMeta, _locale: Locale): string {
  return v4t(_locale, `rail.${v4GroupFor(p.section) ?? "system"}`) + " › " + p.section;
}

/* — ActionBar — */

function ActionBar({
  locale,
  pending,
  observationStale,
  failed,
  applying,
  awaitConfirm,
  page,
  onApply,
  onDiscard,
}: {
  locale: Locale;
  pending: ReadonlyArray<{ id: string }>;
  observationStale: boolean;
  failed: boolean;
  applying: boolean;
  awaitConfirm: boolean;
  page: PageMeta;
  onApply: () => void;
  onDiscard: () => void;
}) {
  const status = page.section === "easy" ? "easy" : "full";
  let visualState: "idle-quiet" | "idle-stale" | "pending" | "applying" | "confirm" | "failed" = "idle-quiet";
  let message = v4t(locale, "actionbar.idleQuiet");
  let primary: { label: string; disabled: boolean } | null = null;
  let secondary: { label: string } | null = null;
  if (failed) {
    visualState = "failed";
    message = v4t(locale, "actionbar.failed");
    primary = { label: v4t(locale, "actionbar.failedRecovery"), disabled: false };
  } else if (applying) {
    visualState = "applying";
    message = v4t(locale, "actionbar.applying");
    primary = { label: v4t(locale, "actionbar.applying"), disabled: true };
  } else if (awaitConfirm) {
    visualState = "confirm";
    message = v4t(locale, "actionbar.confirmRequired", 30);
    primary = { label: v4t(locale, "actionbar.confirm"), disabled: false };
    secondary = { label: v4t(locale, "actionbar.rollback") };
  } else if (pending.length > 0) {
    visualState = "pending";
    message = v4t(locale, "actionbar.pending", pending.length);
    primary = { label: v4t(locale, "actionbar.apply"), disabled: observationStale };
    secondary = { label: v4t(locale, "actionbar.discard") };
  } else if (observationStale) {
    visualState = "idle-stale";
    message = v4t(locale, "actionbar.idleStale");
  } else {
    visualState = "idle-quiet";
    message = v4t(locale, "actionbar.idleAllGood");
  }
  void status;
  return (
    <div className="duck-actionbar" data-state={visualState}>
      <span className="ab-message">{message}</span>
      {secondary && (
        <button type="button" className="ab-ghost" onClick={onDiscard}>
          {secondary.label}
        </button>
      )}
      {primary && (
        <button
          type="button"
          onClick={onApply}
          disabled={primary.disabled}
          className={visualState === "failed" ? "ab-danger" : ""}
        >
          {primary.label}
          {visualState !== "failed" && (
            <Icon name={primary.disabled ? "lock" : "arrow"} size={14} />
          )}
        </button>
      )}
    </div>
  );
}

/* — InspectorDrawer — */

function InspectorDrawer({
  locale,
  open,
  page,
  selection,
  tab,
  onTab,
  onClose,
  onClearSelection,
}: {
  locale: Locale;
  open: boolean;
  page: PageMeta;
  selection: InspectorObject | null;
  tab: "what" | "why" | "risk" | "example";
  onTab: (t: "what" | "why" | "risk" | "example") => void;
  onClose: () => void;
  onClearSelection: () => void;
}) {
  if (!open) return null;
  const heading = selection?.displayName ?? page.name;

  return (
    <aside className="duck-drawer" role="complementary" aria-label="Inspector">
      <div className="drawer-head">
        <Icon name="info" size={18} />
        <h3>{heading}</h3>
        <span style={{ flex: 1 }} />
        <button
          type="button"
          className="duck-tool"
          onClick={onClose}
          aria-label="Close inspector"
        >
          <Icon name="close" size={16} />
        </button>
      </div>
      <div className="tabs" role="tablist">
        <button
          role="tab"
          aria-selected={tab === "what"}
          onClick={() => onTab("what")}
        >
          {v4t(locale, "inspector.tabs.what")}
        </button>
        <button
          role="tab"
          aria-selected={tab === "why"}
          onClick={() => onTab("why")}
        >
          {v4t(locale, "inspector.tabs.why")}
        </button>
        <button
          role="tab"
          aria-selected={tab === "risk"}
          onClick={() => onTab("risk")}
        >
          {v4t(locale, "inspector.tabs.risk")}
        </button>
        <button
          role="tab"
          aria-selected={tab === "example"}
          onClick={() => onTab("example")}
        >
          {v4t(locale, "inspector.tabs.example")}
        </button>
      </div>
      <div className="drawer-body">
        {selection ? (
          <>
            <p style={{ marginTop: 0, color: "var(--text-muted)" }}>{page.question}</p>
            <button
              type="button"
              onClick={onClearSelection}
              className="duck-tool"
              style={{ border: "1px solid var(--line)", width: "auto", padding: "6px 12px", height: "auto" }}
            >
              <Icon name="close" size={14} />
              <span style={{ marginLeft: 6 }}>Clear selection</span>
            </button>
          </>
        ) : (
          <p style={{ color: "var(--text-muted)" }}>{v4t(locale, "inspector.noObjectSelected")}</p>
        )}

        <div className="tech">
          <h4 style={{ margin: "0 0 var(--space-s)", fontSize: "var(--fs-s)", color: "var(--text-subtle)", letterSpacing: "var(--track-caps)", textTransform: "uppercase" }}>
            {v4t(locale, "inspector.technicalDetails")}
          </h4>
          {selection && (
            <dl>
              <dt>{v4t(locale, "inspector.kind")}</dt>
              <dd>{selection.refKind ?? selection.kind}</dd>
              <dt>{v4t(locale, "inspector.objectId")}</dt>
              <dd>{selection.refId ?? selection.ref}</dd>
              <dt>{v4t(locale, "inspector.snapshot")}</dt>
              <dd>
                {(selection.diagnostics &&
                  (selection.diagnostics as { snapshot?: string }).snapshot) ||
                  "—"}
              </dd>
            </dl>
          )}
        </div>
      </div>
    </aside>
  );
}

/* local re-exports kept for downstream pages */
export type { HelpHit };
