/**
 * DuckOS V1 — Safe Apply workflow hook.
 *
 * One typed hook every configurator uses. The flow is exactly the
 * product contract:
 *
 *   diff    → POST /api/v1/config/diff       (validate + review)
 *   apply   → POST /api/v1/config/apply     (start transaction)
 *   confirm → POST /api/v1/config/confirm/{id} (commit, before deadline)
 *   rollback→ POST /api/v1/config/rollback/{id} (cancel, optional reason)
 *
 * The hook exposes:
 *   - state machine with the four phases
 *   - commit-confirmed deadline (ms since epoch) and countdown
 *   - per-step success and failure surfaces
 *   - explicit `reset()` and `apply(intent)` actions
 *
 * The dashboard NEVER bypasses this hook to call a low-level
 * implementation API on a mutable system.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import type {
  ConfigApplyResponse,
  ConfigDiffResponse,
  ConfigIntent,
  ConfigStatus,
} from "../api/types";

export type Phase =
  | "idle"
  | "diffing"
  | "diffed"
  | "applying"
  | "awaiting-confirmation"
  | "confirming"
  | "rolling-back"
  | "applied"
  | "rolled-back"
  | "failed";

export interface SafeApplyState {
  phase: Phase;
  diff?: ConfigDiffResponse;
  transaction?: ConfigApplyResponse;
  status?: ConfigStatus;
  error?: { stage: Phase; message: string; status?: number };
  /** Epoch ms when confirm must complete, or null. */
  deadline: number | null;
  /** Seconds remaining to confirm. */
  remainingSeconds: number | null;
}

export interface SafeApplyApi {
  state: SafeApplyState;
  /** Run diff → apply, returning the diff so the operator can review. */
  diffAndApply(
    intent: ConfigIntent,
    options?: { confirmWindowSeconds?: number; note?: string },
  ): Promise<FetchResult<ConfigDiffResponse>>;
  confirm(): Promise<FetchResult<unknown>>;
  rollback(reason?: string): Promise<FetchResult<unknown>>;
  reset(): void;
  /** Refresh the live pending status from /api/v1/config/status. */
  refreshStatus(): Promise<void>;
}

export function useSafeApply(client: APIClient): SafeApplyApi {
  const [state, setState] = useState<SafeApplyState>({
    phase: "idle",
    deadline: null,
    remainingSeconds: null,
  });
  const timer = useRef<number | null>(null);

  const clearTimer = useCallback(() => {
    if (timer.current !== null) {
      window.clearInterval(timer.current);
      timer.current = null;
    }
  }, []);

  const tick = useCallback(() => {
    setState((s) => {
      if (!s.deadline) return s;
      const remainingMs = s.deadline - Date.now();
      const remainingSeconds = Math.max(0, Math.floor(remainingMs / 1000));
      return { ...s, remainingSeconds };
    });
  }, []);

  const startCountdown = useCallback(
    (deadline: number) => {
      void deadline;
      clearTimer();
      tick();
      timer.current = window.setInterval(tick, 1000);
    },
    [clearTimer, tick],
  );

  useEffect(() => clearTimer, [clearTimer]);

  const refreshStatus = useCallback(async () => {
    const r = await client.fetchConfigStatus();
    if (r.ok) {
      setState((s) => ({
        ...s,
        status: r.data,
        deadline: r.data.pendingDeadline ?? null,
        phase:
          r.data.pendingTransactionId && s.phase === "idle"
            ? "awaiting-confirmation"
            : s.phase,
      }));
      if (r.data.pendingDeadline) startCountdown(r.data.pendingDeadline);
    }
  }, [client, startCountdown]);

  const diffAndApply = useCallback(
    async (
      intent: ConfigIntent,
      options?: { confirmWindowSeconds?: number; note?: string },
    ): Promise<FetchResult<ConfigDiffResponse>> => {
      setState((s) => ({ ...s, phase: "diffing", error: undefined }));
      const d = await client.configDiff(intent);
      if (!d.ok) {
        setState((s) => ({ ...s, phase: "failed", error: { stage: "diffing", message: d.message, status: d.status } }));
        return d;
      }
      setState((s) => ({ ...s, phase: "diffed", diff: d.data }));

      // Server issues block apply. We still allow the user to see the
      // diff and choose to attempt apply; the server is the gate.
      setState((s) => ({ ...s, phase: "applying" }));
      // Compute a stable hash of the diff so the apply call can be
      // bound to the exact diff the operator reviewed. The Engine
      // requires If-Match-style alignment in production.
      const diffHash = await subtleHash(JSON.stringify(d.data));
      const a = await client.configApply(diffHash, intent, options);
      if (!a.ok) {
        setState((s) => ({
          ...s,
          phase: "failed",
          error: { stage: "applying", message: a.message, status: a.status },
        }));
        return d;
      }
      setState((s) => ({
        ...s,
        phase: "awaiting-confirmation",
        transaction: a.data,
        deadline: a.data.deadline,
        remainingSeconds: Math.max(0, Math.floor((a.data.deadline - Date.now()) / 1000)),
      }));
      startCountdown(a.data.deadline);
      return d;
    },
    [client, startCountdown],
  );

  const confirm = useCallback(async (): Promise<FetchResult<unknown>> => {
    const tx = state.transaction;
    if (!tx) {
      setState((s) => ({
        ...s,
        phase: "failed",
        error: { stage: "confirming", message: "No active transaction to confirm." },
      }));
      return { ok: false, reason: "failed", message: "No active transaction." };
    }
    setState((s) => ({ ...s, phase: "confirming" }));
    clearTimer();
    const r = await client.configConfirm(tx.transactionId);
    if (!r.ok) {
      setState((s) => ({
        ...s,
        phase: "failed",
        error: { stage: "confirming", message: r.message, status: r.status },
      }));
      return r;
    }
    setState((s) => ({ ...s, phase: "applied", transaction: undefined, deadline: null, remainingSeconds: null }));
    return r;
  }, [state.transaction, client, clearTimer]);

  const rollback = useCallback(
    async (reason?: string): Promise<FetchResult<unknown>> => {
      const tx = state.transaction;
      if (!tx) {
        setState((s) => ({
          ...s,
          phase: "failed",
          error: { stage: "rolling-back", message: "No active transaction to roll back." },
        }));
        return { ok: false, reason: "failed", message: "No active transaction." };
      }
      setState((s) => ({ ...s, phase: "rolling-back" }));
      clearTimer();
      const r = await client.configRollback(tx.transactionId, reason);
      if (!r.ok) {
        setState((s) => ({
          ...s,
          phase: "failed",
          error: { stage: "rolling-back", message: r.message, status: r.status },
        }));
        return r;
      }
      setState((s) => ({ ...s, phase: "rolled-back", transaction: undefined, deadline: null, remainingSeconds: null }));
      return r;
    },
    [state.transaction, client, clearTimer],
  );

  const reset = useCallback(() => {
    clearTimer();
    setState({ phase: "idle", deadline: null, remainingSeconds: null });
  }, [clearTimer]);

  return { state, diffAndApply, confirm, rollback, reset, refreshStatus };
}

async function subtleHash(input: string): Promise<string> {
  if (typeof crypto === "undefined" || !crypto.subtle) {
    // Fallback for older environments; the Engine tolerates plain hashes
    // when the binding is not strict.
    let h = 0;
    for (let i = 0; i < input.length; i++) {
      h = (h * 31 + input.charCodeAt(i)) | 0;
    }
    return `xxhash-${(h >>> 0).toString(16)}`;
  }
  const data = new TextEncoder().encode(input);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}