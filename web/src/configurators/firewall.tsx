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
import { StatePanel } from "../components/forms/States";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import type { Overview } from "../../../shared/api/types";

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
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchOverview({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result) return <PageFrame page={page} bundle={bundle}><StatePanel state="loading" /></PageFrame>;
  if (!result.ok) return (
    <PageFrame page={page} bundle={bundle}>
      <StatePanel state={result.reason === "missing" ? "missing-contract" : "error"} body={result.message} />
    </PageFrame>
  );
  const nft = result.data.nftables;
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        title="nftables summary"
        rows={[
          { key: "tables", value: nft.tables },
          { key: "chains", value: nft.chains },
          { key: "rules", value: nft.rules },
          { key: "status", value: nft.status },
        ]}
        observedAt={result.data.observedAt}
      />
      <div className="v1-card">
        <div className="card-head">
          <h3>Rule editor columns (live workspace contract)</h3>
        </div>
        <p className="v1-mute">
          The live firewall workspace needs a typed /api/v1/firewall/rules
          endpoint that returns chains and rules with these columns. Until
          then the page renders here as a contract-gap stub.
        </p>
        <ul className="v1-row" style={{ gap: "var(--v1-s2)", flexWrap: "wrap", listStyle: "none", padding: 0 }}>
          {[
            "chain", "rule_id", "match", "action", "direction", "from", "to",
            "protocol", "src_ports", "dst_ports", "interfaces", "comment",
            "packets", "bytes",
          ].map((c) => (
            <li key={c} className="v1-pill"><strong>{c}</strong></li>
          ))}
        </ul>
      </div>
      <StatePanel
        state="missing-contract"
        title="Firewall rule CRUD not exposed"
        body="The agent exposes the nftables counter summary via /overview but does not publish typed chain/rule CRUD. The UI stays empty on purpose until that surface exists."
      />
    </PageFrame>
  );
}

export const FirewallSubareaSpecs: Record<string, { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string }> = {
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
  spec: { columns: ReadonlyArray<{ key: string; label: string; description?: string }>; endpoint: string };
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