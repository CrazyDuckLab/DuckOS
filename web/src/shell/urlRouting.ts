/**
 * DuckOS V1 — URL routing helpers for the SPA shell.
 *
 * The shell stores its active route in component state, but mirrors it to
 * `window.location.hash` so:
 *   - refreshing the page reopens the same screen;
 *   - the browser back/forward buttons navigate between screens;
 *   - direct URL entry (deep links) reaches the canonical page.
 *
 * We use hash-based routing on purpose: the catalog exposes canonical paths
 * like `/system/overview`, but a single-page app does not own the server
 * side, so any browser reload would otherwise 404 on those paths. Hash
 * routing keeps the URL meaningful without requiring server rewrites.
 */
import { PAGES_BY_ROUTE } from "../pages/registry";

/**
 * Read the current route from `window.location.hash`, normalising the
 * leading slash and validating it against the V1 catalog. Returns `null`
 * when the hash is empty or references an unknown page, so callers can fall
 * back to the canonical home route.
 *
 * Visible to tests so the URL contract can be exercised in isolation.
 */
export function readRouteFromHash(): string | null {
  if (typeof window === "undefined") return null;
  const raw = window.location.hash;
  if (!raw || raw.length <= 1) return null;
  let path = raw.slice(1);
  if (!path.startsWith("/")) path = "/" + path;
  // Drop any query/fragment suffix the user appended.
  const q = path.indexOf("?");
  if (q >= 0) path = path.slice(0, q);
  if (!PAGES_BY_ROUTE.has(path)) return null;
  return path;
}

/**
 * Make sure the address bar reflects the active route. No-op when the hash
 * already matches, and uses `replaceState` so the back stack is not polluted
 * by every shell render.
 */
export function syncHashToRoute(route: string): void {
  if (typeof window === "undefined") return;
  const expected = "#" + route;
  if (window.location.hash === expected) return;
  try {
    window.history.replaceState(null, "", expected);
  } catch {
    /* sandboxed contexts may forbid history writes; fall back silently */
  }
}