/**
 * DuckOS V1 — application entry.
 *
 * Holds the auth state and wires the AppShell with a router. Three modes
 * are supported, matching the existing Startup handshake:
 *   - "legacy"   — token-only admin form (pre-identity installations)
 *   - "enroll"   — first-time owner enrolment via local identity
 *   - "login"    — sign in with the local DuckOS account
 *
 * Once authenticated, the AppShell takes over with the 99-page router.
 */
import { useMemo, useState, type FormEvent } from "react";
import { APIClient } from "./api/client";
import { AppShell } from "./shell/AppShell";
import { PagesRouter } from "./pages/Router";
import { IdentityLogin } from "./IdentityLogin";
import { ALL_PAGES, PAGES_BY_ROUTE } from "./pages/registry";
import { messages } from "./i18n/messages";
import { DuckMark, Icon } from "./icons";
import { ErrorBoundary } from "./components/ErrorBoundary";

export interface AppProps {
  identityMode: "legacy" | "enroll" | "login";
}

const SESSION_KEY = "duckos.v1.session.v1";

export function App({ identityMode }: AppProps) {
  const [token, setToken] = useState<string | null>(() => loadToken());
  const [error, setError] = useState<string>("");

  const client = useMemo(() => {
    return new APIClient({
      token: token ?? "",
      locale: "en",
      onFetch: () => {
        /* global hooks — pages write to the health summary directly */
      },
    });
  }, [token]);

  if (!token) {
    if (identityMode === "legacy") {
      return (
        <LegacyLogin
          error={error}
          onConnect={(t) => {
            persistToken(t);
            setToken(t);
            setError("");
          }}
        />
      );
    }
    return (
      <IdentityLogin
        enroll={identityMode === "enroll"}
        error={error}
        onConnect={(t) => {
          persistToken(t);
          setToken(t);
          setError("");
        }}
      />
    );
  }

  return (
    <ErrorBoundary>
      <AppShell
        token={token}
        client={client}
        initialRoute={PAGES_BY_ROUTE.has("/system/overview") ? "/system/overview" : ALL_PAGES[0].route}
        signOut={() => {
          persistToken(null);
          setToken(null);
        }}
        renderRoute={(route, navigate) => (
          <ErrorBoundary>
            <PagesRouter
              route={route}
              navigate={navigate}
              bundle={messages.en}
              client={client}
              token={token}
            />
          </ErrorBoundary>
        )}
      />
    </ErrorBoundary>
  );
}

function loadToken(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.sessionStorage.getItem(SESSION_KEY);
  } catch {
    return null;
  }
}

function persistToken(t: string | null): void {
  if (typeof window === "undefined") return;
  try {
    if (t) window.sessionStorage.setItem(SESSION_KEY, t);
    else window.sessionStorage.removeItem(SESSION_KEY);
  } catch {
    /* sessionStorage may be disabled */
  }
}

/* — Legacy token-only login (pre-identity installations) —
 * Preserved from V0 — does not invent data, asks for the admin token. */
function LegacyLogin({
  error,
  onConnect,
}: {
  error: string;
  onConnect: (token: string) => void;
}) {
  const [value, setValue] = useState("");
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (value.trim()) {
      onConnect(value.trim());
      setValue("");
    }
  }
  return (
    <div className="login-page">
      <div className="login-top">
        <div className="brand">
          <DuckMark />
          <span>
            Duck<span className="brand-light">OS</span>
            <small>YOUR NETWORK. CLEARLY.</small>
          </span>
        </div>
        <span className="eyebrow">LOCAL-FIRST CONTROL</span>
      </div>
      <main className="login-content">
        <div className="login-story">
          <div className="eyebrow accent-text">
            <span className="status-dot" /> DUCKOS CONTROL PLANE
          </div>
          <h1>
            A clear view.
            <br />
            <span>A calmer network.</span>
          </h1>
          <p>
            Your system, interfaces, and services — together in one considered workspace.
          </p>
        </div>
        <section className="login-card glass" aria-labelledby="connect-title">
          <div className="login-icon">
            <Icon name="lock" />
          </div>
          <div className="eyebrow">WELCOME TO DUCKOS</div>
          <h2 id="connect-title">Connect to your system</h2>
          <p>
            Use the administrator token from your DuckOS installation to view live Linux state.
          </p>
          <form onSubmit={submit}>
            <label htmlFor="access-token">Administrator token</label>
            <input
              id="access-token"
              type="password"
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="none"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              placeholder="Enter your access token"
              required
              aria-describedby="token-help"
            />
            {error && (
              <div className="login-error" role="alert">
                {error}
              </div>
            )}
            <button
              type="submit"
              className="button button-primary"
              disabled={!value.trim()}
            >
              Open workspace
              <Icon name="arrow" />
            </button>
          </form>
          <div id="token-help" className="token-help">
            <Icon name="lock" />
            <span>
              Your token stays in this tab's memory. Closing or refreshing the tab ends the session.
            </span>
          </div>
        </section>
      </main>
    </div>
  );
}