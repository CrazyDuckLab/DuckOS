/**
 * Router — maps every route in the V1 catalog to a concrete page
 * component. Routes dispatch to:
 *   - live read-only / live configurable components (Phase C/D + reconciliation)
 *   - a typed ObjectsPage against /api/v1/objects/{kind} when the page
 *     matches one of the 19 canonical kinds
 *   - a typed contract-gap stub that renders the column list and an
 *     explicit "API contract not implemented yet" notice
 */
import { type Messages } from "../i18n/messages";
import type { APIClient } from "../api/client";
import { PAGES_BY_ID, PAGES_BY_ROUTE } from "../pages/registry";
import type { ViewObject, SupportedObjectKind } from "../api/types";
import type { DataColumn } from "../components/forms/Table";
import {
  AboutPage,
  CandidatePage,
  WifiPage as OverviewWifiPage,
  DesignOnlyPage,
} from "./OverviewPage";
import {
  TopologyPage,
} from "./Concrete";
import { SpeedLabPage } from "./SpeedLabPage";
import { ServiceDiscoveryPage } from "./ServiceDiscoveryPage";
import {
  EasyHomePage,
  EasyInternetPage,
  EasyNetworksPage,
  EasyDevicesPage,
  EasyPublishPage,
  EasyCarePage,
} from "./EasyV2";
import {
  SystemOverviewV2,
} from "./SystemOverviewV2";

import {
  InterfacesLivePage,
  InterfaceSubareaPage,
  InterfacesSubareaSpecs,
} from "../configurators/interfaces";
import {
  WanConnectionsPage,
  MultiwanPage,
  WanSubareaStub,
  WanSubareaSpecs,
} from "../configurators/wan";
import {
  NetworksLivePage,
  ClientsPage,
} from "../configurators/lan";
import {
  DnsResolverPage,
  DhcpLeasesPage,
} from "../configurators/dns";
import {
  FirewallRulesPage,
} from "../configurators/firewall";
import {
  WireguardServersPage,
  WireguardPeersPage,
  XrayServersPage,
  TunnelSubareaStub,
  TunnelSubareaSpecs,
} from "../configurators/tunnels";
import {
  RoutingPage,
  BgpPage,
  RoutingSubareaStub,
  RoutingSubareaSpecs,
} from "../configurators/routing";
import {
  ServicesLivePage,
  SubscriptionsPage,
  ServicesSubareaStub,
  ServicesSubareaSpecs,
} from "../configurators/services";
import {
  SitesLivePage,
  BackupPage,
  UpsPage,
  UsersPage,
  SystemSubareaStub,
  SystemSubareaSpecs,
} from "../configurators/system";
import {
  MonitoringPage,
  DoctorPage,
  LogsSubareaStub,
  LogsSubareaSpecs,
} from "../configurators/logs";
import { ObjectsPage } from "../configurators/Objects";

export interface PageContext {
  route: string;
  navigate: (route: string) => void;
  bundle: Messages;
  client: APIClient;
  token: string;
}

/* Per-kind column maps for the generic ObjectsPage. Each entry is
 * keyed by page id and resolved against the ViewObject envelope. */

const KIND_FOR_PAGE: Record<string, SupportedObjectKind> = {
  P008: "network.interface", // Bridges table reuses the same envelope
  P009: "network.interface",
  P010: "network.interface",
  P011: "network.interface",
  P012: "network.interface",
  P013: "network.interface",
  P014: "network.interface",
  P015: "network.interface",
  P016: "network.interface",
  P023: "network.address",
  P026: "network.address",
  P041: "network.firewall.rule",
  P042: "network.firewall.rule",
  P043: "network.firewall.rule",
  P044: "network.firewall.set",
  P045: "network.lease",
  P046: "network.lease",
  P054: "dns.zone",
  P055: "dns.zone",
  P056: "dhcp.subnet",
  P050: "xray.user",
  P047: "wireguard.server",
  P048: "wireguard.peer",
  P049: "xray.server",
  P067: "qos.class",
  P028: "route.map",
  P029: "route.map",
  P030: "bgp.peer",
  P031: "ospf.area",
  P032: "isis.area",
  P034: "route.map",
  P062: "network.address",
};

const OBJECTS_COLUMNS: Record<string, DataColumn<ViewObject>[]> = {
  "network.interface": [
    { key: "name", label: "Name", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "network.address": [
    { key: "name", label: "Address", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "network.firewall.rule": [
    { key: "name", label: "Rule", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "network.firewall.set": [
    { key: "name", label: "Set", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "network.lease": [
    { key: "name", label: "Lease", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "dns.zone": [
    { key: "name", label: "Zone", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "dhcp.subnet": [
    { key: "name", label: "Subnet", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "qos.class": [
    { key: "name", label: "Class", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "wireguard.server": [
    { key: "name", label: "Server", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "wireguard.peer": [
    { key: "name", label: "Peer", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "xray.server": [
    { key: "name", label: "Server", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "xray.user": [
    { key: "name", label: "User", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "bgp.peer": [
    { key: "name", label: "BGP peer", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "ospf.area": [
    { key: "name", label: "OSPF area", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "isis.area": [
    { key: "name", label: "IS-IS area", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
  "route.map": [
    { key: "name", label: "Route map", render: (v) => <strong>{v.displayName}</strong> },
    { key: "id", label: "Id", mono: true, render: (v) => v.ref.id },
  ],
};

export function PagesRouter({
  route,
  navigate,
  bundle,
  client,
}: PageContext) {
  const page = PAGES_BY_ROUTE.get(route) ?? PAGES_BY_ID.get("P001")!;
  const kind = KIND_FOR_PAGE[page.id];

  if (kind && OBJECTS_COLUMNS[kind]) {
    return (
      <ObjectsPage
        page={page}
        bundle={bundle}
        client={client}
        kind={kind}
        columns={OBJECTS_COLUMNS[kind]}
        canEdit={(v) => v.permissions.configure}
        canDelete={(v) => v.permissions.delete ?? v.permissions.configure}
      />
    );
  }

  switch (page.id) {
    /* — System — */
    case "P001":
      return (
        <SystemOverviewV2
          page={page}
          bundle={bundle}
          client={client}
          navigate={navigate}
        />
      );
    case "P002":
      return <CandidatePage page={page} bundle={bundle} client={client} />;
    case "P025":
      return <OverviewWifiPage page={page} bundle={bundle} client={client} />;
    case "P051":
      return <SubscriptionsPage page={page} bundle={bundle} client={client} />;
    case "P068":
      return <BackupPage page={page} bundle={bundle} client={client} />;
    case "P072":
      return <UpsPage page={page} bundle={bundle} client={client} />;
    case "P006":
      return <UsersPage page={page} bundle={bundle} client={client} />;
    case "P073":
      return <SitesLivePage page={page} bundle={bundle} client={client} />;
    case "P081":
      return <AboutPage page={page} bundle={bundle} client={client} />;
    case "P003":
    case "P004":
    case "P005":
    case "P069":
    case "P070":
    case "P071":
    case "P074":
    case "P075":
    case "P076":
    case "P077":
    case "P078":
    case "P079":
    case "P080":
      return (
        <SystemSubareaStub
          page={page}
          bundle={bundle}
          spec={SystemSubareaSpecs[page.id] ?? { columns: [], endpoint: "" }}
        />
      );

    /* — Interfaces — */
    case "P007":
      return <InterfacesLivePage page={page} bundle={bundle} client={client} />;
    case "P008":
    case "P009":
    case "P010":
    case "P011":
    case "P012":
    case "P013":
    case "P014":
    case "P015":
    case "P016":
      return (
        <InterfaceSubareaPage
          page={page}
          bundle={bundle}
          columns={InterfacesSubareaSpecs[page.id] ?? []}
        />
      );

    /* — WAN — */
    case "P017":
      return <WanConnectionsPage page={page} bundle={bundle} client={client} />;
    case "P019":
      return <MultiwanPage page={page} bundle={bundle} client={client} />;
    case "P018":
    case "P020":
    case "P021":
      return (
        <WanSubareaStub
          page={page}
          bundle={bundle}
          spec={WanSubareaSpecs[page.id] ?? { columns: [], endpoint: "" }}
        />
      );

    /* — LAN — */
    case "P022":
      return <NetworksLivePage page={page} bundle={bundle} client={client} />;
    case "P024":
      return <ClientsPage page={page} bundle={bundle} client={client} />;

    /* — Routing — */
    case "P027":
      return <RoutingPage page={page} bundle={bundle} client={client} />;
    case "P030":
      return <BgpPage page={page} bundle={bundle} client={client} />;
    case "P028":
    case "P029":
    case "P031":
    case "P032":
    case "P033":
    case "P034":
    case "P035":
    case "P036":
    case "P037":
    case "P038":
    case "P039":
    case "P040":
    case "P067":
      return (
        <RoutingSubareaStub
          page={page}
          bundle={bundle}
          spec={RoutingSubareaSpecs[page.id] ?? { columns: [], endpoint: "" }}
        />
      );

    /* — DNS / DHCP — */
    case "P053":
      return <DnsResolverPage page={page} bundle={bundle} client={client} />;
    case "P057":
      return <DhcpLeasesPage page={page} bundle={bundle} client={client} />;

    /* — Firewall — */
    case "P042":
      return <FirewallRulesPage page={page} bundle={bundle} client={client} />;

    /* — Tunnels — */
    case "P047":
      return <WireguardServersPage page={page} bundle={bundle} client={client} />;
    case "P048":
      return <WireguardPeersPage page={page} bundle={bundle} client={client} />;
    case "P049":
      return <XrayServersPage page={page} bundle={bundle} client={client} />;
    case "P050":
    case "P052":
      return (
        <TunnelSubareaStub
          page={page}
          bundle={bundle}
          spec={TunnelSubareaSpecs[page.id] ?? { columns: [], endpoint: "" }}
        />
      );

    /* — Services — */
    case "P058":
      return <ServicesLivePage page={page} bundle={bundle} client={client} />;
    case "P059":
    case "P060":
    case "P061":
    case "P062":
    case "P063":
    case "P064":
    case "P066":
      return (
        <ServicesSubareaStub
          page={page}
          bundle={bundle}
          spec={ServicesSubareaSpecs[page.id] ?? { columns: [], endpoint: "" }}
        />
      );
    case "P065":
      return (
        <ServiceDiscoveryPage
          page={page}
          bundle={bundle as unknown as Record<string, unknown>}
          client={client}
        />
      );

    /* — Logs / Diagnostics — */
    case "P087":
      return <TopologyPage page={page} bundle={bundle} client={client} />;
    case "P089":
      return <MonitoringPage page={page} bundle={bundle} client={client} />;
    case "P091":
      return <DoctorPage page={page} bundle={bundle} client={client} />;
    case "P082":
    case "P083":
    case "P084":
    case "P085":
    case "P086":
    case "P088":
    case "P092":
      return (
        <LogsSubareaStub
          page={page}
          bundle={bundle}
          spec={LogsSubareaSpecs[page.id] ?? { columns: [], endpoint: "" }}
        />
      );
    case "P090":
      return <SpeedLabPage page={page} bundle={bundle as unknown as Record<string, unknown>} client={client} />;

    /* — Easy mode (v3) — */
    case "P094":
      return (
        <EasyHomePage
          page={page}
          bundle={bundle}
          navigate={navigate}
          client={client}
        />
      );
    case "P095":
      return (
        <EasyInternetPage
          page={page}
          bundle={bundle}
          client={client}
          navigate={navigate}
        />
      );
    case "P096":
      return (
        <EasyNetworksPage
          page={page}
          bundle={bundle}
          client={client}
          navigate={navigate}
        />
      );
    case "P097":
      return (
        <EasyDevicesPage
          page={page}
          bundle={bundle}
          client={client}
          navigate={navigate}
        />
      );
    case "P098":
      return (
        <EasyPublishPage
          page={page}
          bundle={bundle}
          client={client}
          navigate={navigate}
        />
      );
    case "P099":
      return (
        <EasyCarePage
          page={page}
          bundle={bundle}
          client={client}
          navigate={navigate}
        />
      );

    /* — Everything else — design-only page shell — */
    default:
      return <DesignOnlyPage page={page} bundle={bundle} />;
  }
}
