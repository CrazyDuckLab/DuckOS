# DuckOS V1 — Qualification Network Lab

Built on `duck-dev-01` (Debian 13). All assets are disposable and
isolated from the protected management boundary
(`eth0` [redacted management address], GW [redacted management address]).

## Topology

```
                    ducklab-br0  (host bridge)
                    10.99.99.1/24 + fd00:99::1/64
                          |
    +-------+-------+-------+-------+-------+
    |       |       |       |       |       |
ducklab-  ducklab- ducklab- ducklab- ducklab- (no iface)
wanA      lanA    clientA  peerA    qosA
   |         |        |       |       |
ducklab-  ducklab- ducklab- ducklab- ducklab- (namespace end)
wanB      lanB    clientB  peerB    qosB
10.99.99.10/24 20/24  30/24   40/24   50/24
```

## Namespaces

| Namespace        | Endpoint          | Role                              |
|------------------|-------------------|-----------------------------------|
| `ducklab-wan`    | 10.99.99.10/24    | simulated upstream / WAN peer     |
| `ducklab-lan`    | 10.99.99.20/24    | simulated LAN host                |
| `ducklab-client` | 10.99.99.30/24    | simulated client / DHCP target    |
| `ducklab-peer`   | 10.99.99.40/24    | simulated tunnel / BGP peer       |
| `ducklab-qos`    | 10.99.99.50/24    | simulated QoS / tc subject        |
| `ducklab-isolated`| (lo only)         | isolated no-route sandbox         |

## Verification

```
sudo ip netns list | grep ducklab
sudo ip -br addr show ducklab-br0
sudo ip netns exec ducklab-client ping -c 1 -W 1 10.99.99.10
```

## Teardown

```
sudo ip link del ducklab-br0
for ns in wan lan client peer qos isolated; do
  sudo ip netns del ducklab-$ns
done
```

## Guard rails

- eth0 / [redacted management address] / [redacted management address] are NEVER referenced from the lab.
- Lab bridges use 10.99.99.0/24 and fd00:99::/64, both outside the management subnet.
- All test assets (dummy, veth, ns) are removed by the teardown snippet.
