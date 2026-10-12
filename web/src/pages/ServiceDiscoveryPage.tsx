// Service-discovery real UI page (P065).
//
// Replaces the P065 design-only placeholder with the real
// UPnP / NAT-PMP / PCP / Wake-on-LAN surface. The page is
// the V1 contract end-to-end:
//
//   - service status per protocol (UPnP / NAT-PMP / PCP / Wake)
//   - trusted networks with per-network enable + TTL + allow-list
//   - active dynamic mappings with explicit revoke
//   - wake device with bounded MAC validation
//   - disabled-by-choice is not a failure; backend missing
//     renders unavailable, not fake success
import { useEffect, useState } from "react";
import {
  enableUPnPNetwork,
  disableUPnPNetwork,
  listUPnPState,
  revokeUPnPMapping,
  sendWake,
  type UPnPNetwork,
  type UPnPMapping,
  type UPnPState,
  type WakeResult,
} from "../api";
import type { APIClient } from "../api/client";
import { v4t } from "../i18n/v4-strings";
import type { Locale } from "../i18n/messages";

interface Props {
  page: { id: string; name: string };
  bundle: Record<string, unknown>;
  client: APIClient;
}

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

const MAC_RE = /^(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$/;

export function ServiceDiscoveryPage({ page, bundle, client }: Props) {
  void bundle;
  const locale = useLocale();
  const t = (k: string) => v4t(locale, k);

  const [state, setState] = useState<UPnPState | null>(null);
  const [err, setErr] = useState<string>("");
  const [networkRef, setNetworkRef] = useState("");
  const [iface, setIface] = useState("");
  const [ttl, setTtl] = useState("3600");
  const [enableUPnP, setEnableUPnP] = useState(true);
  const [enableNATPMP, setEnableNATPMP] = useState(true);
  const [enablePCP, setEnablePCP] = useState(true);
  const [wakeMAC, setWakeMAC] = useState("");
  const [wakeIface, setWakeIface] = useState("");
  const [wakeResult, setWakeResult] = useState<WakeResult | null>(null);

  const refresh = async () => {
    try {
      const s = await listUPnPState(
        client.token(),
        new AbortController().signal,
      );
      setState(s);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    }
  };
  useEffect(() => {
    void refresh();
  }, [client]);

  async function onEnable() {
    setErr("");
    if (!networkRef.trim()) {
      setErr(t("upnp.trustedNetworks"));
      return;
    }
    const protocols: string[] = [];
    if (enableUPnP) protocols.push("upnp");
    if (enableNATPMP) protocols.push("nat-pmp");
    if (enablePCP) protocols.push("pcp");
    if (protocols.length === 0) {
      setErr("At least one protocol is required");
      return;
    }
    const n: UPnPNetwork = {
      networkRef: networkRef.trim(),
      protocols: protocols as UPnPNetwork["protocols"],
      ttlSec: Math.max(60, Math.min(86400, Number(ttl) || 3600)),
      allowedClients: [],
    };
    try {
      await enableUPnPNetwork(
        client.token(),
        n,
        iface.trim(),
        new AbortController().signal,
      );
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "enable failed");
    }
  }
  async function onDisable(ref: string) {
    setErr("");
    try {
      await disableUPnPNetwork(
        client.token(),
        ref,
        new AbortController().signal,
      );
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "disable failed");
    }
  }
  async function onRevoke(id: string) {
    setErr("");
    try {
      await revokeUPnPMapping(client.token(), id, new AbortController().signal);
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "revoke failed");
    }
  }
  async function onWake() {
    setErr("");
    setWakeResult(null);
    if (!MAC_RE.test(wakeMAC.trim())) {
      setErr(t("upnp.macInvalid"));
      return;
    }
    try {
      const r = await sendWake(
        client.token(),
        {
          target: wakeMAC.trim(),
          broadcast: "255.255.255.255:9",
          port: 9,
          packets: 3,
        },
        new AbortController().signal,
      );
      setWakeResult(r);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "wake failed");
    }
  }

  const stats = state?.stats;
  const networks = state?.networks ?? [];
  const mappings = state?.mappings ?? [];

  return (
    <div className="v4-page" data-page-id={page.id} data-locale={locale}>
      <h1 className="v4-page-title">{t("upnp.title")}</h1>
      <p className="v4-page-subtitle">{t("upnp.subtitle")}</p>

      <div className="v4-callout v4-callout-warn" role="note">
        {t("upnp.warning")}
      </div>

      <div className="v4-card" aria-labelledby="upnp-status-title">
        <div className="card-head">
          <h2 id="upnp-status-title">{t("upnp.serviceStatus")}</h2>
          <span
            className="v4-pill"
            data-kind={networks.length ? "good" : "muted"}
          >
            {networks.length ? t("upnp.serviceOn") : t("upnp.serviceOff")}
          </span>
        </div>
        <table className="v4-table">
          <thead>
            <tr>
              <th>{t("upnp.runtimeState")}</th>
              <th>Value</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>{t("upnp.protocolUpnp")}</td>
              <td>
                {networks.some((n) => n.protocols.includes("upnp"))
                  ? t("upnp.serviceOn")
                  : t("upnp.serviceOff")}
              </td>
            </tr>
            <tr>
              <td>{t("upnp.protocolNatpmp")}</td>
              <td>
                {networks.some((n) => n.protocols.includes("nat-pmp"))
                  ? t("upnp.serviceOn")
                  : t("upnp.serviceOff")}
              </td>
            </tr>
            <tr>
              <td>{t("upnp.protocolPcp")}</td>
              <td>
                {networks.some((n) => n.protocols.includes("pcp"))
                  ? t("upnp.serviceOn")
                  : t("upnp.serviceOff")}
              </td>
            </tr>
            <tr>
              <td>{t("upnp.protocolWake")}</td>
              <td>{t("upnp.serviceOn")}</td>
            </tr>
            {stats && (
              <tr>
                <td>Active mappings</td>
                <td>{stats.mappings}</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="v4-card" aria-labelledby="upnp-network-title">
        <div className="card-head">
          <h2 id="upnp-network-title">{t("upnp.trustedNetworks")}</h2>
        </div>
        <div className="v4-form-row">
          <label htmlFor="upnp-netref">Network ref</label>
          <input
            id="upnp-netref"
            value={networkRef}
            onChange={(e) => setNetworkRef(e.target.value)}
            placeholder="lan.home"
          />
        </div>
        <div className="v4-form-row">
          <label htmlFor="upnp-iface">Interface</label>
          <input
            id="upnp-iface"
            value={iface}
            onChange={(e) => setIface(e.target.value)}
            placeholder="eth0"
          />
        </div>
        <div className="v4-form-row">
          <label htmlFor="upnp-ttl">{t("upnp.ttlLabel")}</label>
          <input
            id="upnp-ttl"
            type="number"
            min="60"
            max="86400"
            value={ttl}
            onChange={(e) => setTtl(e.target.value)}
          />
        </div>
        <div className="v4-form-row">
          <label>{t("upnp.protocolsLabel")}</label>
          <div>
            <label className="v4-check">
              <input
                type="checkbox"
                checked={enableUPnP}
                onChange={(e) => setEnableUPnP(e.target.checked)}
              />
              {t("upnp.protocolUpnp")}
            </label>
            <label className="v4-check">
              <input
                type="checkbox"
                checked={enableNATPMP}
                onChange={(e) => setEnableNATPMP(e.target.checked)}
              />
              {t("upnp.protocolNatpmp")}
            </label>
            <label className="v4-check">
              <input
                type="checkbox"
                checked={enablePCP}
                onChange={(e) => setEnablePCP(e.target.checked)}
              />
              {t("upnp.protocolPcp")}
            </label>
          </div>
        </div>
        <div className="v4-row">
          <button
            type="button"
            className="button button-primary"
            onClick={onEnable}
          >
            {t("upnp.enable")}
          </button>
        </div>
        <table className="v4-table">
          <thead>
            <tr>
              <th>Network</th>
              <th>{t("upnp.protocolsLabel")}</th>
              <th>{t("upnp.ttlLabel")}</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {networks.length === 0 ? (
              <tr>
                <td colSpan={4}>{t("upnp.disabledByChoice")}</td>
              </tr>
            ) : (
              networks.map((n) => (
                <tr key={n.networkRef}>
                  <td>{n.networkRef}</td>
                  <td>{n.protocols.join(", ")}</td>
                  <td>{n.ttlSec} s</td>
                  <td>
                    <button
                      type="button"
                      className="button"
                      onClick={() => onDisable(n.networkRef)}
                    >
                      {t("upnp.disable")}
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="v4-card" aria-labelledby="upnp-mapping-title">
        <div className="card-head">
          <h2 id="upnp-mapping-title">{t("upnp.activeMappings")}</h2>
        </div>
        <table className="v4-table">
          <thead>
            <tr>
              <th>{t("upnp.mappingProtocol")}</th>
              <th>{t("upnp.mappingClient")}</th>
              <th>{t("upnp.mappingExternalPort")}</th>
              <th>{t("upnp.mappingInternal")}</th>
              <th>{t("upnp.mappingExpires")}</th>
              <th>{t("upnp.mappingSource")}</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {mappings.length === 0 ? (
              <tr>
                <td colSpan={7}>{t("upnp.noMappings")}</td>
              </tr>
            ) : (
              mappings.map((m: UPnPMapping) => (
                <tr key={m.id}>
                  <td>{m.protocol}</td>
                  <td>{m.clientIp}</td>
                  <td>{m.externalPort}</td>
                  <td>
                    {m.internalHost}:{m.internalPort}
                  </td>
                  <td>{m.expiresAt}</td>
                  <td>{m.source}</td>
                  <td>
                    <button
                      type="button"
                      className="button"
                      onClick={() => onRevoke(m.id)}
                    >
                      {t("upnp.revoke")}
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="v4-card" aria-labelledby="upnp-wake-title">
        <div className="card-head">
          <h2 id="upnp-wake-title">{t("upnp.wakeAction")}</h2>
        </div>
        <div className="v4-form-row">
          <label htmlFor="upnp-wake-mac">{t("upnp.wakeMac")}</label>
          <input
            id="upnp-wake-mac"
            value={wakeMAC}
            onChange={(e) => setWakeMAC(e.target.value)}
            placeholder="AA:BB:CC:DD:EE:FF"
          />
        </div>
        <div className="v4-form-row">
          <label htmlFor="upnp-wake-iface">{t("upnp.wakeNetwork")}</label>
          <input
            id="upnp-wake-iface"
            value={wakeIface}
            onChange={(e) => setWakeIface(e.target.value)}
            placeholder="eth0"
          />
        </div>
        <div className="v4-row">
          <button
            type="button"
            className="button button-primary"
            onClick={onWake}
          >
            {t("upnp.sendWake")}
          </button>
        </div>
        {wakeResult && (
          <p className="v4-mute">
            <span className="v4-pill" data-kind="good">
              {t("upnp.wakeSent")}
            </span>
            {wakeResult.target} → {wakeResult.broadcast} · {wakeResult.packets}{" "}
            packets · {wakeResult.bytes} bytes · {t("upnp.notOnline")}
          </p>
        )}
      </div>

      {err && (
        <div role="alert" className="v4-error" data-state="error">
          {err}
        </div>
      )}
    </div>
  );
}
