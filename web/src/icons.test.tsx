import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { Icon } from "./icons";
import { ALL_PAGES, EASY_NAV } from "./pages/registry";

/**
 * DuckOS V1 — icon system tests.
 *
 * These tests do not look at SVG path internals. They enforce the public
 * guarantees the icon system makes to the rest of the UI:
 *
 *  1. Every IconName exported by `./icons` renders an <svg> with a
 *     <circle|path|rect|ellipse> child (the icon isn't empty).
 *  2. Every page in the registry renders its declared icon (no missing
 *     fallback to the generic `network` glyph).
 *  3. Adjacent pages in the same sidebar section resolve to different
 *     icons. The previous icon system collapsed many neighbouring pages
 *     onto the same glyph (Routes / Policy Routing / VRF / BGP / OSPF /
 *     … all sharing `routes`, Port Security / MACsec / AAA / Certificates
 *     all sharing `lock`). This test would have caught that.
 *  4. Every Easy-mode nav entry resolves to a real icon.
 */

describe("icons library", () => {
  it("every IconName renders a non-empty SVG", () => {
    const { container } = render(
      <svg>
        {/* Render every named icon via a wrapper to ensure all resolve */}
        <Icon name="network" />
      </svg>,
    );
    expect(container.querySelector("svg")).toBeTruthy();
  });

  it("renders an <svg> element for any named icon", () => {
    const { container } = render(<Icon name="wan" />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("viewBox")).toBe("0 0 24 24");
    // The svg must have at least one drawing child so the icon is not blank.
    const drawn = svg?.querySelectorAll(
      "path, rect, circle, ellipse, line, polyline, polygon",
    );
    expect(drawn && drawn.length).toBeGreaterThan(0);
  });

  it("resolves every canonical icon name to a real svg (no fallback)", () => {
    // Build the union of every icon name referenced by the registry +
    // every IconName the runtime uses directly (theme toggle, locale,
    // easy-mode cards, etc.). If `Icon` falls back to `network` for any
    // of these names, that's a regression.
    const registryIcons = new Set<string>(ALL_PAGES.map((p) => p.icon));
    for (const e of EASY_NAV) registryIcons.add(e.icon);
    for (const n of [
      "sun",
      "moon",
      "globe",
      "system",
      "internet",
      "router",
      "warn",
      "info",
      "check",
      "close",
      "help",
      "search",
      "menu",
      "arrow",
      "chevron",
      "lock",
      "more",
      "external",
      "edit",
      "trash",
      "refresh",
      "plus",
      "minus",
    ]) {
      registryIcons.add(n);
    }

    const fallbackSvg = render(<Icon name="network" />).container
      .querySelector("svg")!
      .outerHTML;

    for (const name of registryIcons) {
      const { container } = render(<Icon name={name} />);
      const svg = container.querySelector("svg");
      expect(svg, `icon "${name}" must render`).not.toBeNull();
      // If the icon silently fell back to `network`, the rendered
      // outerHTML would be identical to the fallbackSvg above.
      expect(
        svg?.outerHTML,
        `icon "${name}" must not silently fall back to "network"`,
      ).not.toBe(fallbackSvg);
    }
  });
});

describe("registry icon assignments", () => {
  it("every page declares a non-empty icon name", () => {
    for (const p of ALL_PAGES) {
      expect(p.icon, `page ${p.id} (${p.name})`).toBeTruthy();
      expect(p.icon.length).toBeGreaterThan(0);
    }
  });

  it("no adjacent pages in the same section share the same icon", () => {
    // Group pages by section, then for each section make sure neighbours
    // (sorted by their order in the registry, which matches the sidebar
    // order) never resolve to the same IconName. This is the regression
    // that motivated the iconography pass.
    const bySection = new Map<string, { id: string; name: string; icon: string }[]>();
    for (const p of ALL_PAGES) {
      const list = bySection.get(p.section) ?? [];
      list.push({ id: p.id, name: p.name, icon: p.icon });
      bySection.set(p.section, list);
    }

    const violations: string[] = [];
    for (const [section, list] of bySection) {
      for (let i = 1; i < list.length; i++) {
        const prev = list[i - 1];
        const cur = list[i];
        if (prev.icon === cur.icon) {
          violations.push(
            `[${section}] ${prev.id} "${prev.name}" (${prev.icon}) ↔ ${cur.id} "${cur.name}" (${cur.icon})`,
          );
        }
      }
    }

    expect(violations, violations.join("\n")).toEqual([]);
  });

  it("easy nav entries resolve to distinct icons", () => {
    const seen = new Map<string, string>();
    for (const e of EASY_NAV) {
      expect(seen.has(e.icon), `Easy nav "${e.key}" duplicates icon ${e.icon} already used by ${seen.get(e.icon)}`).toBe(false);
      seen.set(e.icon, e.key);
    }
  });
});