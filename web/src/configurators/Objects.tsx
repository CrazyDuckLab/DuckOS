/**
 * DuckOS V1 — typed objects collection page (V4 chrome).
 *
 * Renders every object kind registered in `KIND_FOR_PAGE` against
 * /api/v1/objects/{kind}. The chrome is V4:
 *  - page header from PageFrame
 *  - small product summary row (Total / Active / Issues)
 *  - localized filter and availability chips
 *  - per-domain EmptyState when the collection is empty
 *  - technical details disclosure holds the envelope snapshot id
 * Safe Apply lifecycle is unchanged.
 */
import { useCallback, useContext, useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import type { Collection, SupportedObjectKind, ViewObject } from "../api/types";
import { detectLocale, type Messages } from "../i18n/messages";
import { type Locale } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable, FilterBar, type DataColumn } from "../components/forms/Table";
import { ConfiguratorShell } from "./ConfiguratorShell";
import { bindingForKind, type KindBinding } from "./intentBuilders";
import { useSafeApply } from "../state/safeApply";
import { v4t } from "../i18n/v4-strings";
import { EmptyState } from "../pages/EmptyState";
import { PrefsContext } from "../state/store";

interface ObjectsPageProps {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
  kind: SupportedObjectKind;
  columns: DataColumn<ViewObject>[];
  canDelete?: (v: ViewObject) => boolean;
  canEdit?: (v: ViewObject) => boolean;
  canCreate?: boolean;
  /** Override binding lookup with a kind-specific one (e.g. VLANs). */
  overrideBinding?: KindBinding;
  /** Per-page override of the kind-specific initial edit values. */
  toInitial?: (v: ViewObject) => unknown;
}

/** Map every SupportedObjectKind to the empty-state namespace we want
 * to show when its collection is empty. Defaults to `generic`. */
function emptyKeyForKind(kind: SupportedObjectKind): string {
  switch (kind) {
    case "network.interface":
      return "empty.interfaces";
    case "network.address":
      return "empty.interfaces";
    case "network.firewall.rule":
    case "network.firewall.set":
      return "empty.firewallRules";
    case "network.lease":
      return "empty.dhcpLeases";
    case "wireguard.server":
    case "wireguard.peer":
      return "empty.wireguardPeers";
    case "xray.server":
    case "xray.user":
      return "empty.xray";
    case "dns.zone":
      return "empty.dns";
    case "dhcp.subnet":
      return "empty.dhcpLeases";
    case "bgp.peer":
    case "ospf.area":
    case "isis.area":
    case "route.map":
    case "qos.class":
      return "empty.routes";
    default:
      return "empty.generic";
  }
}

/** Short product label for each kind. Used as the page-name suffix
 * inside the chrome, e.g. "WireGuard peers · 0 total". */
function kindLabel(kind: SupportedObjectKind, locale: Locale): string {
  const labels: Partial<Record<SupportedObjectKind, { en: string; ru: string; de: string; es: string }>> = {
    "network.interface": { en: "interfaces", ru: "интерфейсы", de: "Schnittstellen", es: "interfaces" },
    "network.address": { en: "addresses", ru: "адреса", de: "Adressen", es: "direcciones" },
    "network.firewall.rule": { en: "rules", ru: "правила", de: "Regeln", es: "reglas" },
    "network.firewall.set": { en: "sets", ru: "наборы", de: "Sets", es: "conjuntos" },
    "network.lease": { en: "leases", ru: "lease-ы", de: "Leases", es: "concesiones" },
    "wireguard.server": { en: "servers", ru: "серверы", de: "Server", es: "servidores" },
    "wireguard.peer": { en: "peers", ru: "peer-ы", de: "Peers", es: "peers" },
    "xray.server": { en: "servers", ru: "серверы", de: "Server", es: "servidores" },
    "xray.user": { en: "users", ru: "пользователи", de: "Benutzer", es: "usuarios" },
    "dns.zone": { en: "zones", ru: "зоны", de: "Zonen", es: "zonas" },
    "dhcp.subnet": { en: "subnets", ru: "подсети", de: "Subnetze", es: "subredes" },
    "bgp.peer": { en: "peers", ru: "peer-ы", de: "Peers", es: "peers" },
    "ospf.area": { en: "areas", ru: "области", de: "Areas", es: "áreas" },
    "isis.area": { en: "areas", ru: "области", de: "Areas", es: "áreas" },
    "route.map": { en: "maps", ru: "map-ы", de: "Maps", es: "mapas" },
    "qos.class": { en: "classes", ru: "классы", de: "Klassen", es: "clases" },
  };
  return labels[kind]?.[locale] ?? kind;
}

export function ObjectsPage({
  page,
  bundle,
  client,
  kind,
  columns,
  canDelete,
  canEdit,
  canCreate,
  overrideBinding,
  toInitial,
}: ObjectsPageProps) {
  const [result, setResult] = useState<FetchResult<Collection> | null>(null);
  const [query, setQuery] = useState("");
  const [availability, setAvailability] = useState<"all" | "available" | "non-available">("all");
  const [showTech, setShowTech] = useState(false);
  const [activeAction, setActiveAction] = useState<{
    mode: "create" | "edit" | "delete";
    target?: ViewObject;
    value?: unknown;
  } | null>(null);

  const binding = overrideBinding ?? bindingForKind(kind);

  const locale = useLocale();

  const reload = useCallback(async () => {
    const r = await client.fetchObjects(kind, { limit: 200 });
    setResult(r);
  }, [client, kind]);

  useEffect(() => {
    const ctl = new AbortController();
    client.fetchObjects(kind, { limit: 200 }, { signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client, kind]);

  if (!result) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" title={v4t(locale, "sys.dataStale")} />
      </PageFrame>
    );
  }
  if (!result.ok) {
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );
  }

  const col = result.data;
  const items = col.items;
  const q = query.toLowerCase();
  const filtered = items.filter((v) => {
    if (availability === "available" && v.availability !== "available") return false;
    if (availability === "non-available" && v.availability === "available") return false;
    if (!q) return true;
    const haystack = `${v.displayName} ${v.ref.id} ${v.tags?.join(" ") ?? ""}`.toLowerCase();
    return haystack.includes(q);
  });

  const counts = {
    total: items.length,
    active: items.filter((v) => v.availability === "available").length,
    issues: col.issues?.length ?? 0,
  };

  const showCreate = (canCreate ?? true) && binding !== undefined;
  const showEditCol = binding !== undefined;
  const showDeleteCol = binding !== undefined;

  const effectiveCanEdit = canEdit ?? ((v: ViewObject) => v.permissions.configure === true);
  const effectiveCanDelete =
    canDelete ?? ((v: ViewObject) => (v.permissions.delete ?? v.permissions.configure) === true);

  const emptyKey = emptyKeyForKind(kind);
  const emptyTitle = v4t(locale, `${emptyKey}.title`);
  const emptyBody = v4t(locale, `${emptyKey}.body`);
  const emptyAction = v4t(locale, `${emptyKey}.action`);

  const summary: Array<{ label: string; value: React.ReactNode; tone?: "muted" | "warn" | "bad" }> = [
    { label: v4t(locale, "chrome.total"), value: counts.total },
    { label: v4t(locale, "chrome.active"), value: counts.active },
  ];
  if (counts.issues > 0) {
    summary.push({
      label: v4t(locale, "chrome.issues"),
      value: counts.issues,
      tone: counts.issues > 0 ? "warn" : undefined,
    });
  }

  return (
    <PageFrame page={page} bundle={bundle}>
      <div className="v4-page-section">
        <div className="v4-summary-row" data-count={summary.length}>
          {summary.map((s) => (
            <div className="v4-summary" key={s.label}>
              <span className="v4-summary-label">{s.label}</span>
              <span className="v4-summary-value" data-tone={s.tone}>{s.value}</span>
            </div>
          ))}
        </div>

        {(col.issues ?? []).length > 0 && (
          <StatePanel
            state="degraded"
            title={v4t(locale, "chrome.issues")}
            body={v4t(locale, "chrome.emptyKind")}
          />
        )}

        <FilterBar
          value={query}
          onChange={setQuery}
          placeholder={`${v4t(locale, "chrome.filter")} · ${kindLabel(kind, locale)}`}
          right={
            <>
              <div className="v4-chip-group" role="group" aria-label={v4t(locale, "chrome.availability")}>
                <button
                  type="button"
                  className="v4-chip"
                  data-active={availability === "all"}
                  aria-pressed={availability === "all"}
                  onClick={() => setAvailability("all")}
                >
                  {v4t(locale, "chrome.allAvailability")}
                </button>
                <button
                  type="button"
                  className="v4-chip"
                  data-active={availability === "available"}
                  aria-pressed={availability === "available"}
                  onClick={() => setAvailability("available")}
                >
                  {v4t(locale, "chrome.availableOnly")}
                </button>
                <button
                  type="button"
                  className="v4-chip"
                  data-active={availability === "non-available"}
                  aria-pressed={availability === "non-available"}
                  onClick={() => setAvailability("non-available")}
                >
                  {v4t(locale, "chrome.nonAvailable")}
                </button>
              </div>
              <span className="v4-subtle v4-fs-xs">
                {v4t(locale, "chrome.ofTotal", filtered.length, items.length)}
              </span>
              {showCreate && (
                <button
                  type="button"
                  className="v4-btn-primary"
                  onClick={() => setActiveAction({ mode: "create" })}
                  aria-label={v4t(locale, "chrome.create")}
                >
                  {v4t(locale, "chrome.create")}
                </button>
              )}
            </>
          }
        />

        {filtered.length === 0 ? (
          <EmptyState
            icon="collection"
            title={emptyTitle}
            body={emptyBody}
            action={showCreate ? emptyAction : undefined}
            onAction={showCreate ? () => setActiveAction({ mode: "create" }) : undefined}
          />
        ) : (
          <DataTable<ViewObject>
            rows={filtered}
            rowKey={(v) => `${v.ref.kind}:${v.ref.id}`}
            pageSize={25}
            initialSort={{ key: "name", direction: "asc" }}
            columns={[
              ...columns,
              {
                key: "availability",
                label: v4t(locale, "chrome.availability"),
                render: (v) => (
                  <span className="v4-status-pill" data-state={availabilityToState(v.availability)}>
                    {v.availability}
                  </span>
                ),
              },
              {
                key: "actions",
                label: v4t(locale, "chrome.actions"),
                sortable: false,
                render: (v) => (
                  <span className="v4-row-actions">
                    {showEditCol && effectiveCanEdit(v) && binding && (
                      <button
                        type="button"
                        className="v4-btn-ghost"
                        onClick={() =>
                          setActiveAction({ mode: "edit", target: v, value: toInitial?.(v) ?? v.desired })
                        }
                      >
                        {v4t(locale, "chrome.edit")}
                      </button>
                    )}
                    {showDeleteCol && effectiveCanDelete(v) && binding && (
                      <button
                        type="button"
                        className="v4-btn-danger-ghost"
                        onClick={() => setActiveAction({ mode: "delete", target: v })}
                      >
                        {v4t(locale, "chrome.delete")}
                      </button>
                    )}
                  </span>
                ),
              },
            ]}
          />
        )}

        <details
          className="v4-tech-details"
          open={showTech}
          onToggle={(e) => setShowTech((e.target as HTMLDetailsElement).open)}
        >
          <summary>{v4t(locale, "chrome.technicalDetails")}</summary>
          <div className="v4-tech-body">
            <div className="v4-tech-row">
              <span className="v4-tech-key">{v4t(locale, "chrome.collectionSnapshot")}</span>
              <span className="v4-mono">{col.snapshotId || "—"}</span>
            </div>
            <div className="v4-tech-row">
              <span className="v4-tech-key">{v4t(locale, "inspector.kind")}</span>
              <span className="v4-mono">{kind}</span>
            </div>
          </div>
        </details>

        {activeAction && binding && (
          <ActionModal
            binding={binding}
            bundle={bundle}
            client={client}
            action={activeAction}
            onClose={() => {
              setActiveAction(null);
              void reload();
            }}
          />
        )}
      </div>
    </PageFrame>
  );
}

function ActionModal({
  binding,
  bundle,
  client,
  action,
  onClose,
}: {
  binding: KindBinding;
  bundle: Messages;
  client: APIClient;
  action: { mode: "create" | "edit" | "delete"; target?: ViewObject; value?: unknown };
  onClose: () => void;
}) {
  const safeApply = useSafeApply(client);
  const locale = useLocale();
  const [formValue, setFormValue] = useState<unknown>(
    action.mode === "create"
      ? undefined
      : action.value ?? action.target?.desired ?? {},
  );

  const title =
    action.mode === "create"
      ? binding.createTitle(bundle)
      : action.mode === "edit"
        ? `${binding.editTitle(bundle)} — ${action.target?.displayName ?? action.target?.ref.id ?? ""}`
        : `${binding.deleteTitle(bundle)} — ${action.target?.displayName ?? action.target?.ref.id ?? ""}`;

  const canApply =
    action.mode === "delete"
      ? (action.target?.permissions.delete ?? action.target?.permissions.configure) === true
      : formValue !== undefined && binding.validate(formValue);

  const buildIntent = useCallback(() => {
    if (action.mode === "create") {
      return binding.intentForCreate(formValue);
    }
    if (action.mode === "edit" && action.target) {
      return binding.intentForUpdate(action.target.ref.id, formValue, action.target.ref.revision);
    }
    if (action.mode === "delete" && action.target) {
      return binding.intentForDelete(action.target.ref.id, action.target.ref.revision);
    }
    return { objects: [] };
  }, [action, binding, formValue]);

  const form =
    action.mode === "delete" ? (
      <StatePanel
        state="degraded"
        title={`${v4t(locale, "chrome.delete")} ${action.target?.displayName ?? action.target?.ref.id ?? ""}`}
        body="The dashboard will submit a Safe Apply transaction whose desired state removes this object. The change is reversible within the commit-confirmed window."
      />
    ) : action.mode === "edit" ? (
      binding.EditForm({ initial: formValue, onChange: setFormValue })
    ) : (
      binding.CreateForm({ onChange: setFormValue })
    );

  return (
    <ConfiguratorShell
      api={safeApply}
      bundle={bundle}
      client={client}
      buildIntent={buildIntent}
      canApply={canApply}
      actionTitle={title}
      actionBody={binding.help}
      form={form}
      onClose={onClose}
    />
  );
}

function availabilityToState(a: string): "good" | "warn" | "bad" | "unknown" {
  switch (a) {
    case "available":
      return "good";
    case "experimental":
      return "warn";
    case "hardware-pending":
    case "credential-pending":
      return "warn";
    case "unsupported":
      return "bad";
    default:
      return "unknown";
  }
}

function useLocale(): Locale {
  // Pull from the prefs context so the page follows the topbar's locale.
  // Falls back to the data-locale attribute and to detectLocale().
  try {
    const { state } = useContext(PrefsContext) ?? {};
    if (state?.locale) return state.locale;
  } catch {
    /* outside a Provider — fall through */
  }
  if (typeof document !== "undefined") {
    const l = document.documentElement.getAttribute("data-locale") as Locale | null;
    if (l) return l;
  }
  return detectLocale();
}
