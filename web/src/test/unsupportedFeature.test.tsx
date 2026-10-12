/**
 * DuckOS V1 — UnsupportedFeature tests.
 *
 * Locks in the intentional-placeholder contract:
 *   - renders the page question
 *   - lists the planned columns (from registry metadata) so the operator
 *     can see what the live view will expose when the API lands
 *   - shows the dependency list (what would unblock the contract)
 *   - exposes the easy-hint summary when present
 *   - shows the contract endpoint when provided
 */
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Messages } from "../i18n/messages";
import { PAGES_BY_ID } from "../pages/registry";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import { PrefsContext, initialPrefs } from "../state/store";

const bundle = {} as Messages;

const prefsValue = {
  state: {
    ...initialPrefs,
    health: {
      overview: "good" as const,
      readiness: "good" as const,
      doctor: "good" as const,
      topology: "good" as const,
      reachable: true,
      lastObservedAt: Date.now(),
      timeTrust: "trusted" as const,
      activeIncidents: 0,
      pendingChanges: 0,
    },
  },
  dispatch: () => {},
};

function wrap(node: React.ReactNode) {
  return render(
    <PrefsContext.Provider value={prefsValue}>{node}</PrefsContext.Provider>,
  );
}

describe("UnsupportedFeature", () => {
  it("renders the page question from registry metadata", () => {
    const page = PAGES_BY_ID.get("P033")!; // BFD - small registry entry
    expect(page).toBeDefined();
    const { container } = wrap(
      <UnsupportedFeature page={page} bundle={bundle} contract="/api/v1/bfd" />,
    );
    expect(container.textContent).toContain(page.question);
  });

  it("lists the planned columns from registry metadata", () => {
    const page = PAGES_BY_ID.get("P033")!;
    const firstColumn = page.columns.split(/[,;]/)[0].trim();
    expect(firstColumn.length).toBeGreaterThan(0);
    const { container } = wrap(
      <UnsupportedFeature page={page} bundle={bundle} />,
    );
    expect(container.textContent).toContain(firstColumn);
  });

  it("shows the dependency list", () => {
    const page = PAGES_BY_ID.get("P033")!;
    const deps = page.dependencies
      .split(/[,;]/)
      .map((s) => s.trim())
      .filter(Boolean);
    expect(deps.length).toBeGreaterThan(0);
    const { container } = wrap(
      <UnsupportedFeature page={page} bundle={bundle} />,
    );
    for (const dep of deps) {
      expect(container.textContent).toContain(dep);
    }
  });

  it("exposes the easy-mode hint when present", () => {
    const page = PAGES_BY_ID.get("P033")!;
    expect(page.easyHint).toBeTruthy();
    const { container } = wrap(
      <UnsupportedFeature page={page} bundle={bundle} />,
    );
    expect(container.textContent).toContain("Easy mode summary");
    expect(container.textContent).toContain(page.easyHint);
  });

  it("renders the expected contract endpoint when provided", () => {
    const page = PAGES_BY_ID.get("P033")!;
    const { container } = wrap(
      <UnsupportedFeature page={page} bundle={bundle} contract="/api/v1/bfd" />,
    );
    expect(container.textContent).toContain("/api/v1/bfd");
  });

  it("uses the override blocker copy when provided", () => {
    const page = PAGES_BY_ID.get("P033")!;
    const { container } = wrap(
      <UnsupportedFeature
        page={page}
        bundle={bundle}
        contract="/api/v1/bfd"
        blocker="Custom blocker copy for testing."
      />,
    );
    expect(container.textContent).toContain("Custom blocker copy for testing.");
  });
});
