/**
 * Locks in the canonical product-level health conclusion.
 *
 * Rules (matching AppShell.tsx HealthChip):
 *   1. !reachable                 → "Status incomplete" / unknown
 *   2. !observed (no lastObservedAt) → "Status incomplete" / unknown
 *   3. !fresh (timeTrust != "trusted") → "Status incomplete" / unknown
 *   4. doctor or overview === "bad"  → "Unavailable" / bad
 *   5. doctor or overview === "warn" → "Needs attention" / warn
 *   6. all observed tones === "good" → "Healthy" / good
 *   7. otherwise                  → "Status incomplete" / unknown
 *
 * "Healthy" requires real evidence from the doctor and overview endpoints
 * plus a trusted observation timestamp. The /api/v1/health endpoint alone
 * is not enough — it proves only API reachability.
 */
import { describe, expect, it } from "vitest";
import type { HealthSummary } from "../state/store";

/** Mirrors the chip logic in AppShell.tsx. */
function chipFor(h: HealthSummary | null): { tone: string; label: string } {
  const reachable = h?.reachable === true;
  const observed = h != null && h.lastObservedAt !== null;
  const fresh = h?.timeTrust === "trusted";
  const tone: "good" | "warn" | "bad" | "unknown" = !reachable
    ? "unknown"
    : !observed
      ? "unknown"
      : !fresh
        ? "unknown"
        : h?.doctor === "bad" || h?.overview === "bad"
          ? "bad"
          : h?.doctor === "warn" || h?.overview === "warn"
            ? "warn"
            : h?.doctor === "good" && h?.overview === "good"
              ? "good"
              : "unknown";
  const label =
    tone === "good"
      ? "Healthy"
      : tone === "warn"
        ? "Needs attention"
        : tone === "bad"
          ? "Unavailable"
          : observed && !fresh
            ? "Status check incomplete"
            : "Status incomplete";
  return { tone, label };
}

function makeHealth(over: Partial<HealthSummary> = {}): HealthSummary {
  return {
    overview: "good",
    readiness: "good",
    doctor: "good",
    topology: "good",
    reachable: true,
    lastObservedAt: Date.now(),
    timeTrust: "trusted",
    activeIncidents: 0,
    pendingChanges: 0,
    ...over,
  };
}

describe("HealthChip semantics", () => {
  it("reports Healthy only when reachable + observed + fresh + doctor=good + overview=good", () => {
    const result = chipFor(
      makeHealth({ doctor: "good", overview: "good", timeTrust: "trusted", lastObservedAt: Date.now(), reachable: true }),
    );
    expect(result.tone).toBe("good");
    expect(result.label).toBe("Healthy");
  });

  it("refuses to say Healthy when API is not reachable", () => {
    const result = chipFor(
      makeHealth({ doctor: "good", overview: "good", timeTrust: "trusted", lastObservedAt: Date.now(), reachable: false }),
    );
    expect(result.tone).not.toBe("good");
    expect(result.label).toBe("Status incomplete");
  });

  it("refuses to say Healthy when observation timeTrust is untrusted", () => {
    const result = chipFor(
      makeHealth({ doctor: "good", overview: "good", timeTrust: "untrusted", lastObservedAt: Date.now() }),
    );
    expect(result.tone).not.toBe("good");
    // observed && !fresh → "Status check incomplete" (per health.observationUntrusted)
    expect(result.label).toBe("Status check incomplete");
  });

  it("refuses to say Healthy when observation has not been trusted yet", () => {
    const result = chipFor(
      makeHealth({ doctor: "good", overview: "good", timeTrust: "unknown", lastObservedAt: Date.now() }),
    );
    expect(result.tone).not.toBe("good");
    expect(result.label).toBe("Status check incomplete");
  });

  it("refuses to say Healthy when there has been no observation yet", () => {
    const result = chipFor(
      makeHealth({ doctor: "good", overview: "good", timeTrust: "unknown", lastObservedAt: null }),
    );
    expect(result.tone).toBe("unknown");
    expect(result.label).toBe("Status incomplete");
  });

  it("reports Needs attention when doctor is warn and observation is fresh", () => {
    const result = chipFor(
      makeHealth({ doctor: "warn", overview: "good", timeTrust: "trusted", lastObservedAt: Date.now() }),
    );
    expect(result.tone).toBe("warn");
    expect(result.label).toBe("Needs attention");
  });

  it("reports Unavailable when doctor is bad and observation is fresh", () => {
    const result = chipFor(
      makeHealth({ doctor: "bad", overview: "good", timeTrust: "trusted", lastObservedAt: Date.now() }),
    );
    expect(result.tone).toBe("bad");
    expect(result.label).toBe("Unavailable");
  });

  it("reports Status incomplete when health is null", () => {
    const result = chipFor(null);
    expect(result.label).toBe("Status incomplete");
  });

  it("does NOT default unknown tones to good", () => {
    // Default HealthSummary has doctor: "unknown" / overview: "unknown".
    // Even with reachable + observed + fresh, that must NOT become Healthy.
    const result = chipFor(
      makeHealth({ doctor: "unknown", overview: "unknown" }),
    );
    expect(result.tone).not.toBe("good");
    expect(result.label).toBe("Status incomplete");
  });
});