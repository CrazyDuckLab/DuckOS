// Speed Lab real UI page.
//
// Replaces the P090 design-only placeholder. The page is the
// V1_SPEEDLAB contract end-to-end:
//
//   - probe kinds: Latency / HTTP / Throughput
//   - input: host + port OR URL
//   - output: bounded redacted summary (no raw samples)
//   - states: pending / running / completed / failed / cancelled
//   - the UI never calls TCP-handshake RTT "ping"
//   - the UI never calls local-server throughput "Internet speed"
import { useState } from "react";
import {
  cancelSpeedLabProbe,
  getSpeedLabProbe,
  startSpeedLabProbe,
  type SpeedLabKind,
  type SpeedLabRequest,
  type SpeedLabSummary,
} from "../api";
import type { APIClient } from "../api/client";
import { v4t } from "../i18n/v4-strings";
import type { Locale } from "../i18n/messages";

interface Props {
  page: { id: string; name: string };
  bundle: Record<string, unknown>;
  client: APIClient;
}

type ProbeState =
  | { kind: "idle" }
  | { kind: "running"; id: string; startedAt?: string }
  | { kind: "completed"; id: string; summary: SpeedLabSummary }
  | { kind: "failed"; id: string; message: string };

function useLocale(): Locale {
  if (typeof document === "undefined") return "en";
  const html = document.documentElement.getAttribute("data-locale");
  if (html === "ru" || html === "de" || html === "es" || html === "en")
    return html;
  const shell =
    document.querySelector(".duck-shell")?.getAttribute("data-locale") ?? null;
  if (shell === "ru" || shell === "de" || shell === "es" || shell === "en")
    return shell;
  return "en";
}

function fmtMicros(us: number): string {
  if (us < 1000) return `${us} µs`;
  if (us < 1000 * 1000) return `${(us / 1000).toFixed(2)} ms`;
  return `${(us / 1000 / 1000).toFixed(3)} s`;
}

function fmtBps(bps: number): { primary: string; mbps: string } {
  const mbps = (bps * 8) / 1_000_000;
  if (mbps >= 1) {
    return {
      primary: `${mbps.toFixed(2)} Mbps`,
      mbps: `${mbps.toFixed(2)}`,
    };
  }
  const kbps = (bps * 8) / 1000;
  return {
    primary: `${kbps.toFixed(0)} kbps`,
    mbps: `${mbps.toFixed(2)}`,
  };
}

function fmtBytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(2)} MiB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KiB`;
  return `${n} B`;
}

export function SpeedLabPage({ page, bundle, client }: Props) {
  void bundle;
  const locale = useLocale();
  const [probeKind, setProbeKind] = useState<SpeedLabKind>("latency");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [url, setUrl] = useState("");
  const [duration, setDuration] = useState("2");
  const [maxSamples, setMaxSamples] = useState("32");
  const [label, setLabel] = useState("");
  const [state, setState] = useState<ProbeState>({ kind: "idle" });
  const [lastError, setLastError] = useState<string>("");
  const t = (key: string): string => v4t(locale, key);

  async function start() {
    setLastError("");
    const dur = Math.max(1, Math.min(30, Math.floor(Number(duration))));
    const samples = Math.max(1, Math.min(1024, Math.floor(Number(maxSamples))));
    let body: SpeedLabRequest;
    if (probeKind === "throughput" || probeKind === "http") {
      const cleanUrl = url.trim();
      if (!/^https?:\/\//.test(cleanUrl)) {
        setLastError(t("speedlab.invalidUrl"));
        return;
      }
      body = {
        kind: probeKind,
        target: { url: cleanUrl, label: label.trim() || undefined },
        duration: dur * 1000,
        maxSamples: samples,
      };
    } else {
      if (!host.trim()) {
        setLastError(t("speedlab.hostRequired"));
        return;
      }
      const p = Math.max(1, Math.min(65535, Number(port) || 0));
      if (p == 0) {
        setLastError(t("speedlab.portRequired"));
        return;
      }
      body = {
        kind: "latency",
        target: {
          host: host.trim(),
          port: p,
          label: label.trim() || undefined,
        },
        duration: dur * 1000,
        maxSamples: samples,
      };
    }
    try {
      const ctrl = new AbortController();
      const ref = await startSpeedLabProbe(client.token(), body, ctrl.signal);
      setState({
        kind: "running",
        id: ref.id,
        startedAt: ref.startedAt,
      });
      const tick = async () => {
        try {
          const got = await getSpeedLabProbe(
            client.token(),
            ref.id,
            ctrl.signal,
          );
          if (got.state === "done" || got.state === "failed") {
            setState({ kind: "completed", id: ref.id, summary: got.summary });
          } else {
            setTimeout(tick, 1000);
          }
        } catch (e) {
          setLastError(
            e instanceof Error ? e.message : "Probe status fetch failed",
          );
        }
      };
      setTimeout(tick, 1000);
    } catch (e) {
      setLastError(e instanceof Error ? e.message : "Start failed");
    }
  }

  async function cancel() {
    if (state.kind !== "running") return;
    try {
      await cancelSpeedLabProbe(
        client.token(),
        state.id,
        new AbortController().signal,
      );
      setLastError("");
    } catch (e) {
      setLastError(e instanceof Error ? e.message : "Cancel failed");
    }
  }

  return (
    <div className="v4-page" data-page-id={page.id} data-locale={locale}>
      <h1 className="v4-page-title">{t("speedlab.title")}</h1>
      <p className="v4-page-subtitle">{t("speedlab.subtitle")}</p>

      <div className="v4-card" aria-labelledby="speedlab-form-title">
        <div className="card-head">
          <h2 id="speedlab-form-title">{t("speedlab.newProbe")}</h2>
        </div>
        <div className="v4-form-row">
          <label htmlFor="speedlab-kind">{t("speedlab.kind")}</label>
          <select
            id="speedlab-kind"
            value={probeKind}
            onChange={(e) => setProbeKind(e.target.value as SpeedLabKind)}
            disabled={state.kind === "running"}
          >
            <option value="latency">{t("speedlab.kindLatency")}</option>
            <option value="http">{t("speedlab.kindHttp")}</option>
            <option value="throughput">{t("speedlab.kindThroughput")}</option>
          </select>
        </div>
        {probeKind === "latency" && (
          <div className="v4-form-row">
            <label htmlFor="speedlab-host">{t("speedlab.host")}</label>
            <input
              id="speedlab-host"
              type="text"
              placeholder="example.com"
              value={host}
              onChange={(e) => setHost(e.target.value)}
              disabled={state.kind === "running"}
            />
          </div>
        )}
        {probeKind === "latency" && (
          <div className="v4-form-row">
            <label htmlFor="speedlab-port">{t("speedlab.port")}</label>
            <input
              id="speedlab-port"
              type="number"
              min="1"
              max="65535"
              placeholder="443"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              disabled={state.kind === "running"}
            />
          </div>
        )}
        {(probeKind === "http" || probeKind === "throughput") && (
          <div className="v4-form-row">
            <label htmlFor="speedlab-url">{t("speedlab.url")}</label>
            <input
              id="speedlab-url"
              type="text"
              placeholder="https://example.com/file.dat"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              disabled={state.kind === "running"}
            />
          </div>
        )}
        <div className="v4-form-row">
          <label htmlFor="speedlab-duration">{t("speedlab.duration")}</label>
          <input
            id="speedlab-duration"
            type="number"
            min="1"
            max="30"
            value={duration}
            onChange={(e) => setDuration(e.target.value)}
            disabled={state.kind === "running"}
          />
        </div>
        {probeKind !== "throughput" && (
          <div className="v4-form-row">
            <label htmlFor="speedlab-samples">{t("speedlab.samples")}</label>
            <input
              id="speedlab-samples"
              type="number"
              min="1"
              max="1024"
              value={maxSamples}
              onChange={(e) => setMaxSamples(e.target.value)}
              disabled={state.kind === "running"}
            />
          </div>
        )}
        <div className="v4-form-row">
          <label htmlFor="speedlab-label">{t("speedlab.label")}</label>
          <input
            id="speedlab-label"
            type="text"
            placeholder="operator label"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            disabled={state.kind === "running"}
          />
        </div>
        <div className="v4-row">
          {state.kind !== "running" ? (
            <button
              type="button"
              className="button button-primary"
              onClick={start}
              data-action="start"
            >
              {t("speedlab.start")}
            </button>
          ) : (
            <button
              type="button"
              className="button"
              onClick={cancel}
              data-action="cancel"
            >
              {t("speedlab.cancel")}
            </button>
          )}
        </div>
        {lastError && (
          <div role="alert" className="v4-error" data-state="error">
            {lastError}
          </div>
        )}
      </div>

      <div className="v4-card" aria-labelledby="speedlab-state-title">
        <div className="card-head">
          <h2 id="speedlab-state-title">{t("speedlab.state")}</h2>
        </div>
        <StateView state={state} locale={locale} />
      </div>

      {state.kind === "completed" && (
        <div className="v4-card" aria-labelledby="speedlab-results-title">
          <div className="card-head">
            <h2 id="speedlab-results-title">{t("speedlab.results")}</h2>
          </div>
          <ResultsView summary={state.summary} locale={locale} />
        </div>
      )}
    </div>
  );
}

function StateView({ state, locale }: { state: ProbeState; locale: Locale }) {
  const t = (k: string) => v4t(locale, k);
  if (state.kind === "idle") {
    return (
      <p className="v4-mute">
        <span className="v4-pill" data-kind="muted">
          {t("speedlab.stateIdle")}
        </span>
        {t("speedlab.emptyState")}
      </p>
    );
  }
  if (state.kind === "running") {
    return (
      <p className="v4-mute">
        <span className="v4-status-pill" data-tone="busy">
          {t("speedlab.stateRunning")}
        </span>
        {t("speedlab.probeId")} <code className="v4-mono">{state.id}</code>
        {state.startedAt && (
          <>
            {" · "}
            {t("speedlab.startedAt")}{" "}
            <time dateTime={state.startedAt}>{state.startedAt}</time>
          </>
        )}
      </p>
    );
  }
  if (state.kind === "failed") {
    return (
      <p className="v4-mute">
        <span className="v4-status-pill" data-tone="bad">
          {t("speedlab.stateFailed")}
        </span>{" "}
        {state.message}
      </p>
    );
  }
  return (
    <p className="v4-mute">
      <span className="v4-status-pill" data-tone="good">
        {t("speedlab.stateCompleted")}
      </span>
      {t("speedlab.probeId")} <code className="v4-mono">{state.id}</code>
      {state.summary.finishedAt && (
        <>
          {" · "}
          {t("speedlab.finishedAt")}{" "}
          <time dateTime={state.summary.finishedAt}>
            {state.summary.finishedAt}
          </time>
        </>
      )}
    </p>
  );
}

function ResultsView({
  summary,
  locale,
}: {
  summary: SpeedLabSummary;
  locale: Locale;
}) {
  const t = (k: string) => v4t(locale, k);
  if (summary.kind === "latency" && summary.latency) {
    const l = summary.latency;
    return (
      <table className="v4-table" data-result="latency">
        <thead>
          <tr>
            <th>{t("speedlab.colMetric")}</th>
            <th>{t("speedlab.colValue")}</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>{t("speedlab.resultSamples")}</td>
            <td>{l.count}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultMin")}</td>
            <td>{fmtMicros(l.minMicros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultAvg")}</td>
            <td>{fmtMicros(l.avgMicros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultMax")}</td>
            <td>{fmtMicros(l.maxMicros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultP50")}</td>
            <td>{fmtMicros(l.p50Micros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultP95")}</td>
            <td>{fmtMicros(l.p95Micros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultP99")}</td>
            <td>{fmtMicros(l.p99Micros)}</td>
          </tr>
        </tbody>
      </table>
    );
  }
  if (summary.kind === "http" && summary.latency) {
    const l = summary.latency;
    return (
      <table className="v4-table" data-result="http">
        <thead>
          <tr>
            <th>{t("speedlab.colMetric")}</th>
            <th>{t("speedlab.colValue")}</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>{t("speedlab.resultSamples")}</td>
            <td>{l.count}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultMin")}</td>
            <td>{fmtMicros(l.minMicros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultAvg")}</td>
            <td>{fmtMicros(l.avgMicros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultMax")}</td>
            <td>{fmtMicros(l.maxMicros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultP50")}</td>
            <td>{fmtMicros(l.p50Micros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultP95")}</td>
            <td>{fmtMicros(l.p95Micros)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultP99")}</td>
            <td>{fmtMicros(l.p99Micros)}</td>
          </tr>
        </tbody>
      </table>
    );
  }
  if (summary.kind === "throughput" && summary.throughput) {
    const tp = summary.throughput;
    const fmt = fmtBps(tp.bytesPerSecond);
    return (
      <table className="v4-table" data-result="throughput">
        <thead>
          <tr>
            <th>{t("speedlab.colMetric")}</th>
            <th>{t("speedlab.colValue")}</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>{t("speedlab.resultBytes")}</td>
            <td>{fmtBytes(tp.bytes)}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultSeconds")}</td>
            <td>{tp.seconds} s</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultBps")}</td>
            <td>{tp.bytesPerSecond.toLocaleString()}</td>
          </tr>
          <tr>
            <td>{t("speedlab.resultMbps")}</td>
            <td>{fmt.primary}</td>
          </tr>
          <tr>
            <td>{t("speedlab.colCaveat")}</td>
            <td>{t("speedlab.caveat")}</td>
          </tr>
        </tbody>
      </table>
    );
  }
  return (
    <p className="v4-mute">No measurements were produced for this probe.</p>
  );
}
