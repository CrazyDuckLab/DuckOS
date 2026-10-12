import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DuckMark, DuckWordmark } from "./brand/DuckMark";

describe("DuckMark", () => {
  it("renders the rubber-duck SVG with an accessible label", () => {
    const { container } = render(<DuckMark />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("aria-label")).toBe("DuckOS");
    // The body+head silhouette is a single <path>; the tail + wing + beak
    // contribute the rest of the closed shapes that make the duck read.
    expect(container.querySelectorAll("path").length).toBeGreaterThanOrEqual(3);
  });

  it("uses warm yellow body + warm orange beak regardless of theme", () => {
    const { container } = render(<DuckMark />);
    const svgText = container.innerHTML;
    // body gradient stops — warm yellow base
    expect(svgText.toUpperCase()).toMatch(/F5C533|F4BB3D/);
    // beak gradient stops — warm orange
    expect(svgText.toUpperCase()).toMatch(/FF9244|E5802A/);
    // dark eye
    expect(svgText.toUpperCase()).toMatch(/1A1A1A|1F2937/);
  });

  it("respects a custom size", () => {
    const { container } = render(<DuckMark size={48} />);
    const svg = container.querySelector("svg") as SVGSVGElement | null;
    expect(svg?.getAttribute("width")).toBe("48");
    expect(svg?.getAttribute("height")).toBe("48");
  });

  it("hides the eye when withEye is false", () => {
    const { container } = render(<DuckMark withEye={false} />);
    // No dark-slate eye circle
    const eye = container.querySelector('circle[fill="#1A1A1A"]');
    expect(eye).toBeNull();
  });

  it("hides the eye automatically below 18px so the silhouette stays clean at favicon size", () => {
    const { container } = render(<DuckMark size={16} />);
    const eye = container.querySelector('circle[fill="#1A1A1A"]');
    expect(eye).toBeNull();
  });

  it("keeps the wing detail visible above 22px", () => {
    const { container } = render(<DuckMark size={32} />);
    // The wing path uses fill #D89F2C with opacity 0.42
    const wing = container.querySelector('path[fill="#D89F2C"]');
    expect(wing).not.toBeNull();
  });

  it("hides the wing detail below 22px so the favicon stays clean", () => {
    const { container } = render(<DuckMark size={20} />);
    const wing = container.querySelector('path[fill="#D89F2C"]');
    expect(wing).toBeNull();
  });
});

describe("DuckWordmark", () => {
  it("renders the brand name", () => {
    const { container } = render(<DuckWordmark />);
    expect(container.textContent).toBe("DuckOS");
  });
});
