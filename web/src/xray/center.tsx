/**
 * DuckOS — Xray Center management page (P050).
 *
 * Functional server / client / subscription management on the typed
 * /api/v1/xray surface. Every enabled control performs a real
 * supported action; destructive actions require an explicit confirm
 * dialog. The protocol × transport × security capability matrix is
 * mirrored from the Go engine (APIClient statics) so invalid
 * combinations are refused before any request. Help content is
 * offline, deterministic and localized (en/ru/de/es).
 */
import { useCallback, useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import type { XrayServerView, XrayUserView } from "../api/types";
import type { Locale, Messages } from "../i18n/messages";
import type { PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";

type Dict = {
  title: string;
  subtitle: string;
  sectionServers: string;
  sectionClients: string;
  sectionSubscriptions: string;
  create: string;
  cancel: string;
  delete: string;
  confirmDelete: string;
  confirmDeleteBody: string;
  name: string;
  protocol: string;
  transport: string;
  security: string;
  listen: string;
  port: string;
  ssMethod: string;
  server: string;
  uuid: string;
  generate: string;
  profileIds: string;
  emptyServers: string;
  emptyClients: string;
  emptySubscriptions: string;
  errName: string;
  errListen: string;
  errPort: string;
  errMatrix: string;
  errSubmit: string;
  helpTitle: string;
  users: string;
};

const en: Dict = {
  title: "Xray Center",
  subtitle: "Manage Xray servers, clients and subscriptions.",
  sectionServers: "Servers (inbounds)",
  sectionClients: "Clients",
  sectionSubscriptions: "Subscriptions",
  create: "Create",
  cancel: "Cancel",
  delete: "Delete",
  confirmDelete: "Confirm deletion",
  confirmDeleteBody: "This cannot be undone. Delete",
  name: "Name",
  protocol: "Protocol",
  transport: "Transport",
  security: "Security",
  listen: "Listen address",
  port: "Port",
  ssMethod: "Cipher",
  server: "Server",
  uuid: "UUID",
  generate: "Generate",
  profileIds: "Profile IDs (comma-separated)",
  emptyServers: "No Xray servers are configured yet.",
  emptyClients: "No clients for this server yet.",
  emptySubscriptions: "No subscriptions yet.",
  errName: "Name: lowercase letters, digits, dot, dash, underscore (max 48).",
  errListen: "Listen address is required (host:port or host).",
  errPort: "Port must be 1..65535.",
  errMatrix: "This protocol/transport/security combination is not supported.",
  errSubmit: "The API rejected the request.",
  helpTitle: "What is this?",
  users: "clients",
};

const ru: Dict = {
  title: "Xray Center",
  subtitle: "Управление серверами, клиентами и подписками Xray.",
  sectionServers: "Серверы (входящие)",
  sectionClients: "Клиенты",
  sectionSubscriptions: "Подписки",
  create: "Создать",
  cancel: "Отмена",
  delete: "Удалить",
  confirmDelete: "Подтвердите удаление",
  confirmDeleteBody: "Действие необратимо. Удалить",
  name: "Имя",
  protocol: "Протокол",
  transport: "Транспорт",
  security: "Защита",
  listen: "Адрес прослушивания",
  port: "Порт",
  ssMethod: "Шифр",
  server: "Сервер",
  uuid: "UUID",
  generate: "Сгенерировать",
  profileIds: "ID профилей (через запятую)",
  emptyServers: "Серверы Xray ещё не настроены.",
  emptyClients: "Для этого сервера пока нет клиентов.",
  emptySubscriptions: "Подписок пока нет.",
  errName:
    "Имя: строчные латинские буквы, цифры, точка, дефис, подчёркивание (до 48).",
  errListen: "Укажите адрес прослушивания (host:port или host).",
  errPort: "Порт должен быть 1..65535.",
  errMatrix: "Эта комбинация протокол/транспорт/защита не поддерживается.",
  errSubmit: "API отклонил запрос.",
  helpTitle: "Что это?",
  users: "клиентов",
};

const de: Dict = {
  ...en,
  subtitle: "Xray-Server, Clients und Abonnements verwalten.",
  sectionServers: "Server (eingehend)",
  sectionClients: "Clients",
  sectionSubscriptions: "Abonnements",
  create: "Erstellen",
  cancel: "Abbrechen",
  delete: "Löschen",
  confirmDelete: "Löschen bestätigen",
  confirmDeleteBody: "Dies kann nicht widerrufen werden. Löschen",
  name: "Name",
  protocol: "Protokoll",
  transport: "Transport",
  security: "Sicherheit",
  listen: "Listen-Adresse",
  port: "Port",
  ssMethod: "Chiffre",
  server: "Server",
  uuid: "UUID",
  generate: "Generieren",
  profileIds: "Profil-IDs (kommagetrennt)",
  emptyServers: "Noch keine Xray-Server konfiguriert.",
  emptyClients: "Für diesen Server gibt es noch keine Clients.",
  emptySubscriptions: "Noch keine Abonnements.",
  errName:
    "Name: Kleinbuchstaben, Ziffern, Punkt, Bindestrich, Unterstrich (max. 48).",
  errListen: "Listen-Adresse erforderlich (host:port oder host).",
  errPort: "Port muss 1..65535 sein.",
  errMatrix:
    "Diese Protokoll-/Transport-/Sicherheitskombination wird nicht unterstützt.",
  errSubmit: "Die API hat die Anfrage abgelehnt.",
  helpTitle: "Was ist das?",
  users: "Clients",
};

const es: Dict = {
  ...en,
  subtitle: "Gestione servidores, clientes y suscripciones Xray.",
  sectionServers: "Servidores (entrantes)",
  sectionClients: "Clientes",
  sectionSubscriptions: "Suscripciones",
  create: "Crear",
  cancel: "Cancelar",
  delete: "Eliminar",
  confirmDelete: "Confirmar eliminación",
  confirmDeleteBody: "Esto no se puede deshacer. Eliminar",
  name: "Nombre",
  protocol: "Protocolo",
  transport: "Transporte",
  security: "Seguridad",
  listen: "Dirección de escucha",
  port: "Puerto",
  ssMethod: "Cifrado",
  server: "Servidor",
  uuid: "UUID",
  generate: "Generar",
  profileIds: "IDs de perfil (separados por comas)",
  emptyServers: "Aún no hay servidores Xray configurados.",
  emptyClients: "Aún no hay clientes para este servidor.",
  emptySubscriptions: "Aún no hay suscripciones.",
  errName: "Nombre: minúsculas, dígitos, punto, guion, guion bajo (máx. 48).",
  errListen: "Se requiere la dirección de escucha (host:puerto o host).",
  errPort: "El puerto debe ser 1..65535.",
  errMatrix:
    "Esta combinación de protocolo/transporte/seguridad no está soportada.",
  errSubmit: "La API rechazó la solicitud.",
  helpTitle: "¿Qué es esto?",
  users: "clientes",
};

const dicts: Record<Locale, Dict> = { en, ru, de, es };

/* — Offline help content (no LLM, deterministic) — */

type HelpEntry = {
  id: string;
  title: Record<Locale, string>;
  body: Record<Locale, string>;
};

const helpEntries: HelpEntry[] = [
  {
    id: "vless",
    title: { en: "VLESS", ru: "VLESS", de: "VLESS", es: "VLESS" },
    body: {
      en: "VLESS is a lightweight proxy protocol: the client authenticates with a UUID and the server relays traffic without extra encryption (security comes from the TLS/REALITY layer).",
      ru: "VLESS — лёгкий прокси-протокол: клиент аутентифицируется UUID, сервер передаёт трафик без дополнительного шифрования (защиту даёт уровень TLS/REALITY).",
      de: "VLESS ist ein schlankes Proxy-Protokoll: Der Client authentifiziert sich per UUID, der Server leitet Traffic ohne Zusatzverschlüsselung weiter (Sicherheit kommt aus TLS/REALITY).",
      es: "VLESS es un protocolo proxy ligero: el cliente se autentica con un UUID y el servidor reenvía el tráfico sin cifrado extra (la seguridad proviene de TLS/REALITY).",
    },
  },
  {
    id: "reality",
    title: { en: "REALITY", ru: "REALITY", de: "REALITY", es: "REALITY" },
    body: {
      en: "REALITY lets the server borrow the TLS handshake of a real public site, so active probing sees an ordinary website. It needs an X25519 keypair, short IDs and a destination like yahoo.com:443. Works with VLESS over tcp or xhttp.",
      ru: "REALITY позволяет серверу использовать TLS-handshake настоящего сайта: активное зондирование видит обычный сайт. Нужны ключ X25519, short ID и назначение вида yahoo.com:443. Работает с VLESS поверх tcp или xhttp.",
      de: "REALITY lässt den Server den TLS-Handshake einer echten öffentlichen Seite ausleihen; aktives Scannen sieht eine gewöhnliche Website. Benötigt ein X25519-Schlüsselpaar, Short-IDs und ein Ziel wie yahoo.com:443. Funktioniert mit VLESS über tcp oder xhttp.",
      es: "REALITY permite al servidor tomar prestado el handshake TLS de un sitio real: el sondeo activo ve un sitio común. Requiere un par X25519, short IDs y un destino como yahoo.com:443. Funciona con VLESS sobre tcp o xhttp.",
    },
  },
  {
    id: "xhttp",
    title: {
      en: "XHTTP transport",
      ru: "Транспорт XHTTP",
      de: "XHTTP-Transport",
      es: "Transporte XHTTP",
    },
    body: {
      en: "XHTTP carries proxy traffic as ordinary HTTP requests (modes: packet-up, stream-up, stream-one), which makes it hard to tell apart from web traffic and works well with REALITY and CDNs.",
      ru: "XHTTP передаёт прокси-трафик обычными HTTP-запросами (режимы packet-up, stream-up, stream-one): его трудно отличить от веб-трафика, он хорошо работает с REALITY и CDN.",
      de: "XHTTP transportiert Proxy-Traffic als gewöhnliche HTTP-Anfragen (Modi: packet-up, stream-up, stream-one) — schwer vom Webtraffic unterscheidbar, gut mit REALITY und CDNs.",
      es: "XHTTP transporta el tráfico del proxy como peticiones HTTP normales (modos: packet-up, stream-up, stream-one), difícil de distinguir del tráfico web y funciona bien con REALITY y CDN.",
    },
  },
  {
    id: "quota",
    title: {
      en: "Traffic quotas",
      ru: "Лимиты трафика",
      de: "Traffic-Quoten",
      es: "Cuotas de tráfico",
    },
    body: {
      en: "Quotas are evaluated from polled statistics and can auto-suspend a client when the limit is reached. They are advisory-accurate: counters are polled, not per-packet enforced.",
      ru: "Квоты вычисляются по опрашиваемой статистике и могут автоматически приостановить клиента при исчерпании. Учёт точный, но это не жёсткое по-пакетное ограничение.",
      de: "Quoten werden aus abgefragten Statistiken ausgewertet und können einen Client bei Erreichen des Limits automatisch sperren. Die Erfassung ist genau, aber keine Paket-für-Paket-Durchsetzung.",
      es: "Las cuotas se evalúan con estadísticas consultadas y pueden suspender al cliente al alcanzar el límite. El conteo es preciso, pero no es una limitación por paquete.",
    },
  },
  {
    id: "sub",
    title: {
      en: "Subscription formats",
      ru: "Форматы подписок",
      de: "Abo-Formate",
      es: "Formatos de suscripción",
    },
    body: {
      en: "A subscription URL returns the connection list in one of the verified formats: base64 Xray links (default), plain links, Clash/Mihomo YAML (?format=clash) or JSON (?format=json). Clients poll it periodically.",
      ru: "URL подписки возвращает список подключений в одном из проверенных форматов: base64-ссылки Xray (по умолчанию), простые ссылки, YAML Clash/Mihomo (?format=clash) или JSON (?format=json). Клиенты опрашивают его периодически.",
      de: "Die Abo-URL liefert die Verbindungsliste in einem geprüften Format: Base64-Xray-Links (Standard), einfache Links, Clash/Mihomo-YAML (?format=clash) oder JSON (?format=json).",
      es: "La URL de suscripción devuelve la lista de conexiones en un formato verificado: enlaces Xray base64 (predeterminado), enlaces simples, YAML Clash/Mihomo (?format=clash) o JSON (?format=json).",
    },
  },
  {
    id: "nodes",
    title: {
      en: "Node synchronization",
      ru: "Синхронизация узлов",
      de: "Knoten-Synchronisation",
      es: "Sincronización de nodos",
    },
    body: {
      en: "Nodes enroll with a single-use token and authenticate with a credential that is shown once and stored only as a hash. Stale nodes (no heartbeat within 5 minutes) stop receiving configuration until they recover.",
      ru: "Узлы регистрируются одноразовым токеном и аутентифицируются креденшалом, который показывается один раз и хранится только в виде хэша. «Уснувшие» узлы (без отклика 5 минут) перестают получать конфигурацию до восстановления.",
      de: "Knoten registrieren sich mit einem Einmal-Token und authentifizieren sich mit einem nur einmal angezeigten, ausschließlich als Hash gespeicherten Credential. Stale-Knoten (5 Min. ohne Heartbeat) erhalten keine Konfiguration mehr, bis sie sich melden.",
      es: "Los nodos se registran con un token de un solo uso y se autentican con una credencial que se muestra una vez y se guarda solo como hash. Los nodos inactivos (sin latido por 5 min) dejan de recibir configuración hasta recuperarse.",
    },
  },
];

function Help({ id, locale }: { id: string; locale: Locale }) {
  const entry = helpEntries.find((e) => e.id === id);
  if (!entry) return null;
  return (
    <details className="xray-help">
      <summary>{entry.title[locale]}</summary>
      <p>{entry.body[locale]}</p>
    </details>
  );
}

/* — Utilities — */

function randomHex32(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

function randomUUID(): string {
  if ("randomUUID" in crypto) return crypto.randomUUID();
  const hex = randomHex32();
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

function ConfirmDialog({
  locale,
  subject,
  onConfirm,
  onCancel,
}: {
  locale: Locale;
  subject: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const d = dicts[locale];
  return (
    <div
      className="xray-confirm"
      role="dialog"
      aria-modal="true"
      aria-label={d.confirmDelete}
    >
      <p>
        {d.confirmDeleteBody} <strong>{subject}</strong>?
      </p>
      <button type="button" className="danger" onClick={onConfirm}>
        {d.delete}
      </button>{" "}
      <button type="button" onClick={onCancel}>
        {d.cancel}
      </button>
    </div>
  );
}

/* — Page — */

export function XrayCenterPage({
  page,
  bundle,
  locale,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  locale?: Locale;
  client: APIClient;
}) {
  void bundle;
  const resolvedLocale: Locale =
    locale ??
    (document.documentElement.getAttribute("data-locale") as Locale) ??
    "en";
  const d = dicts[resolvedLocale];
  const [servers, setServers] = useState<FetchResult<XrayServerView[]> | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<{
    kind: "server" | "user" | "subscription";
    id: string;
    label: string;
  } | null>(null);
  const [busy, setBusy] = useState(false);

  // Create-server form state.
  const [fName, setFName] = useState("");
  const [fProto, setFProto] = useState<string>("vless");
  const [fTrans, setFTrans] = useState<string>("tcp");
  const [fSec, setFSec] = useState<string>("tls");
  const [fListen, setFListen] = useState("127.0.0.1");
  const [fPort, setFPort] = useState("443");
  const [fMethod, setFMethod] = useState<string>(APIClient.XRAY_SS_METHODS[0]);

  // Client form state.
  const [cServer, setCServer] = useState("");
  const [cName, setCName] = useState("");
  const [cUUID, setCUUID] = useState("");

  // Subscription form state.
  const [sName, setSName] = useState("");
  const [sProfiles, setSProfiles] = useState("");

  const reload = useCallback(() => {
    client
      .fetchXrayServers()
      .then(setServers)
      .catch(() =>
        setServers({ ok: false, reason: "network", message: "network" }),
      );
  }, [client]);

  useEffect(() => {
    reload();
  }, [reload]);

  // The only combination the three selects can produce that the
  // engine refuses is REALITY outside vless+tcp/xhttp.
  const matrixError =
    fSec === "reality" && !APIClient.xrayRealityAllowed(fProto, fTrans)
      ? d.errMatrix
      : null;

  async function createServer(ev: React.FormEvent) {
    ev.preventDefault();
    setError(null);
    if (!/^[a-z][a-z0-9._-]{0,47}$/.test(fName)) return setError(d.errName);
    if (!fListen) return setError(d.errListen);
    const port = Number(fPort);
    if (!Number.isInteger(port) || port < 1 || port > 65535)
      return setError(d.errPort);
    if (matrixError) return setError(d.errMatrix);
    const payload: Record<string, unknown> = {
      id: randomHex32(),
      name: fName,
      protocol: fProto,
      transport: fTrans,
      security: fSec,
      listen: `${fListen}:${port}`,
      port,
      enabled: true,
    };
    if (fProto === "shadowsocks") payload.ssMethod = fMethod;
    setBusy(true);
    try {
      await client.xrayCreateServer(payload);
      setFName("");
      reload();
    } catch {
      setError(d.errSubmit);
    } finally {
      setBusy(false);
    }
  }

  async function createClient(ev: React.FormEvent) {
    ev.preventDefault();
    setError(null);
    if (!cServer || !cName) return;
    setBusy(true);
    try {
      await client.xrayCreateUser({
        id: randomHex32(),
        serverId: cServer,
        name: cName,
        uuid: cUUID || randomUUID(),
        enabled: true,
      });
      setCName("");
      setCUUID("");
      reload();
    } catch {
      setError(d.errSubmit);
    } finally {
      setBusy(false);
    }
  }

  async function createSubscription(ev: React.FormEvent) {
    ev.preventDefault();
    setError(null);
    const ids = sProfiles
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (!sName || ids.length === 0) return;
    setBusy(true);
    try {
      await client.xrayCreateSubscription({
        id: randomHex32(),
        name: sName,
        profileIds: ids,
        enabled: true,
      });
      setSName("");
      setSProfiles("");
      reload();
    } catch {
      setError(d.errSubmit);
    } finally {
      setBusy(false);
    }
  }

  async function runDelete() {
    if (!confirm) return;
    setBusy(true);
    setError(null);
    try {
      if (confirm.kind === "server") await client.xrayDeleteServer(confirm.id);
      if (confirm.kind === "user") await client.xrayDeleteUser(confirm.id);
      if (confirm.kind === "subscription")
        await client.xrayDeleteSubscription(confirm.id);
      setConfirm(null);
      reload();
    } catch {
      setError(d.errSubmit);
    } finally {
      setBusy(false);
    }
  }

  const serverList = servers?.ok ? servers.data : undefined;

  return (
    <PageFrame page={page} bundle={bundle}>
      {error && (
        <p className="xray-error" role="alert">
          {error}
        </p>
      )}
      {confirm && (
        <ConfirmDialog
          locale={resolvedLocale}
          subject={confirm.label}
          onConfirm={runDelete}
          onCancel={() => setConfirm(null)}
        />
      )}

      <section aria-labelledby="xray-servers-h">
        <h2 id="xray-servers-h">{d.sectionServers}</h2>
        <Help id="vless" locale={resolvedLocale} />
        <Help id="reality" locale={resolvedLocale} />
        <Help id="xhttp" locale={resolvedLocale} />
        {!servers && <StatePanel state="loading" />}
        {servers && !servers.ok && <StatePanel state="error" />}
        {servers && servers.ok && (serverList?.length ?? 0) === 0 && (
          <StatePanel state="empty" />
        )}
        {serverList && serverList.length > 0 && (
          <table className="xray-table">
            <thead>
              <tr>
                <th>{d.name}</th>
                <th>{d.protocol}</th>
                <th>{d.listen}</th>
                <th>{d.users}</th>
                <th aria-label="actions" />
              </tr>
            </thead>
            <tbody>
              {serverList.map((s) => (
                <tr key={s.ref.id}>
                  <td>{s.displayName}</td>
                  <td>{s.protocol}</td>
                  <td>
                    {s.listen}:{s.port}
                  </td>
                  <td>{s.userCount}</td>
                  <td>
                    <button
                      type="button"
                      className="danger"
                      disabled={busy}
                      onClick={() =>
                        setConfirm({
                          kind: "server",
                          id: s.ref.id,
                          label: s.displayName,
                        })
                      }
                    >
                      {d.delete}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <form className="xray-form" onSubmit={createServer}>
          <label>
            {d.name}
            <input
              value={fName}
              onChange={(e) => setFName(e.target.value)}
              autoComplete="off"
              required
            />
          </label>
          <label>
            {d.protocol}
            <select value={fProto} onChange={(e) => setFProto(e.target.value)}>
              {APIClient.XRAY_PROTOCOLS.map((p) => (
                <option key={p}>{p}</option>
              ))}
            </select>
          </label>
          <label>
            {d.transport}
            <select value={fTrans} onChange={(e) => setFTrans(e.target.value)}>
              {APIClient.XRAY_TRANSPORTS.map((t) => (
                <option key={t}>{t}</option>
              ))}
            </select>
          </label>
          <label>
            {d.security}
            <select value={fSec} onChange={(e) => setFSec(e.target.value)}>
              {APIClient.XRAY_SECURITY.map((s) => (
                <option
                  key={s}
                  disabled={
                    s === "reality" &&
                    !APIClient.xrayRealityAllowed(fProto, fTrans)
                  }
                >
                  {s}
                </option>
              ))}
            </select>
          </label>
          {fProto === "shadowsocks" && (
            <label>
              {d.ssMethod}
              <select
                value={fMethod}
                onChange={(e) => setFMethod(e.target.value)}
              >
                {APIClient.XRAY_SS_METHODS.map((m) => (
                  <option key={m}>{m}</option>
                ))}
              </select>
            </label>
          )}
          <label>
            {d.listen}
            <input
              value={fListen}
              onChange={(e) => setFListen(e.target.value)}
              required
            />
          </label>
          <label>
            {d.port}
            <input
              type="number"
              min={1}
              max={65535}
              value={fPort}
              onChange={(e) => setFPort(e.target.value)}
              required
            />
          </label>
          <button type="submit" disabled={busy || matrixError !== null}>
            {d.create}
          </button>
          {matrixError && <p className="xray-error">{matrixError}</p>}
        </form>
      </section>

      <section aria-labelledby="xray-clients-h">
        <h2 id="xray-clients-h">{d.sectionClients}</h2>
        <Help id="quota" locale={resolvedLocale} />
        <form
          className="xray-form"
          onSubmit={(ev) => {
            ev.preventDefault();
            setCServer(cServer || serverList?.[0]?.ref.id || "");
            return createClient(ev);
          }}
        >
          <label>
            {d.server}
            <select
              value={cServer}
              onChange={(e) => setCServer(e.target.value)}
              required
            >
              <option value="" disabled>
                —
              </option>
              {(serverList ?? []).map((s) => (
                <option key={s.ref.id} value={s.ref.id}>
                  {s.displayName}
                </option>
              ))}
            </select>
          </label>
          <label>
            {d.name}
            <input
              value={cName}
              onChange={(e) => setCName(e.target.value)}
              autoComplete="off"
              required
            />
          </label>
          <label>
            {d.uuid}
            <input
              value={cUUID}
              onChange={(e) => setCUUID(e.target.value)}
              placeholder={randomUUID()}
            />
          </label>
          <button type="button" onClick={() => setCUUID(randomUUID())}>
            {d.generate}
          </button>
          <button type="submit" disabled={busy || !cServer || !cName}>
            {d.create}
          </button>
        </form>
        {cServer && (
          <UserList
            client={client}
            serverId={cServer}
            locale={resolvedLocale}
            emptyLabel={d.emptyClients}
            onDelete={(id, label) => setConfirm({ kind: "user", id, label })}
            busy={busy}
            d={d}
          />
        )}
      </section>

      <section aria-labelledby="xray-subs-h">
        <h2 id="xray-subs-h">{d.sectionSubscriptions}</h2>
        <Help id="sub" locale={resolvedLocale} />
        <Help id="nodes" locale={resolvedLocale} />
        <form className="xray-form" onSubmit={createSubscription}>
          <label>
            {d.name}
            <input
              value={sName}
              onChange={(e) => setSName(e.target.value)}
              autoComplete="off"
              required
            />
          </label>
          <label>
            {d.profileIds}
            <input
              value={sProfiles}
              onChange={(e) => setSProfiles(e.target.value)}
              autoComplete="off"
              required
            />
          </label>
          <button type="submit" disabled={busy || !sName}>
            {d.create}
          </button>
        </form>
      </section>
    </PageFrame>
  );
}

function UserList({
  client,
  serverId,
  locale,
  emptyLabel,
  onDelete,
  busy,
  d,
}: {
  client: APIClient;
  serverId: string;
  locale: Locale;
  emptyLabel: string;
  onDelete: (id: string, label: string) => void;
  busy: boolean;
  d: Dict;
}) {
  void locale;
  const [result, setResult] = useState<FetchResult<XrayUserView[]> | null>(
    null,
  );
  useEffect(() => {
    let alive = true;
    client.fetchXrayUsers(serverId).then((r) => {
      if (alive) setResult(r);
    });
    return () => {
      alive = false;
    };
  }, [client, serverId]);
  if (!result) return <StatePanel state="loading" />;
  if (!result.ok) return <StatePanel state="error" />;
  const mine = result.data.filter((u) => u.serverRef?.id === serverId);
  if (mine.length === 0) return <p className="xray-empty">{emptyLabel}</p>;
  return (
    <table className="xray-table">
      <tbody>
        {mine.map((u) => (
          <tr key={u.ref.id}>
            <td>{u.displayName}</td>
            <td>
              <button
                type="button"
                className="danger"
                disabled={busy}
                onClick={() => onDelete(u.ref.id, u.displayName)}
              >
                {d.delete}
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
