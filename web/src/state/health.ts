/**
 * DuckOS V1 — Centralized health-state derivation.
 *
 * This module is the SINGLE source of truth for health-related UI state
 * across the AppShell health chip, Easy Home, Easy Care, and System
 * Overview. Pages MUST NOT derive their own overall verdict.
 *
 * Authoritative semantics (per production API contracts):
 *
 *   GET /api/v1/health
 *     → { "status": "...", "schemaVersion": "1" }
 *     proves only API/service reachability at its own level.
 *     It does NOT carry:
 *       - component health
 *       - Doctor findings
 *       - observation freshness
 *       - time trust
 *       - verified system health
 *
 *   GET /api/v1/doctor
 *     → { "findings": [...] }
 *     carries current Doctor findings. A non-OK response is NOT
 *     equivalent to "no findings"; it means Doctor status is
 *     unavailable / incomplete.
 *
 *   GET /api/v1/overview
 *     → { "observedAt": "...", "issues": [...], ... }
 *     carries the server timestamp of the last observation and any
 *     issues observed at the system level. We only treat
 *     observedAt as "recent" when:
 *       - it parses to a finite number,
 *       - it is not in the future beyond a small skew tolerance,
 *       - it is not older than the freshness threshold,
 *       - it is not absent.
 *
 * Conservative rule (the one and only rule):
 *   Unknown or unavailable evidence NEVER becomes positive evidence.
 *
 *   Doctor request failed         != zero Doctor findings
 *   Missing observedAt            != fresh observation
 *   Future observedAt             != fresh observation
 *   API reachable                 != system healthy
 *   Recent observation            != system healthy
 *   Zero Doctor findings          != system healthy
 *   No time-trust evidence        != trusted time
 *
 * As a result, the global "Healthy" verdict MUST NOT be emitted
 * unless the existing backend provides ALL evidence required by the
 * established DuckOS health semantics (verified system health +
 * trusted time). Today, no production endpoint provides verified
 * system health, so "Healthy" is structurally unreachable until the
 * backend grows that field. We default to `incomplete` rather than
 * fabricate a positive claim.
 */
import type { Overview } from "../../../shared/api/types";
import type { FetchResult } from "../api/client";
import type { HealthPayload } from "../api/client";
import type { DoctorFinding } from "../api/types";
import type { HealthTone } from "./store";

/* — Source states — */

/**
 * API health-probe state. Loading is the "before the first fetch
 * resolved" period. Reachable means the production `{status:"ok"}`
 * payload came back. Unreachable means the request failed, the
 * payload was missing, or `status !== "ok"`.
 */
export type ApiState = "loading" | "reachable" | "unreachable";

/**
 * Doctor endpoint state. Loading is "no fetch resolved yet". Success
 * means the request returned OK with a payload (even an empty
 * findings array). Error means the request failed or the payload was
 * malformed — Doctor is unavailable.
 */
export type DoctorState = "loading" | "success" | "error";

/**
 * Parsed observation timestamp classification.
 *
 *   missing   - no observedAt in the payload
 *   invalid   - observedAt present but not parseable
 *   future    - observedAt is more than `FUTURE_SKEW_MS` ahead of now
 *   stale     - observedAt is older than `FRESHNESS_MS`
 *   recent    - observedAt parsed, not future, within freshness window
 *
 * A future timestamp is NEVER recent.
 */
export type ObservationState =
  | "missing"
  | "invalid"
  | "future"
  | "stale"
  | "recent";

/**
 * Per-source input bag. Anything that is unknown stays undefined /
 * null — that's the only honest way to feed this derivation without
 * fabricating evidence.
 */
export interface HealthSources {
  /** Result of /api/v1/health, or null while loading. */
  health: FetchResult<HealthPayload> | null;
  /** Result of /api/v1/doctor, or null while loading. */
  doctor: FetchResult<unknown> | null;
  /** Result of /api/v1/overview, or null while loading. */
  overview: FetchResult<Overview> | null;
}

/* — Tuning — */

/**
 * Maximum age, in milliseconds, for an observation to be considered
 * "recent". Beyond this, the observation is stale.
 */
export const FRESHNESS_MS = 5 * 60 * 1000; // 5 minutes

/**
 * Clock-skew tolerance for the future-timestamp guard. We allow a
 * few seconds of forward drift before declaring a timestamp "future".
 * This is intentionally small: an hour-ahead timestamp is not
 * "fresh".
 */
export const FUTURE_SKEW_MS = 5_000; // 5 seconds

/**
 * "Now" injection point so tests can pin time without monkey-patching
 * `Date.now`. Pages pass `undefined` and get `Date.now()`; tests can
 * pass an explicit number.
 */
export type NowFn = () => number;

const defaultNow: NowFn = () => Date.now();

/* — Helpers — */

/**
 * Parse `payload.observedAt` into a finite epoch ms, or `null` if the
 * timestamp is missing or not a valid date.
 *
 * Accepts ISO-8601 strings and anything `new Date(...)` can parse.
 * Returns null for empty strings, NaN-producing inputs, and
 * non-finite numbers.
 */
export function parseObservedAt(
  payload: Pick<Overview, "observedAt"> | null | undefined,
): number | null {
  if (!payload || typeof payload !== "object") return null;
  const raw = payload.observedAt;
  if (typeof raw !== "string" || raw.length === 0) return null;
  const ms = new Date(raw).getTime();
  if (!Number.isFinite(ms)) return null;
  return ms;
}

/**
 * Classify an observation timestamp into one of the five
 * observation states. Always returns a state — never throws, never
 * fabricates freshness.
 *
 * Rules:
 *   - parse failure      → invalid
 *   - missing            → missing
 *   - now + skew < ts    → future (NOT fresh)
 *   - now - ts > fresh   → stale
 *   - otherwise          → recent
 */
export function classifyObservation(
  payload: Pick<Overview, "observedAt"> | null | undefined,
  now: number = Date.now(),
): ObservationState {
  const ms = parseObservedAt(payload);
  if (ms === null) {
    // Distinguish "field present but invalid" from "field missing".
    if (payload && typeof payload === "object" && typeof payload.observedAt === "string" && payload.observedAt.length > 0) {
      return "invalid";
    }
    return "missing";
  }
  if (ms > now + FUTURE_SKEW_MS) return "future";
  if (now - ms > FRESHNESS_MS) return "stale";
  return "recent";
}

/**
 * Extract Doctor findings from a Doctor fetch result. Returns
 * `null` when the request failed — Doctor unavailable is NOT the
 * same as Doctor empty. Empty arrays are real findings (Doctor
 * explicitly reported "nothing wrong").
 */
export function doctorFindingsFromResult(
  result: FetchResult<unknown> | null,
): DoctorFinding[] | null {
  if (!result || !result.ok) return null;
  const p = result.data as { findings?: unknown; issues?: unknown };
  const list = Array.isArray(p.findings) ? p.findings : Array.isArray(p.issues) ? p.issues : [];
  return list as DoctorFinding[];
}

/**
 * Classify the Doctor source state from a fetch result.
 */
export function doctorStateFromResult(
  result: FetchResult<unknown> | null,
): DoctorState {
  if (!result) return "loading";
  if (!result.ok) return "error";
  return "success";
}

/**
 * Classify the API health-probe state from a fetch result.
 *
 *   null result            → loading
 *   !ok result             → unreachable
 *   status !== "ok"        → unreachable (per production contract
 *                             only `status:"ok"` is reachable)
 *   status === "ok"        → reachable
 */
export function apiStateFromResult(
  result: FetchResult<HealthPayload> | null,
): ApiState {
  if (!result) return "loading";
  if (!result.ok) return "unreachable";
  if (result.data.status !== "ok") return "unreachable";
  return "reachable";
}

/* — Derived per-source tones (low-level signals, NOT the overall) — */

/**
 * Doctor-derived tone — NEVER promoted to "good" on Doctor success
 * alone. Doctor success with findings is `warn`. Doctor success
 * with empty findings is `unknown` (Doctor empty is not verified
 * healthy). Doctor error is `unknown` (we never auto-convert to
 * `[]` and we never claim Doctor empty).
 */
export function doctorTone(sources: HealthSources): HealthTone {
  const state = doctorStateFromResult(sources.doctor);
  if (state === "loading" || state === "error") return "unknown";
  // state === "success"
  const findings = doctorFindingsFromResult(sources.doctor);
  if (!findings || findings.length > 0) return "warn";
  // Doctor empty is honest "unknown" — not verified healthy.
  return "unknown";
}

/**
 * Overview-derived tone. Overview issues come from
 * `payload.issues[]`. A recent observation with no issues is
 * `unknown` (recent is not verified healthy). Stale / future /
 * missing / invalid are `unknown`. Doctor findings are NOT mixed
 * in here — that lives in the overall derivation.
 */
export function overviewTone(sources: HealthSources): HealthTone {
  const overview = sources.overview;
  if (!overview || !overview.ok) return "unknown";
  const data = overview.data;
  const issues = Array.isArray(data.issues) ? data.issues.length : 0;
  const obs = classifyObservation(data);
  if (issues > 0) return "warn";
  if (obs !== "recent") return "unknown";
  return "unknown"; // recent + zero issues is still NOT verified healthy.
}

/* — Derived state model — */

export interface DerivedHealth {
  /* source states (truthful, no fabrication) */
  api: ApiState;
  doctor: DoctorState;
  observation: ObservationState;

  /* low-level per-source tones */
  doctorTone: HealthTone;
  overviewTone: HealthTone;

  /* doctor findings, or null when Doctor is unavailable */
  doctorFindings: DoctorFinding[] | null;

  /* observation parsed timestamp, or null when missing/invalid */
  observedAtMs: number | null;

  /* API reachability is exposed separately because pages need it for
   * binary liveness pills; it MUST NOT be promoted to a Healthy
   * verdict. */
  apiReachable: boolean;

  /* Top-level verdict for the Easy Home headline + System Overview
   * headline + AppShell chip.
   *
   *   unreachable  - API cannot be reached at all
   *   attention    - Doctor or overview raised findings
   *   stale        - observation is too old
   *   incomplete   - evidence is insufficient (no recent verified
   *                   signal yet, doctor empty, etc.)
   *
   * IMPORTANT: a "good" verdict is intentionally NOT present in
   * this enum. Verified system health is structurally unreachable
   * today because the production /api/v1/health contract carries
   * no verified-health evidence. We refuse to synthesize it.
   */
  overall:
    | "unreachable"
    | "attention"
    | "stale"
    | "incomplete";
}

/**
 * Derive the centralized health state from raw source results.
 *
 * Rules in priority order (first match wins):
 *   1. api === unreachable              → overall: unreachable
 *   2. doctor === error                 → overall: incomplete (Doctor
 *                                          unavailable, NOT zero
 *                                          findings)
 *   3. doctor === success AND findings
 *      OR overview issues > 0           → overall: attention
 *   4. observation in (stale, future)   → overall: stale
 *   5. observation in (missing, invalid,
 *      recent) AND doctor empty AND
 *      overview issues 0               → overall: incomplete
 *
 * "Healthy" / "verified healthy" is intentionally unreachable.
 */
export function deriveHealth(
  sources: HealthSources,
  now: number = defaultNow(),
): DerivedHealth {
  const api = apiStateFromResult(sources.health);
  const doctor = doctorStateFromResult(sources.doctor);
  const observation = classifyObservation(
    sources.overview && sources.overview.ok ? sources.overview.data : null,
    now,
  );

  const doctorFindings = doctorFindingsFromResult(sources.doctor);
  const doctorToneVal: HealthTone =
    doctor === "loading"
      ? "unknown"
      : doctor === "error"
        ? "unknown"
        : doctorFindings && doctorFindings.length > 0
          ? "warn"
          : "unknown";
  const overviewToneVal = overviewTone(sources);

  const apiReachable = api === "reachable";

  let overall: DerivedHealth["overall"];
  if (api === "unreachable") {
    overall = "unreachable";
  } else if (doctor === "error") {
    // Doctor request failed — surface as incomplete. We do NOT
    // synthesize an empty Doctor.
    overall = "incomplete";
  } else if (
    (doctor === "success" && doctorFindings && doctorFindings.length > 0) ||
    overviewToneVal === "warn"
  ) {
    overall = "attention";
  } else if (observation === "stale" || observation === "future") {
    overall = "stale";
  } else {
    // observation in (missing, invalid, recent) AND no Doctor
    // findings AND no overview issues. Even a recent observation
    // is not "verified healthy" — we do not have trusted-time
    // evidence. Stay incomplete.
    overall = "incomplete";
  }

  return {
    api,
    doctor,
    observation,
    doctorTone: doctorToneVal,
    overviewTone: overviewToneVal,
    doctorFindings,
    observedAtMs: parseObservedAt(
      sources.overview && sources.overview.ok ? sources.overview.data : null,
    ),
    apiReachable,
    overall,
  };
}

/* — Convenience for AppShell HealthChip — */

/**
 * Map the centralized derived state to the existing AppShell
 * HealthTone vocabulary (`"good" | "warn" | "bad" | "unknown"`) for
 * the topbar chip. The chip never shows "good" without verified
 * evidence, so we map "incomplete" / "unreachable" to "unknown" and
 * "stale" to "unknown" (a stale observation is honest, not
 * Healthy), and "attention" to "warn".
 *
 * Returning "unknown" is the default and represents an honest "we
 * can't tell yet" verdict.
 */
export function chipTone(derived: DerivedHealth): HealthTone {
  switch (derived.overall) {
    case "unreachable":
    case "stale":
    case "incomplete":
      return "unknown";
    case "attention":
      return "warn";
  }
}

/**
 * Localized label key for the topbar chip. Returns the v4 i18n key
 * (without the prefix) so callers can resolve it via
 * `v4t(locale, ...)`.
 */
export function chipLabelKey(
  derived: DerivedHealth,
):
  | "health.healthy"
  | "health.needsAttention"
  | "health.statusIncomplete"
  | "health.observationUntrusted"
  | "health.unavailable" {
  switch (derived.overall) {
    case "unreachable":
      return "health.unavailable";
    case "attention":
      return "health.needsAttention";
    case "stale":
      return "health.observationUntrusted";
    case "incomplete":
      return "health.statusIncomplete";
  }
}