/**
 * DuckOS V1 — object search.
 *
 * Extends the global search with hits on real observed objects:
 *   - interface names (eth0, eth1, wg0, ...)
 *   - IPv4 / IPv6 addresses
 *   - interface MAC addresses
 *   - VLAN ids
 *   - system hostname
 *   - service names
 *
 * Every hit is a navigation suggestion. We never execute destructive
 * actions from search. Selecting an INTERFACE hit routes the user to
 * /interfaces and selects the row + opens Inspector.
 */
import type { Overview } from "../../../shared/api/types";

export type ObjectKind =
  "interface" | "address" | "vlan" | "device" | "service";

export interface ObjectHit {
  /** Stable object id (e.g. "eth0", "10.0.0.1"). */
  id: string;
  /** Object category. */
  kind: ObjectKind;
  /** Operator-facing display name. */
  displayName: string;
  /** One-line contextual value (address, MAC, state, etc.). */
  context: string;
  /** Route the user lands on when they pick this hit. */
  route: string;
  /** Extra navigation hint — for INTERFACE hits this is the interface
   *  ref kind/id so the destination page can highlight the row. */
  ref?: { kind: string; id: string };
  /** Search relevance score (higher = better). */
  score: number;
}

const STOP = new Set([
  "the",
  "and",
  "for",
  "with",
  "this",
  "that",
  "from",
  "your",
  "is",
  "of",
  "to",
  "in",
  "on",
  "at",
  "as",
  "by",
  "an",
  "a",
]);

/** Normalise an IP-ish string so users can paste "172.18.4.149/16". */
function normalise(input: string): string {
  return input.trim().toLowerCase();
}

/** Loose IPv4 matcher. */
function isIPv4ish(input: string): boolean {
  // accepts "172", "172.18", "172.18.4", "172.18.4.149", "172.18.4.149/16"
  return /^[\d.]+(\/\d+)?$/.test(input);
}

/** Loose IPv6 matcher — accepts "fe80", "fe80::1", with optional /prefix. */
function isIPv6ish(input: string): boolean {
  return /^[0-9a-f:]+(\/\d+)?$/i.test(input) && input.includes(":");
}

function tokenise(query: string): string[] {
  return normalise(query)
    .split(/[^a-z0-9:.]+/g)
    .filter((t) => t.length > 0 && !STOP.has(t));
}

/** Single-hit interface builder — produces hits for the row, its addresses,
 *  its MAC, and its state (so "eth0 up" still matches). */
function interfaceHits(iface: Overview["interfaces"][number]): ObjectHit[] {
  const base = {
    route: "/interfaces",
    ref: { kind: "network.interface", id: iface.name },
  };
  const out: ObjectHit[] = [
    {
      ...base,
      id: iface.name,
      kind: "interface",
      displayName: iface.name,
      context: `Interface · ${iface.addresses.length} address${iface.addresses.length === 1 ? "" : "es"} · ${iface.state}`,
      score: 1,
    },
  ];
  for (const a of iface.addresses) {
    out.push({
      ...base,
      id: a.address,
      kind: "address",
      displayName: a.address,
      context: `${iface.name} · ${a.family} /${a.prefixLength}`,
      score: 0.95,
    });
  }
  if (iface.mac) {
    out.push({
      ...base,
      id: iface.mac,
      kind: "interface",
      displayName: iface.mac,
      context: `${iface.name} · MAC`,
      score: 0.85,
    });
  }
  // vlan id when present in the name (e.g. "eth0.10", "vlan100")
  const m =
    /^(.+?)\.(\d+)$/.exec(iface.name) || /^vlan(\d+)$/i.exec(iface.name);
  if (m) {
    const vlan = m[2] || m[1];
    out.push({
      ...base,
      id: `vlan:${vlan}`,
      kind: "vlan",
      displayName: `VLAN ${vlan}`,
      context: `${iface.name}`,
      score: 0.8,
    });
  }
  return out;
}

/** Run a token against an haystack and return a relevance score. */
function scoreText(query: string, haystack: string): number {
  if (!haystack) return 0;
  const h = haystack.toLowerCase();
  const i = h.indexOf(query);
  if (i < 0) return 0;
  // exact-prefix match beats substring
  if (h.startsWith(query)) return 2 - i * 0.001;
  return 1 - i * 0.001;
}

/** Public entry point. Returns ranked hits across all real objects
 *  the current Overview knows about. */
export function searchObjects(
  query: string,
  overview: Overview | null,
): ObjectHit[] {
  const q = normalise(query);
  if (q.length < 2) return [];
  const tokens = tokenise(query);
  if (tokens.length === 0) return [];

  // Build the universe of candidate hits from the Overview.
  const universe: ObjectHit[] = [];
  if (overview) {
    for (const i of overview.interfaces) universe.push(...interfaceHits(i));
    for (const s of overview.services) {
      universe.push({
        id: s.name,
        kind: "service",
        displayName: s.name,
        context: `Service · ${s.activeState} / ${s.subState}`,
        route: "/services",
        score: 1,
      });
    }
    universe.push({
      id: overview.system.hostname,
      kind: "device",
      displayName: overview.system.hostname,
      context: `Hostname · ${overview.environment}`,
      route: "/system/overview",
      score: 1,
    });
  }

  // Score every universe entry against the tokens. The user can paste a
  // full IP, a hostname fragment, or a partial interface name.
  const out: ObjectHit[] = [];
  for (const hit of universe) {
    const fields = [hit.id, hit.displayName, hit.context];
    let s = 0;
    for (const tok of tokens) {
      let local = 0;
      for (const f of fields) {
        const sc = scoreText(tok, f);
        if (sc > local) local = sc;
      }
      s += local;
    }
    if (s > 0) out.push({ ...hit, score: s });
  }

  // IPv4 / IPv6 short-form: match prefix-starts-with across all
  // addresses / MACs / interface names so "172.18" finds eth0.
  if (isIPv4ish(q) || isIPv6ish(q) || /^[0-9a-f.]+$/.test(q)) {
    for (const hit of universe) {
      const id = hit.id.toLowerCase();
      if (id.startsWith(q)) {
        out.push({ ...hit, score: hit.score + 1.5 });
      } else if (id.includes(q)) {
        out.push({ ...hit, score: hit.score + 0.5 });
      }
    }
  }

  // Dedup by id, keep the highest score.
  const byId = new Map<string, ObjectHit>();
  for (const h of out) {
    const existing = byId.get(h.id);
    if (!existing || h.score > existing.score) byId.set(h.id, h);
  }
  return Array.from(byId.values())
    .sort(
      (a, b) => b.score - a.score || a.displayName.localeCompare(b.displayName),
    )
    .slice(0, 8);
}
