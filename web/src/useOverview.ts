import { useCallback, useEffect, useRef, useState } from "react";
import type { Overview } from "../../shared/api/types";
import { fetchOverview } from "./api";

export function useOverview(token: string) {
  const [data, setData] = useState<Overview | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [loading, setLoading] = useState(false);
  const [now, setNow] = useState(Date.now);
  const refreshRef = useRef<() => void>(() => {});

  useEffect(() => {
    setData(null);
    setError(null);
    if (!token) {
      setLoading(false);
      refreshRef.current = () => {};
      return;
    }
    let stopped = false;
    let inFlight = false;
    let controller: AbortController | undefined;
    async function observe() {
      if (inFlight || stopped) return;
      inFlight = true;
      setLoading(true);
      controller = new AbortController();
      const timeout = window.setTimeout(() => controller?.abort(), 15000);
      try {
        const overview = await fetchOverview(token, controller.signal);
        if (!stopped) {
          setData(overview);
          setError(null);
          setNow(Date.now());
        }
      } catch (cause) {
        if (!stopped) {
          setError(
            cause instanceof Error && cause.name !== "AbortError"
              ? cause
              : new Error(
                  "The system did not respond within 15 seconds. The last observation may be out of date.",
                ),
          );
        }
      } finally {
        window.clearTimeout(timeout);
        inFlight = false;
        if (!stopped) setLoading(false);
      }
    }
    refreshRef.current = () => {
      void observe();
    };
    void observe();
    const timer = window.setInterval(() => {
      setNow(Date.now());
      void observe();
    }, 5000);
    return () => {
      stopped = true;
      controller?.abort();
      window.clearInterval(timer);
      refreshRef.current = () => {};
    };
  }, [token]);

  const refresh = useCallback(() => refreshRef.current(), []);
  const observedTime = data ? new Date(data.observedAt).getTime() : NaN;
  const stale =
    data !== null &&
    (error !== null ||
      !Number.isFinite(observedTime) ||
      now - observedTime > 20000);
  return { data, error, loading, refresh, stale };
}
