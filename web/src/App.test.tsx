import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useOverview } from "./useOverview";
import { overviewFixture } from "./test/fixture";

function respond(data = overviewFixture()) {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("useOverview polling", () => {
  it("polls every five seconds and stops after unmount", async () => {
    vi.useFakeTimers();
    const fetch = vi.fn().mockImplementation(async () => respond());
    vi.stubGlobal("fetch", fetch);
    const { result, unmount } = renderHook(() =>
      useOverview("synthetic-test-token"),
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(result.current.data?.system.hostname).toBe("synthetic-test-router");
    expect(fetch).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(fetch).toHaveBeenCalledTimes(2);
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});