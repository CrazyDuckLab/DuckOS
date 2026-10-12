import { useState } from "react";
import type { Overview } from "../../shared/api/types";
import { DuckMark, Icon } from "./icons";
import { formatTime } from "./format";
import "./Topology.css";

function identity(link: Overview["interfaces"][number]) {
  return `${link.index}:${link.name}:${link.mac}`;
}

export interface TopologySelection {
  kind: "interface";
  ref: string;
  displayName: string;
  status: string;
  fields: ReadonlyArray<{ label: string; value: string; mono?: boolean }>;
}

export function Topology({
  data,
  stale,
  onSelectObject,
}: {
  data: Overview;
  stale: boolean;
  onSelectObject?: (sel: TopologySelection) => void;
}) {
  const [selected, setSelected] = useState("");
  const [full, setFull] = useState(false);
  const [filter, setFilter] = useState("");
  const links = [...data.interfaces].sort((a, b) => a.index - b.index);
  const shown = links.filter((link) =>
    `${link.name} ${link.mac} ${link.addresses.map((a) => a.address).join(" ")}`
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  const current = links.find((link) => identity(link) === selected);
  const routes = current
    ? data.routes.filter((route) => route.device === current.name)
    : [];
  const defaults = routes.filter((route) =>
    ["default", "0.0.0.0/0", "::/0"].includes(route.destination),
  );
  const incomplete = data.issues.some((issue) =>
    ["interfaces", "routes"].includes(issue.component),
  );

  return (
    <section
      id="topology"
      className="content-section topology"
      aria-labelledby="topology-title"
    >
      <header className="topology__heading">
        <div>
          <p className="eyebrow">UNDERSTAND YOUR NETWORK</p>
          <h2 id="topology-title">Topology</h2>
          <p>
            Connections reported by Linux. Select an interface to inspect its
            addresses and routes.
          </p>
        </div>
        <div
          className="topology__modes"
          role="group"
          aria-label="Topology detail level"
        >
          <button
            type="button"
            aria-pressed={!full}
            onClick={() => setFull(false)}
          >
            Easy
          </button>
          <button
            type="button"
            aria-pressed={full}
            onClick={() => setFull(true)}
          >
            Full
          </button>
        </div>
      </header>
      <div className="glass topology__surface">
        <div className="topology__provenance">
          <span>
            {stale ? "Stale snapshot" : "Observed snapshot"}
            {incomplete ? " · incomplete" : ""}
          </span>
          <time dateTime={data.observedAt}>{formatTime(data.observedAt)}</time>
        </div>
        <div className="topology__layout">
          <div
            className="topology__map"
            aria-label="Observed host and interface relationships"
          >
            <div className="topology__host">
              <DuckMark />
              <strong>{data.system.hostname || "DuckOS host"}</strong>
              <span>Local Linux host</span>
            </div>
            <div className="topology__branches">
              <label className="topology__filter">
                <span>Find an interface</span>
                <input
                  type="search"
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                  placeholder="Name, address or MAC"
                />
              </label>
              <p className="topology__count">
                {shown.length} of {links.length} observed interfaces
              </p>
              <ul className="topology__nodes">
                {shown.map((link) => (
                  <li key={identity(link)}>
                    <button
                      type="button"
                      className="topology__node"
                      aria-pressed={identity(link) === selected}
                      aria-label={`Inspect interface ${link.name}`}
                      onClick={() => {
                        const id = identity(link);
                        setSelected(id);
                        if (onSelectObject) {
                          onSelectObject({
                            kind: "interface",
                            ref: link.mac || link.name,
                            displayName: link.name,
                            status: link.state || "unknown",
                            fields: [
                              { label: "Name", value: link.name },
                              {
                                label: "Index",
                                value: String(link.index),
                                mono: true,
                              },
                              {
                                label: "MAC",
                                value: link.mac || "—",
                                mono: true,
                              },
                              {
                                label: "MTU",
                                value: String(link.mtu),
                                mono: true,
                              },
                              {
                                label: "State",
                                value: link.state || "unknown",
                              },
                              {
                                label: "Addresses",
                                value: link.addresses.length
                                  ? link.addresses
                                      .map((a) => a.address)
                                      .join(", ")
                                  : "—",
                                mono: true,
                              },
                            ],
                          });
                        }
                      }}
                    >
                      <Icon name="network" />
                      <span>
                        <strong>{link.name}</strong>
                        <small>
                          {link.addresses.length} assigned{" "}
                          {link.addresses.length === 1
                            ? "address"
                            : "addresses"}
                        </small>
                      </span>
                      <span className="topology__link-state">
                        {link.state || "Unknown state"}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
              {!shown.length && (
                <p className="topology__empty">
                  {links.length
                    ? "No interfaces match this filter."
                    : "No interfaces were reported. This does not prove that the host has no network links."}
                </p>
              )}
            </div>
          </div>
          <aside
            className="topology__details"
            aria-label="Selected topology object"
          >
            {current ? (
              <>
                <p className="eyebrow">OBSERVED INTERFACE</p>
                <h3>{current.name}</h3>
                <dl>
                  <div>
                    <dt>Reported state</dt>
                    <dd>{current.state || "Unavailable"}</dd>
                  </div>
                  <div>
                    <dt>Assigned addresses</dt>
                    <dd>
                      {current.addresses.length
                        ? current.addresses.map((address) => (
                            <code key={`${address.family}:${address.address}`}>
                              {address.address}/{address.prefixLength}
                            </code>
                          ))
                        : "None reported"}
                    </dd>
                  </div>
                  <div>
                    <dt>Related routes</dt>
                    <dd>{routes.length} in this snapshot</dd>
                  </div>
                </dl>
                <h4>Default-route connections</h4>
                {defaults.length ? (
                  <ul className="topology__paths">
                    {defaults.map((route, index) => (
                      <li
                        key={`${route.family}:${route.table}:${route.gateway}:${index}`}
                      >
                        <span>{current.name}</span>
                        <span aria-hidden="true">→</span>
                        <span>
                          {route.gateway || "Direct route"}
                          <small>
                            {route.family.toUpperCase()} · table{" "}
                            {route.table || "unspecified"}
                          </small>
                        </span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p>
                    No default route was reported for this interface. More
                    specific routes may still provide access.
                  </p>
                )}
                <p className="topology__caution">
                  A route or an UP state does not prove Internet reachability,
                  firewall permission or tunnel health.
                </p>
                {full && (
                  <div className="topology__technical">
                    <h4>Technical details</h4>
                    <dl>
                      <div>
                        <dt>Interface index</dt>
                        <dd>{current.index}</dd>
                      </div>
                      <div>
                        <dt>MAC</dt>
                        <dd>{current.mac || "Unavailable"}</dd>
                      </div>
                      <div>
                        <dt>MTU</dt>
                        <dd>{current.mtu}</dd>
                      </div>
                    </dl>
                    <ul>
                      {routes.map((route, index) => (
                        <li
                          key={`${route.family}:${route.table}:${route.destination}:${index}`}
                        >
                          <code>
                            {route.destination} via {route.gateway || "direct"}
                          </code>
                          <small>
                            table {route.table || "unspecified"} · metric{" "}
                            {route.metric} ·{" "}
                            {route.protocol || "protocol unavailable"}
                          </small>
                        </li>
                      ))}
                    </ul>
                    <p>
                      Bridge/VLAN membership, DHCP identity, tunnel dependencies
                      and traffic rates are not yet included in this
                      observation.
                    </p>
                  </div>
                )}
              </>
            ) : (
              <div className="topology__selection">
                <Icon name="network" />
                <h3>
                  {selected
                    ? "This interface is no longer in the snapshot"
                    : "Choose an interface"}
                </h3>
                <p>
                  {selected
                    ? "It may have been removed, renamed or replaced. Select a current object to inspect it."
                    : "See its assigned addresses and route relationships without changing the network."}
                </p>
              </div>
            )}
          </aside>
        </div>
        <p className="topology__scope">
          Lines show host membership, not physical cabling. WAN/LAN roles, Wi-Fi
          clients and VPN topology are not inferred from interface names.
        </p>
      </div>
    </section>
  );
}
