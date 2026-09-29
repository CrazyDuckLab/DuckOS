import { useEffect, useState } from "react";
import { App } from "./App";
import { DuckMark } from "./icons";

export function Startup() {
  const [mode, setMode] = useState<"legacy" | "enroll" | "login" | null>(null);
  const [error, setError] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 10000);
    setError(false);
    fetch("/api/v1/auth/status", {
      cache: "no-store",
      credentials: "omit",
      signal: controller.signal,
    })
      .then(async (response) => {
        if (response.status === 404) {
          setMode("legacy");
          return;
        }
        if (!response.ok) throw new Error("Identity unavailable");
        const status = await response.json();
        if (
          status.mode !== "local-sessions" ||
          typeof status.enrolled !== "boolean"
        )
          throw new Error("Invalid identity status");
        setMode(status.enrolled ? "login" : "enroll");
      })
      .catch(() => {
        if (active) setError(true);
      })
      .finally(() => clearTimeout(timer));
    return () => {
      active = false;
      clearTimeout(timer);
      controller.abort();
    };
  }, [retry]);
  if (mode) return <App identityMode={mode} />;
  return (
    <main className="login-page">
      <section className="login-card glass" aria-live="polite">
        <DuckMark />
        <h1>DuckOS</h1>
        <p>
          {error
            ? "Identity service is unavailable. No fallback credentials were accepted."
            : "Checking local identity…"}
        </p>
        <button
          className="button button-primary"
          onClick={() => setRetry((value) => value + 1)}
        >
          Retry connection
        </button>
      </section>
    </main>
  );
}
