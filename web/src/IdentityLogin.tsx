import { useState, type FormEvent } from "react";
import { DuckMark, Icon } from "./icons";
import { LOCALES, type Locale } from "./i18n/messages";

// PreAuthSettings: a small language + theme selector that
// lives on the login / enrollment screen. It is gated only by
// the OS-level localStorage boundary, NOT by Owner session:
// the operator can pick a language or theme before any Owner
// exists. The selection takes effect on
// document.documentElement so the AppShell sees it on the very
// next render after the session connects.
type Theme = "system" | "light" | "dark";

function readLocale(): Locale {
  if (typeof localStorage !== "undefined") {
    const v = localStorage.getItem("duckos.locale");
    if (v === "en" || v === "ru" || v === "de" || v === "es") return v;
  }
  return "en";
}

function readTheme(): Theme {
  if (typeof localStorage !== "undefined") {
    const v = localStorage.getItem("duckos.theme");
    if (v === "system" || v === "light" || v === "dark") return v;
  }
  return "system";
}

function applyTheme(theme: Theme) {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  let resolved: "light" | "dark";
  if (theme === "system") {
    const prefersDark =
      typeof window !== "undefined" &&
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;
    resolved = prefersDark ? "dark" : "light";
  } else {
    resolved = theme;
  }
  root.setAttribute("data-theme", resolved);
  root.setAttribute("data-theme-pref", theme);
}

function applyLocale(locale: Locale) {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-locale", locale);
}

function PreAuthSettings() {
  const [locale, setLocaleState] = useState<Locale>(readLocale);
  const [theme, setThemeState] = useState<Theme>(readTheme);
  // Apply immediately on mount and on every change. The
  // effect runs synchronously before paint so the login
  // screen itself respects the operator's pick.
  applyLocale(locale);
  applyTheme(theme);
  function setLocale(next: Locale) {
    setLocaleState(next);
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("duckos.locale", next);
    }
    applyLocale(next);
  }
  function setTheme(next: Theme) {
    setThemeState(next);
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("duckos.theme", next);
    }
    applyTheme(next);
  }
  // Follow System changes at runtime via matchMedia.
  if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      if (readTheme() === "system") applyTheme("system");
    };
    if (typeof mq.addEventListener === "function") {
      mq.addEventListener("change", onChange);
    }
  }
  return (
    <div className="pre-auth-settings" aria-label="Display preferences">
      <label className="pre-auth-row">
        <span aria-hidden="true">Language</span>
        <select
          aria-label="Language"
          value={locale}
          onChange={(e) => setLocale(e.target.value as Locale)}
        >
          {LOCALES.map((l) => (
            <option key={l.id} value={l.id}>
              {l.label}
            </option>
          ))}
        </select>
      </label>
      <label className="pre-auth-row">
        <span aria-hidden="true">Theme</span>
        <select
          aria-label="Theme"
          value={theme}
          onChange={(e) => setTheme(e.target.value as Theme)}
        >
          <option value="system">System</option>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
        </select>
      </label>
    </div>
  );
}

export async function identityRequest(path: string, body: unknown, token = "") {
  const response = await fetch(`/api/v1/auth/${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    credentials: "omit",
    cache: "no-store",
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) {
    if (response.status === 429)
      throw new Error("Too many attempts. Wait a minute and try again.");
    if (response.status === 401)
      throw new Error("Credentials are invalid or the session expired.");
    if (response.status === 403)
      throw new Error(
        "Enrollment is closed or this operation is not permitted.",
      );
    throw new Error(
      "Request rejected. Check your input; passwords require 15–128 characters. If the error persists, inspect the local identity service.",
    );
  }
  return response.json();
}

export function IdentityLogin({
  enroll,
  error,
  onConnect,
}: {
  enroll: boolean;
  error: string;
  onConnect: (token: string) => void;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [bootstrap, setBootstrap] = useState("");
  const [failure, setFailure] = useState("");
  const [busy, setBusy] = useState(false);
  const [needsEnrollment, setNeedsEnrollment] = useState(enroll);
  async function submit(event: FormEvent) {
    event.preventDefault();
    setFailure("");
    if (needsEnrollment && password !== confirm) {
      setFailure("Passwords do not match.");
      return;
    }
    // Mirrors the previously-attached HTML pattern. The regex is
    // evaluated here (not as an HTML attribute) so it does not run
    // through Chrome's /v (unicodeSets) parser, which rejects the
    // character class. See UD-002 in
    // docs/ui/V1_LIVE_QUALIFICATION_STATUS.md.
    if (!/^[a-z][a-z0-9._-]{0,47}$/.test(username)) {
      setFailure("Username must start with a lowercase letter and contain only lowercase letters, digits, dots, underscores or dashes (1–48 characters).");
      return;
    }
    setBusy(true);
    try {
      if (needsEnrollment) {
        await identityRequest("enroll", { username, password }, bootstrap);
        setNeedsEnrollment(false);
        setBootstrap("");
        setConfirm("");
      }
      const result = await identityRequest("login", { username, password });
      if (typeof result.token !== "string" || result.token.length !== 64)
        throw new Error("Invalid authentication response.");
      setPassword("");
      onConnect(result.token);
    } catch (caught) {
      setFailure(
        caught instanceof Error
          ? caught.message
          : "Authentication unavailable.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="login-page">
      <div className="login-top">
        <div className="brand">
          <DuckMark />
          <span>
            DuckOS<small>YOUR NETWORK. CLEARLY.</small>
          </span>
        </div>
        <PreAuthSettings />
      </div>
      <main className="login-content">
        <div className="login-story">
          <div className="eyebrow accent-text">YOUR NETWORK. YOUR CONTROL.</div>
          <h1>
            {needsEnrollment ? "Make it yours." : "Welcome back."}
            <br />
            <span>Private by design.</span>
          </h1>
          <p>
            {needsEnrollment
              ? "Create the first Owner account on this installation. No default password. No external identity provider."
              : "Sign in with your local DuckOS account. Permissions are enforced by the backend for every operation."}
          </p>
          <div className="login-caption">
            Configuration changes remain protected by separate safety gates.
          </div>
        </div>
        <section className="login-card glass" aria-labelledby="identity-title">
          <div className="login-icon">
            <Icon name="lock" />
          </div>
          <div className="eyebrow">
            {needsEnrollment ? "FIRST-RUN ENROLLMENT" : "LOCAL ACCOUNT"}
          </div>
          <h2 id="identity-title">
            {needsEnrollment ? "Create your Owner" : "Sign in to DuckOS"}
          </h2>
          <form onSubmit={submit}>
            {needsEnrollment && (
              <>
                <label htmlFor="bootstrap">Installation bootstrap token</label>
                <input
                  id="bootstrap"
                  type="password"
                  autoComplete="off"
                  value={bootstrap}
                  onChange={(e) => setBootstrap(e.target.value)}
                  required
                  disabled={busy}
                />
              </>
            )}
            <label htmlFor="username">Username</label>
            <input
              id="username"
              autoComplete="username"
              autoCapitalize="none"
              spellCheck={false}
              // Removed the HTML `pattern=` attribute: Chrome 153
              // evaluates the attribute with the /v (unicodeSets)
              // flag and rejects `[a-z][a-z0-9._-]{0,47}` with
              // "Invalid character in character class", which logs
              // a console error per login attempt and confuses AT.
              // The same rule is enforced by JS in submit() below.
              // See UD-002 in docs/ui/V1_LIVE_QUALIFICATION_STATUS.md.
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              disabled={busy}
            />
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              autoComplete={
                needsEnrollment ? "new-password" : "current-password"
              }
              minLength={needsEnrollment ? 15 : undefined}
              maxLength={128}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              disabled={busy}
            />
            {needsEnrollment && (
              <>
                <label htmlFor="confirm">Confirm password</label>
                <input
                  id="confirm"
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  required
                  disabled={busy}
                />
                <p className="token-help">
                  Use a unique passphrase of 15–128 characters. Enrollment
                  closes permanently after the first Owner is created.
                </p>
              </>
            )}
            {(failure || error) && (
              <div className="login-error" role="alert">
                {failure || error}
              </div>
            )}
            <button
              className="button button-primary"
              type="submit"
              disabled={busy}
            >
              {busy
                ? "Authenticating…"
                : needsEnrollment
                  ? "Create Owner & sign in"
                  : "Open workspace"}
              <Icon name="arrow" />
            </button>
          </form>
          <div className="token-help">
            <Icon name="lock" />
            <span>
              Session token stays only in this tab’s memory. Idle sessions
              expire after 30 minutes. Disconnect revokes the session.
            </span>
          </div>
        </section>
      </main>
    </div>
  );
}
