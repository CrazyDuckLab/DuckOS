/**
 * DuckOS V1 — Tunnels area configurators (P047–P052).
 *
 * WireGuard, Xray, NetBird: the public API is the typed
 * /api/v1/wireguard and /api/v1/xray surface (proxied to the
 * Agent). The dashboard renders the typed ServerView / PeerView
 * shapes. Private keys and preshared keys are NEVER round-tripped
 * through the dashboard — they live only in the VaultExport
 * contract.
 */
import { useEffect, useState } from "react";
import { APIClient, type FetchResult } from "../api/client";
import type {
  WireguardServerView,
  WireguardPeerView,
  XrayServerView,
} from "../api/types";
import { type Messages } from "../i18n/messages";
import { type PageMeta } from "../pages/registry";
import { PageFrame } from "../components/PageFrame";
import { StatePanel } from "../components/forms/States";
import { DataTable, FilterBar } from "../components/forms/Table";
import { RuntimeEvidencePanel } from "../components/forms/Panels";
import { UnsupportedFeature } from "../components/UnsupportedFeature";
import {
  wireguardServerToInspector,
  wireguardPeerToInspector,
  xrayServerToInspector,
} from "../components/inspector/selectors";
import { usePrefs } from "../state/store";

export function WireguardServersPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<
    WireguardServerView[]
  > | null>(null);
  const { state, dispatch } = usePrefs();
  const selectedKey =
    state.inspectorObject && state.inspectorObject.pageId === page.id
      ? state.inspectorObject.ref
      : null;
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchWireguardServers({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  if (!result.ok)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );
  const servers = result.data;
  const totalPeers = servers.reduce((acc, s) => acc + s.peerCount, 0);
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "servers", value: servers.length },
          { key: "total peers", value: totalPeers },
          {
            key: "with private key",
            value: servers.filter((s) => s.hasPrivateKey).length,
          },
        ]}
      />
      <DataTable<WireguardServerView>
        rows={servers}
        rowKey={(s) => s.ref.id}
        pageSize={20}
        emptyState="No WireGuard servers observed."
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: wireguardServerToInspector(page.id, row),
          });
        }}
        columns={[
          {
            key: "name",
            label: "Server",
            render: (s) => <strong>{s.displayName}</strong>,
          },
          {
            key: "iface",
            label: "Interface",
            mono: true,
            render: (s) => s.interface,
          },
          {
            key: "port",
            label: "Listen port",
            mono: true,
            numeric: true,
            render: (s) => s.listenPort,
          },
          {
            key: "addresses",
            label: "Addresses",
            mono: true,
            render: (s) => s.addresses.join(", ") || "—",
          },
          {
            key: "peerCount",
            label: "Peers",
            mono: true,
            numeric: true,
            render: (s) => s.peerCount,
          },
          {
            key: "publicKeyDigest",
            label: "Public key digest",
            mono: true,
            render: (s) => s.publicKeyDigest.slice(0, 16) + "…",
          },
          {
            key: "hasPrivateKey",
            label: "Private key",
            render: (s) => (
              <span
                className="v1-pill"
                data-state={s.hasPrivateKey ? "good" : "warn"}
              >
                {s.hasPrivateKey ? "present (VaultExport)" : "absent"}
              </span>
            ),
          },
        ]}
      />
      <p className="v1-subtle v1-fs-xs">
        Public keys are shown as a sha256 digest only — never the raw bytes.
        Private keys are reachable via the dedicated VaultExport contract
        (one-time export, never stored).
      </p>
    </PageFrame>
  );
}

export function WireguardPeersPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<WireguardPeerView[]> | null>(
    null,
  );
  const [query, setQuery] = useState("");
  const { state, dispatch } = usePrefs();
  const selectedKey =
    state.inspectorObject && state.inspectorObject.pageId === page.id
      ? state.inspectorObject.ref
      : null;
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchWireguardPeers({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  if (!result.ok)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );

  const peers = result.data;
  const filtered = peers.filter((p) => {
    if (!query) return true;
    return `${p.displayName} ${p.publicKeyDigest}`
      .toLowerCase()
      .includes(query.toLowerCase());
  });
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "peers", value: peers.length },
          { key: "enabled", value: peers.filter((p) => p.enabled).length },
          {
            key: "with preshared key",
            value: peers.filter((p) => p.hasPresharedKey).length,
          },
        ]}
      />
      <FilterBar
        value={query}
        onChange={setQuery}
        placeholder="Filter peers by name or key digest"
        right={
          <span className="v1-subtle v1-fs-xs">
            {filtered.length} of {peers.length}
          </span>
        }
      />
      <DataTable<WireguardPeerView>
        rows={filtered}
        rowKey={(p) => p.ref.id}
        pageSize={25}
        emptyState="No peers match this view."
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: wireguardPeerToInspector(page.id, row),
          });
        }}
        columns={[
          {
            key: "name",
            label: "Peer",
            render: (p) => <strong>{p.displayName}</strong>,
          },
          {
            key: "server",
            label: "Server",
            mono: true,
            render: (p) => p.serverRef.id,
          },
          {
            key: "allowedIPs",
            label: "AllowedIPs",
            mono: true,
            render: (p) => p.allowedIPs.join(", ") || "—",
          },
          {
            key: "endpoint",
            label: "Endpoint",
            mono: true,
            render: (p) => p.endpoint ?? "—",
          },
          {
            key: "enabled",
            label: "Enabled",
            render: (p) => (
              <span
                className="v1-pill"
                data-state={p.enabled ? "good" : "warn"}
              >
                {p.enabled ? "yes" : "no"}
              </span>
            ),
          },
          {
            key: "keepalive",
            label: "Keepalive",
            mono: true,
            numeric: true,
            render: (p) => p.persistentKeepalive ?? "—",
          },
          {
            key: "digest",
            label: "Public key digest",
            mono: true,
            render: (p) => p.publicKeyDigest.slice(0, 16) + "…",
          },
        ]}
      />
      <p className="v1-subtle v1-fs-xs">
        Peer config export requires the VaultExport contract. Private keys are
        never round-tripped through the dashboard.
      </p>
    </PageFrame>
  );
}

export function XrayServersPage({
  page,
  bundle,
  client,
}: {
  page: PageMeta;
  bundle: Messages;
  client: APIClient;
}) {
  const [result, setResult] = useState<FetchResult<XrayServerView[]> | null>(
    null,
  );
  const { state, dispatch } = usePrefs();
  const selectedKey =
    state.inspectorObject && state.inspectorObject.pageId === page.id
      ? state.inspectorObject.ref
      : null;
  useEffect(() => {
    const ctl = new AbortController();
    client.fetchXrayServers({ signal: ctl.signal }).then(setResult);
    return () => ctl.abort();
  }, [client]);
  if (!result)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel state="loading" />
      </PageFrame>
    );
  if (!result.ok)
    return (
      <PageFrame page={page} bundle={bundle}>
        <StatePanel
          state={result.reason === "missing" ? "missing-contract" : "error"}
          body={result.message}
        />
      </PageFrame>
    );
  return (
    <PageFrame page={page} bundle={bundle}>
      <RuntimeEvidencePanel
        rows={[
          { key: "servers", value: result.data.length },
          {
            key: "total users",
            value: result.data.reduce((acc, s) => acc + s.userCount, 0),
          },
        ]}
      />
      <DataTable<XrayServerView>
        rows={result.data}
        rowKey={(s) => s.ref.id}
        pageSize={20}
        emptyState="No Xray servers observed."
        selectedRowKey={selectedKey}
        onRowClick={(row) => {
          dispatch({
            type: "selectObject",
            object: xrayServerToInspector(page.id, row),
          });
        }}
        columns={[
          {
            key: "name",
            label: "Server",
            render: (s) => <strong>{s.displayName}</strong>,
          },
          { key: "protocol", label: "Protocol" },
          { key: "listen", label: "Listen", mono: true },
          { key: "port", label: "Port", mono: true, numeric: true },
          { key: "userCount", label: "Users", mono: true, numeric: true },
        ]}
      />
    </PageFrame>
  );
}

export const TunnelSubareaSpecs: Record<
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
  P050: {
    endpoint: "/api/v1/xray/users",
    columns: [
      { key: "user", label: "User" },
      { key: "quota", label: "Quota" },
      { key: "used", label: "Used" },
    ],
  },
  P052: {
    endpoint: "/api/v1/netbird",
    columns: [
      { key: "tunnel", label: "Tunnel" },
      { key: "status", label: "Status" },
      { key: "peer", label: "Peer" },
    ],
  },
};

export function TunnelSubareaStub({
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
      blocker="This tunnels area is registered in the V1 catalog but the typed API it needs is not exposed yet. The UI stays empty on purpose — no fake data, no fake success. Once the contract lands, the table renders the columns below."
    />
  );
}
