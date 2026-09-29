/**
 * Locks in the favicon asset. The production HTML must reference a
 * current DuckMark so the browser tab / OS dock / bookmark bar show the
 * rubber-duck brand instead of any older mark.
 */
import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

describe("Favicon / brand asset", () => {
  const webRoot = resolve(__dirname, "..", "..");

  it("index.html links to /duck.svg", () => {
    const html = readFileSync(resolve(webRoot, "index.html"), "utf8");
    expect(html).toMatch(/<link[^>]+rel="icon"[^>]+href="\/duck\.svg"/);
  });

  it("/duck.svg is the rubber-duck mark (warm yellow body, orange bill)", () => {
    const svg = readFileSync(resolve(webRoot, "public", "duck.svg"), "utf8");
    // body uses a warm-yellow gradient stop
    expect(svg.toUpperCase()).toMatch(/F5C533|F4BB3D/);
    // bill uses a warm-orange gradient stop
    expect(svg.toUpperCase()).toMatch(/FF9244|E5802A/);
    // eye is the dark slate (works in light + dark)
    expect(svg.toUpperCase()).toMatch(/1A1A1A|1F2937/);
  });
});