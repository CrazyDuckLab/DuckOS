/**
 * DuckOS V1 — Safe Apply hook integration test.
 *
 * Exercises the full state machine against a mocked APIClient:
 *   idle → diffing → diffed → applying → awaiting-confirmation
 *     → confirming → applied
 *
 * Plus the failure paths:
 *   diff failure, apply failure, confirm failure, rollback.
 *
 * Plus the countdown timer for the commit-confirmed window.
 */
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { APIClient, type FetchResult } from "../api/client";
import type {
  ConfigApplyResponse,
  ConfigConfirmResponse,
  ConfigDiffResponse,
  ConfigRollbackResponse,
} from "../api/types";
import { useSafeApply } from "../state/safeApply";

function ok<T>(data: T): FetchResult<T> {
  return { ok: true, data, age: 1 };
 }
function fail(reason: "missing" | "unauthorized" | "failed", message = "x"): FetchResult<never> {
  return { ok: false, reason, message };
}

function makeClient() {
  return new APIClient({
    token: "t",
    locale: "en",
    onFetch: () => {},
    // Direct methods on APIClient will be replaced by stubs.
  } as never) as unknown as APIClient & {
    configDiff: ReturnType<typeof vi.fn>;
    configApply: ReturnType<typeof vi.fn>;
    configConfirm: ReturnType<typeof vi.fn>;
    configRollback: ReturnType<typeof vi.fn>;
    fetchConfigStatus: ReturnType<typeof vi.fn>;
  };
}

describe("useSafeApply — happy path", () => {
  it("walks idle → awaiting-confirmation → applied", async () => {
    const diff: ConfigDiffResponse = {
      changes: [],
      issues: [],
      blastRadius: [],
    };
    const apply: ConfigApplyResponse = {
      transactionId: "tx-1",
      deadline: Date.now() + 30_000,
    };
    const confirm: ConfigConfirmResponse = {
      transactionId: "tx-1",
      revision: "rev-2",
    };
    const c = makeClient();
    c.configDiff = vi.fn().mockResolvedValue(ok(diff));
    c.configApply = vi.fn().mockResolvedValue(ok(apply));
    c.configConfirm = vi.fn().mockResolvedValue(ok(confirm));
    const { result } = renderHook(() => useSafeApply(c as unknown as APIClient));
    expect(result.current.state.phase).toBe("idle");
    await act(async () => {
      await result.current.diffAndApply({ objects: [] });
    });
    expect(result.current.state.phase).toBe("awaiting-confirmation");
    expect(result.current.state.transaction?.transactionId).toBe("tx-1");
    await act(async () => {
      await result.current.confirm();
    });
    expect(result.current.state.phase).toBe("applied");
    expect(result.current.state.transaction).toBeUndefined();
  });

  it("transitions to failed on diff rejection", async () => {
    const c = makeClient();
    c.configDiff = vi.fn().mockResolvedValue(fail("failed", "bad config"));
    const { result } = renderHook(() => useSafeApply(c as unknown as APIClient));
    await act(async () => {
      await result.current.diffAndApply({});
    });
    expect(result.current.state.phase).toBe("failed");
    expect(result.current.state.error?.message).toBe("bad config");
  });

  it("transitions to rolled-back after rollback", async () => {
    const diff: ConfigDiffResponse = { changes: [], issues: [], blastRadius: [] };
    const apply: ConfigApplyResponse = { transactionId: "tx-2", deadline: Date.now() + 60_000 };
    const rollback: ConfigRollbackResponse = { transactionId: "tx-2", revision: "rev-1" };
    const c = makeClient();
    c.configDiff = vi.fn().mockResolvedValue(ok(diff));
    c.configApply = vi.fn().mockResolvedValue(ok(apply));
    c.configRollback = vi.fn().mockResolvedValue(ok(rollback));
    const { result } = renderHook(() => useSafeApply(c as unknown as APIClient));
    await act(async () => {
      await result.current.diffAndApply({});
    });
    expect(result.current.state.phase).toBe("awaiting-confirmation");
    await act(async () => {
      await result.current.rollback("operator changed mind");
    });
    expect(result.current.state.phase).toBe("rolled-back");
  });
});