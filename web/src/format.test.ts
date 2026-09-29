import { describe, expect, it } from "vitest";
import {
  formatBytes,
  formatTime,
  formatUptime,
  percentage,
  percentLabel,
} from "./format";

describe("observation formatting", () => {
  it("uses binary byte units without disguising unavailable values", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1536)).toBe("1.5 KiB");
    expect(formatBytes(8 * 1024 ** 3)).toBe("8 GiB");
    expect(formatBytes(-1)).toBe("Unavailable");
    expect(formatBytes(NaN)).toBe("Unavailable");
  });
  it("formats boot-relative uptime without inventing a calendar date", () => {
    expect(formatUptime(0)).toBe("0m");
    expect(formatUptime(3661)).toBe("1h 1m");
    expect(formatUptime(90061)).toBe("1d 1h");
    expect(formatUptime(-1)).toBe("Unavailable");
  });
  it("distinguishes unknown capacity and utilization from measured zero", () => {
    expect(percentage(0, 0)).toBeNull();
    expect(percentage(0, 100)).toBe(0);
    expect(percentage(50, 100)).toBe(50);
    expect(percentage(200, 100)).toBe(100);
    expect(percentLabel(null)).toBe("Unavailable");
    expect(percentLabel(0)).toBe("0.0%");
    expect(formatTime("not-a-date")).toBe("Unavailable");
  });
});
