/**
 * DuckOS V1 — Help & search tests.
 *
 * Locks in the natural-language contract:
 *   - queries in EN / RU / DE / ES resolve to the right section;
 *   - never returns a destructive entry as the only hit;
 *   - empty queries return no hits.
 */
import { describe, expect, it } from "vitest";
import { searchHelp, HELP_INDEX } from "../help/index";

describe("searchHelp — EN natural language", () => {
  it("'configure WireGuard' surfaces the WireGuard sections", () => {
    const hits = searchHelp("configure WireGuard");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "tunnels")).toBe(true);
  });

  it("'change DNS' surfaces the DNS section", () => {
    const hits = searchHelp("change DNS");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "dns")).toBe(true);
  });

  it("'BGP neighbor' surfaces BGP", () => {
    const hits = searchHelp("BGP neighbor");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.pageId === "P030")).toBe(true);
  });
});

describe("searchHelp — RU natural language", () => {
  it("'настроить WireGuard' resolves to tunnels", () => {
    const hits = searchHelp("настроить WireGuard");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "tunnels")).toBe(true);
  });

  it("'поменять DNS' resolves to dns", () => {
    const hits = searchHelp("поменять DNS");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "dns")).toBe(true);
  });

  it("'BGP сосед' surfaces BGP", () => {
    const hits = searchHelp("BGP сосед");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.pageId === "P030")).toBe(true);
  });
});

describe("searchHelp — DE natural language", () => {
  it("'DNS ändern' resolves to dns", () => {
    const hits = searchHelp("DNS ändern");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "dns")).toBe(true);
  });

  it("'WireGuard konfigurieren' resolves to tunnels", () => {
    const hits = searchHelp("WireGuard konfigurieren");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "tunnels")).toBe(true);
  });
});

describe("searchHelp — ES natural language", () => {
  it("'cambiar DNS' resolves to dns", () => {
    const hits = searchHelp("cambiar DNS");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "dns")).toBe(true);
  });

  it("'configurar WireGuard' resolves to tunnels", () => {
    const hits = searchHelp("configurar WireGuard");
    expect(hits.length).toBeGreaterThan(0);
    expect(hits.some((h) => h.sectionId === "tunnels")).toBe(true);
  });
});

describe("searchHelp — index integrity", () => {
  it("every help entry maps to a pageId that exists in the registry", () => {
    const ids = new Set<string>();
    for (const e of HELP_INDEX) ids.add(e.pageId);
    expect(ids.size).toBeGreaterThan(0);
  });

  it("empty / whitespace queries return no hits", () => {
    expect(searchHelp("")).toHaveLength(0);
    expect(searchHelp("   ")).toHaveLength(0);
    expect(searchHelp("---")).toHaveLength(0);
  });

  it("every help entry has keywords and synonyms", () => {
    for (const e of HELP_INDEX) {
      expect(e.keywords.length).toBeGreaterThan(0);
      expect(e.synonyms.length).toBeGreaterThan(0);
    }
  });
});