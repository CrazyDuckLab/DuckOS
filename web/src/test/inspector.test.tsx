/**
 * DuckOS V1 — Inspector tests.
 *
 * Locks in the contextual Inspector contract:
 *   - renders the page-doc blocks when nothing is selected;
 *   - renders an object panel at the top when a selection is set;
 *   - ignores selections whose pageId does not match the current page
 *     (so stale selections from a previous route cannot mislead);
 *   - exposes a "clear selection" affordance when an object is shown.
 */
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Messages } from "../i18n/messages";
import { PAGES_BY_ID } from "../pages/registry";
import { Inspector } from "../shell/Inspector";
import {
  PrefsContext,
  initialPrefs,
  type InspectorObject,
} from "../state/store";

const bundle = {} as Messages;

function wrap(
  node: React.ReactNode,
  inspectorObject: InspectorObject | null,
) {
  const value = {
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
      inspectorObject,
    },
    dispatch: () => {},
  };
  return render(
    <PrefsContext.Provider value={value}>{node}</PrefsContext.Provider>,
  );
}

describe("Inspector — page-doc mode (no selection)", () => {
  it("renders the page question from registry metadata", () => {
    const page = PAGES_BY_ID.get("P087")!; // Topology
    const { container } = wrap(
      <Inspector bundle={bundle} page={page} onClose={() => {}} />,
      null,
    );
    expect(container.textContent).toContain(page.question);
  });

  it("lists the columns the page will expose", () => {
    const page = PAGES_BY_ID.get("P087")!;
    const firstColumn = page.columns.split(/[,;]/)[0].trim();
    expect(firstColumn.length).toBeGreaterThan(0);
    const { container } = wrap(
      <Inspector bundle={bundle} page={page} onClose={() => {}} />,
      null,
    );
    expect(container.textContent).toContain(firstColumn);
  });
});

describe("Inspector — contextual object mode", () => {
  const sample: InspectorObject = {
    pageId: "P087",
    kind: "interface",
    ref: "aa:bb:cc:dd:ee:ff",
    displayName: "eth0",
    status: "up",
    fields: [
      { label: "Name", value: "eth0" },
      { label: "MAC", value: "aa:bb:cc:dd:ee:ff", mono: true },
      { label: "MTU", value: "1500", mono: true },
    ],
  };

  it("renders the object panel when a selection matches the page", () => {
    const page = PAGES_BY_ID.get("P087")!;
    const { container } = wrap(
      <Inspector bundle={bundle} page={page} onClose={() => {}} />,
      sample,
    );
    expect(container.textContent).toContain("eth0");
    expect(container.textContent).toContain("aa:bb:cc:dd:ee:ff");
    expect(container.textContent).toContain("MTU");
  });

  it("ignores a stale selection whose pageId does not match", () => {
    const page = PAGES_BY_ID.get("P087")!;
    const stale: InspectorObject = { ...sample, pageId: "P999" };
    const { container } = wrap(
      <Inspector bundle={bundle} page={page} onClose={() => {}} />,
      stale,
    );
    // The object panel must NOT render. The page-doc question for P087
    // still appears, so we can assert presence of the question as a
    // sanity check.
    expect(container.textContent).not.toContain("eth0");
    expect(container.textContent).toContain(page.question);
  });

  it("exposes a clear-selection affordance when an object is shown", () => {
    const page = PAGES_BY_ID.get("P087")!;
    const { container } = wrap(
      <Inspector bundle={bundle} page={page} onClose={() => {}} />,
      sample,
    );
    const buttons = container.querySelectorAll("button");
    const clearButton = Array.from(buttons).find((b) =>
      b.getAttribute("aria-label") === "Clear selection",
    );
    expect(clearButton).toBeDefined();
  });
});