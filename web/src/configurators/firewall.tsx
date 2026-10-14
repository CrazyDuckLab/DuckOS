/**
 * DuckOS V1 — Firewall area configurators (P041–P046).
 *
 * P042 Firewall Rules has a partial read surface via /overview.nftables
 * (table/chain/rule counts) but no typed rule CRUD today. We render a
 * typed stub that explicitly lists the columns the live view would have.
 *
 * NOTE: The user said to BUILD an actual professional firewall workspace
 * where contracts permit. The contract for /api/v1/firewall/rules does
 * not exist in the public API. We are not allowed to invent a backend.
 * The page therefore renders an honest stub that documents exactly what
 * we would need to make it live.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import {
  deleteFirewallRule,
  listFirewallObserved,
  listFirewallRules,
  putFirewallRule,
  setFirewallRuleEnabled,
  type FirewallObservedSnapshot,
  type FirewallRule,
  type FirewallRulesConfig,
} from "../api";
import type { Overview } from "../../../shared/api/types";

const EMPTY_RULE: FirewallRule = {
  id: "",
  enabled: true,
  action: "accept",
  direction: "forward",
  protocol: "any",
};

export function FirewallRulesPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<Overview> | null>(null);
  const [config, setConfig] = useState<FirewallRulesConfig | null>(null);
  const [observed, setObserved] = useState<FirewallObservedSnapshot | null>(
    null,
  );
  const [err, setErr] = useState("");
  const [status, setStatus] = useState("");
  const [draft, setDraft] = useState<FirewallRule>({ ...EMPTY_RULE });

  const refresh = async () => {
    try {
      const [cfg, obs] = await Promise.all([
        listFirewallRules(client.token(), new AbortController().signal),
        listFirewallObserved(client.token(), new AbortController().signal),
      ]);
      setConfig(cfg);
      setObserved(obs);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "load failed");
    }
  };
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setResult);
    void refresh();
    return () => ctl.abort();
  }, [client]);

  const field = (label: string, node: React.ReactNode) => (
    <div className="v4-form-row">
      <label>{label}</label>
      {node}
    </div>
  );

  async function onApply(rule: FirewallRule) {
    setErr("");
    setStatus("");
    if (!/^[a-z0-9][a-z0-9._:-]{0,63}$/.test(rule.id)) {
      setErr("Rule id must be lowercase letters, digits, dot, dash, colon or underscore.");
      return;
    }
    try {
      await putFirewallRule(client.token(), rule, new AbortController().signal);
      setStatus(`Applied rule ${rule.id} (verified on the kernel table).`);
      setDraft({ ...EMPTY_RULE });
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "apply failed");
    }
  }
  async function onDelete(id: string) {
    setErr("");
    setStatus("");
    try {
      await deleteFirewallRule(client.token(), id, new AbortController().signal);
      setStatus(`Deleted rule ${id}.`);
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "delete failed");
    }
  }
  async function onToggle(rule: FirewallRule) {
    setErr("");
    setStatus("");
    try {
      await setFirewallRuleEnabled(
        client.token(),
        rule.id,
        !rule.enabled,
        new AbortController().signal,
      );
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "update failed");
    }
  }

  const nft = result && result.ok ? result.data.nftables : null;
  const rules = config?.rules ?? [];
  return (
    <PageFrame page={page} bundle={bundle}>
      {nft && (
        <RuntimeEvidencePanel
          title="nftables summary"
          rows={[
            { key: "tables", value: nft.tables },
            { key: "chains", value: nft.chains },
            { key: "rules", value: nft.rules },
            { key: "status", value: nft.status },
          ]}
          observedAt={observed?.observedAt ?? new Date().toISOString()}
        />
      )}
      {observed && (
        <RuntimeEvidencePanel
          title="inet duckos_fw (observed)"
          rows={[
            { key: "objects", value: observed.objects.length },
            { key: "empty", value: String(observed.empty) },
            ...(observed.error
              ? [{ key: "error", value: observed.error }]
              : []),
          ]}
          observedAt={observed.observedAt}
        />
      )}

      <div className="v1-card">
        <div className="card-head">
          <h3>{"Forward rules"}</h3>
        </div>
        <p className="v1-mute">
          {"V1 manages FORWARD rules in the exclusively-owned inet duckos_fw table. Every change applies atomically with verify + rollback; the status below reflects the kernel readback, not just the API response."}
        </p>
        <table className="v1-table">
          <thead>
            <tr>
              <th>id</th>
              <th>action</th>
              <th>protocol</th>
              <th>src</th>
              <th>dst</th>
              <th>port</th>
              <th>ifaces</th>
              <th>enabled</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rules.length === 0 ? (
              <tr>
                <td colSpan={9}>No rules configured.</td>
              </tr>
            ) : (
              rules.map((r) => (
                <tr key={r.id}>
                  <td>
                    <strong>{r.id}</strong>
                  </td>
                  <td>{r.action}</td>
                  <td>{r.protocol}</td>
                  <td>{r.srcPrefix || "any"}</td>
                  <td>{r.dstPrefix || "any"}</td>
                  <td>{r.dstPort || "—"}</td>
                  <td>
                    {[r.inIface, r.outIface].filter(Boolean).join(" → ") || "—"}
                  </td>
                  <td>
                    <button
                      type="button"
                      className="v1-pill"
                      data-state={r.enabled ? "good" : "warn"}
                      onClick={() => void onToggle(r)}
                    >
                      {r.enabled ? "enabled" : "disabled"}
                    </button>
                  </td>
                  <td>
                    <button
                      type="button"
                      onClick={() => void onDelete(r.id)}
                    >
                      delete
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="v1-card">
        <div className="card-head">
          <h3>{draft.id ? `Edit rule ${draft.id}` : "Add rule"}</h3>
        </div>
        {field("Rule id", (
          <input
            value={draft.id}
            onChange={(e) => setDraft({ ...draft, id: e.target.value })}
            placeholder="allow-lan-web"
          />
        ))}
        {field("Action", (
          <select
            value={draft.action}
            onChange={(e) =>
              setDraft({ ...draft, action: e.target.value as FirewallRule["action"] })
            }
          >
            <option value="accept">accept</option>
            <option value="drop">drop</option>
            <option value="reject">reject</option>
          </select>
        ))}
        {field("Protocol", (
          <select
            value={draft.protocol}
            onChange={(e) =>
              setDraft({
                ...draft,
                protocol: e.target.value as FirewallRule["protocol"],
                dstPort: e.target.value === "any" ? undefined : draft.dstPort,
              })
            }
          >
            <option value="any">any</option>
            <option value="tcp">tcp</option>
            <option value="udp">udp</option>
          </select>
        ))}
        {field("Source prefix (CIDR)", (
          <input
            value={draft.srcPrefix ?? ""}
            onChange={(e) => setDraft({ ...draft, srcPrefix: e.target.value })}
            placeholder="192.168.10.0/24"
          />
        ))}
        {field("Destination prefix (CIDR)", (
          <input
            value={draft.dstPrefix ?? ""}
            onChange={(e) => setDraft({ ...draft, dstPrefix: e.target.value })}
          />
        ))}
        {draft.protocol !== "any" &&
          field("Destination port", (
            <input
              type="number"
              min={1}
              max={65535}
              value={draft.dstPort ?? ""}
              onChange={(e) =>
                setDraft({
                  ...draft,
                  dstPort: e.target.value ? Number(e.target.value) : undefined,
                })
              }
            />
          ))}
        {field("In interface", (
          <input
            value={draft.inIface ?? ""}
            onChange={(e) => setDraft({ ...draft, inIface: e.target.value })}
            placeholder="br-lan"
          />
        ))}
        {field("Out interface", (
          <input
            value={draft.outIface ?? ""}
            onChange={(e) => setDraft({ ...draft, outIface: e.target.value })}
            placeholder="eth-wan"
          />
        ))}
        {field("Comment", (
          <input
            value={draft.comment ?? ""}
            onChange={(e) => setDraft({ ...draft, comment: e.target.value })}
          />
        ))}
        <div className="v1-row">
          <button
            type="button"
            className="v1-button"
            onClick={() => void onApply(draft)}
          >
            {draft.id ? "Apply rule" : "Add rule"}
          </button>
          {draft.id && (
            <button
              type="button"
              onClick={() =>
                void onDelete(draft.id).then(() => setDraft({ ...EMPTY_RULE }))
              }
            >
              Delete draft
            </button>
          )}
        </div>
        {status && (
          <p className="v1-pill" data-state="good" role="status">
            {status}
          </p>
        )}
      </div>

      {err && (
        <div role="alert" className="v1-error" data-state="error">
          {err}
        </div>
      )}
    </PageFrame>
  );
}

export const FirewallSubareaSpecs: Record<
  string,
  {
    columns: ReadonlyArray<{
      key: string;
      label: string;
      description?: string;
    }>;
    endpoint: string;
  }
> = {
  P041: {
    endpoint: "/api/v1/firewall/policies",
    columns: [
      { key: "from", label: "From" },
      { key: "to", label: "To" },
      { key: "service", label: "Service" },
      { key: "action", label: "Action" },
    ],
  },
  P043: {
    endpoint: "/api/v1/firewall/nat",
    columns: [
      { key: "external", label: "External" },
      { key: "internal", label: "Internal" },
      { key: "service", label: "Service" },
    ],
  },
  P044: {
    endpoint: "/api/v1/firewall/objects",
    columns: [
      { key: "name", label: "Name" },
      { key: "type", label: "Type" },
      { key: "source", label: "Source" },
    ],
  },
  P045: {
    endpoint: "/api/v1/conntrack",
    columns: [
      { key: "src", label: "Source" },
      { key: "dst", label: "Destination" },
      { key: "proto", label: "Protocol" },
      { key: "age", label: "Age" },
    ],
  },
  P046: {
    endpoint: "/api/v1/firewall/analyze",
    columns: [
      { key: "finding", label: "Finding" },
      { key: "severity", label: "Severity" },
      { key: "remediation", label: "Remediation" },
    ],
  },
};

export function FirewallSubareaStub({
  page,
  bundle,
  spec,
}: {
  page: PageMeta;
  bundle: Messages;
  spec: {
    columns: ReadonlyArray<{
      key: string;
      label: string;
      description?: string;
    }>;
    endpoint: string;
  };
}) {
  return (
    <UnsupportedFeature
      page={page}
      bundle={bundle}
      contract={spec.endpoint}
      blocker="This firewall area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}
