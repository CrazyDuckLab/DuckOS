import { createContext, useContext } from "react";
import type { Locale } from "../i18n/messages";

/**
 * DuckOS V1 — global UI state.
 *
 * This is *not* the system state. The system state still comes from the live
 * Engine/API. This holds only the things the operator chooses:
 *   - locale (EN / RU / DE / ES)
 *   - mode (Easy / Full)
 *   - theme (System / Light / Dark)
 *   - sidebar collapse (used in Full mode only)
 *   - the currently-open inspector page id (Full mode)
 *   - the local "pending review" list (used by Global Safe Apply)
 *
 * Important invariants:
 *   1. Disabled intentionally != degraded. We never downgrade a user's
 *      explicit mode choice because we can't observe the system.
 *   2. TIME_UNTRUSTED is represented as a flag (timeTrusted), not a value,
 *      so the rest of the UI can ask "is time trustworthy" without parsing.
 *   3. The pending list is local to the browser; the system has its own
 *      authoritative candidate transaction. We never pretend the local list
 *      *is* the transaction.
 */

export type Mode = "easy" | "full";
export type Theme = "system" | "light" | "dark";
export type HealthTone = "good" | "warn" | "bad" | "unknown";
export type TimeTrust = "trusted" | "untrusted" | "unknown";

export interface HealthSummary {
  /** /api/v1/overview reachable and recent (within 60s) */
  overview: HealthTone;
  /** /api/v1/readiness reachable */
  readiness: HealthTone;
  /** /api/v1/doctor reachable */
  doctor: HealthTone;
  /** /api/v1/topology reachable */
  topology: HealthTone;
  /**
   * Whether the /api/v1/health probe is reachable right now. This is
   * the ONLY signal the production health endpoint carries. It must
   * not be promoted to "good" / "Healthy" without additional evidence
   * from /overview or /doctor.
   */
  reachable: boolean;
  /** Last successful API call, ms since epoch or null. */
  lastObservedAt: number | null;
  /** Whether the system says its clock is trustworthy. */
  timeTrust: TimeTrust;
  /** Active incidents surfaced by /doctor or /overview.issues. */
  activeIncidents: number;
  /** Pending changes count surfaced by /candidate (or 0 when unknown). */
  pendingChanges: number;
}

export interface PendingChange {
  id: string;
  pageId: string;
  title: string;
  owner: string;
  risk: "R" | "C" | "I";
  before: string;
  after: string;
  blastRadius: string;
  canary: boolean;
  confirmWindowSec: number | null;
  expiresAt: number | null;
}

/**
 * A contextual Inspector selection. Pages dispatch `selectObject` when the
 * operator clicks a row; the Inspector switches to the object panel until
 * `clearSelection` (or a route change) resets it.
 *
 * The shape is intentionally small and stable so any area page can produce
 * one without leaking the page's internal data model. `fields` is the
 * ordered, operator-friendly summary (state, MAC, addresses, MTU, etc.);
 * the Inspector renders it verbatim without inventing extras.
 */
export interface InspectorObject {
  /** Page id that produced the selection. Used for breadcrumb and to
   *  know which per-page actions block to render. */
  pageId: string;
  /** Object kind, e.g. "interface", "route", "firewall-rule". */
  kind: string;
  /** Stable reference id from the backend. */
  ref: string;
  /** Parsed reference — split for technical details display. */
  refKind?: string;
  refId?: string;
  /** Operator-facing name (displayName). */
  displayName: string;
  /** Optional badge, e.g. "live", "stale", "degraded". */
  status?: string;
  /** Ordered summary fields (label → value). */
  fields: ReadonlyArray<{ label: string; value: string; mono?: boolean }>;
  /** Optional diagnostic block rendered under Technical details. */
  diagnostics?: Record<string, unknown>;
}

export interface PrefsState {
  locale: Locale;
  mode: Mode;
  theme: Theme;
  sidebarCollapsed: boolean;
  inspectorPageId: string | null;
  /** Contextual object currently shown by the Inspector, or null.
   * Optional so test fixtures can omit it (it always defaults to null). */
  inspectorObject?: InspectorObject | null;
  health: HealthSummary;
  pending: PendingChange[];
}

export const initialPrefs: PrefsState = {
  locale: "en",
  mode: "full",
  theme: "system",
  sidebarCollapsed: false,
  inspectorPageId: null,
  inspectorObject: null,
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
  pending: [],
};

export type PrefsAction =
  | { type: "setLocale"; locale: Locale }
  | { type: "setMode"; mode: Mode }
  | { type: "setTheme"; theme: Theme }
  | { type: "toggleSidebar" }
  | { type: "setSidebar"; collapsed: boolean }
  | { type: "openInspector"; pageId: string }
  | { type: "closeInspector" }
  | { type: "selectObject"; object: InspectorObject }
  | { type: "clearSelection" }
  | { type: "setHealth"; patch: Partial<HealthSummary> }
  | { type: "addPending"; change: PendingChange }
  | { type: "removePending"; id: string }
  | { type: "clearPending" };

export function prefsReducer(state: PrefsState, action: PrefsAction): PrefsState {
  switch (action.type) {
    case "setLocale":
      return { ...state, locale: action.locale };
    case "setMode":
      // Mode is intentional; never auto-downgrade it. The Easy/Full badge
      // is still visible in Full mode for users who switched from Easy.
      return { ...state, mode: action.mode };
    case "setTheme":
      return { ...state, theme: action.theme };
    case "toggleSidebar":
      return { ...state, sidebarCollapsed: !state.sidebarCollapsed };
    case "setSidebar":
      return { ...state, sidebarCollapsed: action.collapsed };
    case "openInspector":
      return { ...state, inspectorPageId: action.pageId };
    case "closeInspector":
      return { ...state, inspectorPageId: null };
    case "selectObject":
      return { ...state, inspectorObject: action.object };
    case "clearSelection":
      return { ...state, inspectorObject: null };
    case "setHealth":
      return { ...state, health: { ...state.health, ...action.patch } };
    case "addPending": {
      if (state.pending.some((c) => c.id === action.change.id)) return state;
      return { ...state, pending: [...state.pending, action.change] };
    }
    case "removePending":
      return {
        ...state,
        pending: state.pending.filter((c) => c.id !== action.id),
      };
    case "clearPending":
      return { ...state, pending: [] };
    default:
      return state;
  }
}

export interface PrefsContextValue {
  state: PrefsState;
  dispatch: React.Dispatch<PrefsAction>;
}

export const PrefsContext = createContext<PrefsContextValue | null>(null);

export function usePrefs(): PrefsContextValue {
  const ctx = useContext(PrefsContext);
  if (!ctx) {
    throw new Error("PrefsContext missing — wrap the tree with PrefsProvider.");
  }
  return ctx;
}

/**
 * Persistence — only the operator's choices belong here. We deliberately
 * *do not* persist the health summary or the pending list: those reset
 * every session, by design. A stale "pending: 3" badge from yesterday
 * would be a fake success.
 */
const PERSIST_KEY = "duckos.v1.prefs.v1";

export interface PersistedPrefs {
  locale: Locale;
  mode: Mode;
  theme: Theme;
  sidebarCollapsed: boolean;
}

export function loadPersisted(): Partial<PersistedPrefs> | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(PERSIST_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return null;
    return parsed as Partial<PersistedPrefs>;
  } catch {
    return null;
  }
}

export function persist(state: PrefsState): void {
  if (typeof window === "undefined") return;
  try {
    const subset: PersistedPrefs = {
      locale: state.locale,
      mode: state.mode,
      theme: state.theme,
      sidebarCollapsed: state.sidebarCollapsed,
    };
    window.localStorage.setItem(PERSIST_KEY, JSON.stringify(subset));
  } catch {
    // localStorage may be unavailable in private mode; failing silently is
    // safer than crashing the shell.
  }
}