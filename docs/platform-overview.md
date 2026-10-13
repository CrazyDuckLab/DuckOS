# Inside DuckOS

DuckOS beta.2 is a local Linux control plane on Debian 13. The diagrams below
show component relationships, not a claim that every catalog workflow is
qualified. See [current evidence](qualification-beta2.md) and [limitations](beta-limitations.md).

## One model, multiple entry points

```mermaid
flowchart TD
    Easy["Easy Web UI"] --> API["Authenticated API / Engine"]
    Full["Full Web UI"] --> API
    CLI["Source-built Duck CLI"] --> API
    Automation["Automation / API clients"] --> API
    API -->|"Private authenticated IPC"| Agent["duck-agent: zero effective capabilities"]
    Agent --> WG["WireGuard launcher: CAP_NET_ADMIN only"]
    Agent --> FRR["FRR launcher: bounded daemon lifecycle"]
    Agent --> XR["Xray runtime: Vault-backed materialization"]
    Agent --> Adapters["Other scoped Linux adapters"]
    WG --> Kernel["Linux kernel / host facilities"]
    FRR --> Daemons["FRR zebra / bgpd"]
    Daemons --> Kernel
    Adapters --> Kernel
    XR --> Xray["Xray user-space process"]
```

Easy and Full modes are parallel views of the same model. The CLI is source-built
in beta.2, not an installed release-archive binary. This diagram describes ordinary
public management commands; offline root-only Owner recovery is a separate CLI
operation. The browser never accesses private Agent IPC.

## Management and privilege separation

```mermaid
flowchart TD
    Browser["Browser / session"] -->|"SSH-forwarded loopback HTTP"| API["duck-api / Engine"]
    API --> Auth["Authentication + RBAC + semantic validation"]
    Auth -->|"Local authenticated IPC"| Agent["duck-agent / bounded operations / CapEff = 0"]
    Agent -->|"Typed WG operations"| WG["duck-wg-launcher / CAP_NET_ADMIN"]
    Agent -->|"Scoped FRR lifecycle"| FRR["duck-frr-launcher / bounded capability set"]
    Agent -->|"Resolve protected references"| Vault["Vault-backed Xray materialization"]
    Vault --> Xray["Validate config / manage Xray process"]
```

Agent IPC is local and private. Zero effective capabilities does not make the
Agent an unprivileged application: its root identity and scoped filesystem
permissions remain part of the threat model. FRR and WireGuard do not have
identical grants; the FRR launcher has a broader bounded set for privilege
drop and daemon operations. Actual units are authoritative. See
[the security model](security-model.md) and [vulnerability reporting](../SECURITY.md).

## Control paths and packet paths differ

```mermaid
flowchart TB
    Agent["Agent subsystem hosts"] -->|"Control"| WG["WG launcher: interface / peer config"]
    Agent -->|"Control"| FRR["FRR launcher: zebra / bgpd lifecycle"]
    Agent -->|"Control"| XR["Vault materialization + Xray runtime"]
    WG --> WGK["Kernel WireGuard interfaces"]
    FRR --> FIB["FRR daemons / kernel routes and FIB"]
    XR --> XRP["Xray user-space process"]
    Peer["WireGuard peer"] <-.->|"Encrypted traffic"| WGK
    Hosts["Routed hosts"] <-.->|"Forwarded traffic"| FIB
    Client["VLESS client"] <-.->|"VLESS / TLS / REALITY"| XRP
```

Solid arrows show selected control dependencies; dotted arrows illustrate
subsystem traffic. FRR updates the routing plane; ordinary forwarded packets
are handled by Linux, not by the API or a BGP web endpoint. Xray handles its
own user-space traffic. This is not a physical topology diagram or universal
packet route. Qualified beta.2 scenarios are listed in [the evidence summary](qualification-beta2.md).

## Safe Apply: operator actions and recovery

```mermaid
flowchart TD
    Intent["Network candidate"] --> Validate["Validate / protect management anchor"]
    Validate --> Snapshot["Durable before-state snapshot"]
    Snapshot --> Apply["Bounded apply and verification"]
    Apply --> Window{"Explicit confirmation in time?"}
    Window -->|"Confirmed"| Keep["Keep candidate / retain audit evidence"]
    Window -->|"Timeout or operator rollback"| Restore["Restore before-state / verify"]
    Apply -->|"Apply or verify failure"| Restore
    Restore --> Result{"Restoration verified?"}
    Result -->|"Yes"| Previous["Previous state active"]
    Result -->|"No"| Attention["Recovery required / preserve evidence"]
```

These are actions and outcomes, not a new state enum. The independent watchdog
and durable journal handle unconfirmed changes. Recovery failure is surfaced;
keep out-of-band access. Boot recovery is ordered before networkd to reconstruct
host routes/rules from durable evidence. See [Safe Apply](user/safe-apply.md)
and [recovery](user/recovery.md).
