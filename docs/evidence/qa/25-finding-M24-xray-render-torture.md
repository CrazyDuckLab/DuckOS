# §4 Xray render torture — M-24

## Scope

Destructive QA of `internal/xray` typed configuration model
(without booting the real binary; that is covered by
`internal/qa/xray_linux_test.go` and is NETWORK_REQUIRED).

11 subtests across protocol whitelist, Reality/Trojan mutex,
JSON validity, disabled-server exclusion, user-server
references, port collision, profile routing, random fuzz,
subscription contract.

## M-24: Profile.Validate was not invoked in buildRoutingRule

### Reproduction

```go
profiles := []Profile{{ID: "11111111111111111111111111111111",
                      Name: "bad-user",
                      UserID: "",            // missing
                      Intent: ProfileIntentFullTunnel}}
RenderConfig(servers, users, profiles, nil)
// before fix: err == nil; routing rule built from an
// invalid profile (UserID = "") — the profile would be
// rendered and forwarded to Xray.
```

### Root cause

`buildRoutingRule` filtered profiles by `Enabled` and
`IsExpired` but never called `Profile.Validate()`. A profile
with an empty `UserID` or a non-canonical `Intent` slipped
through and was used to compose the routing rule. The
canonical `Profile.Validate()` exists in `server.go` and is
correct — it was simply not invoked on the render path.

### Fix

`internal/xray/render.go`: `buildRoutingRule` now calls
`p.Validate()` for every profile before including it in the
routing rule. Invalid profiles are silently dropped (matching
the existing `Enabled` / `IsExpired` skip behaviour); this is
documented as M-24.

### Regression

- `TestTorture_ProfileRouting` (new) — calls `Profile.Validate`
  on good / empty-UserID / non-canonical-Intent variants and
  asserts the rejection.

## Coverage

`internal/xray/torture_test.go`:

| Test                              | Concern                              |
|-----------------------------------|--------------------------------------|
| `ProtocolWhitelist`               | canonical VLESS/Trojan; reject others|
| `RealityTrojanMutex`              | REALITY requires VLESS                |
| `RenderJSONValidity`              | rendered JSON has inbounds/outbounds  |
| `DisabledServerExcluded`          | enabled=false → not in render         |
| `UserServerReference`             | user.ServerID unknown → rejected      |
| `PortCollision`                   | duplicate listen:port → rejected      |
| `ProfileRouting`                  | empty UserID / bad Intent rejected    |
| `RandomServerFuzz`                | 32 random servers: no panic, no nil   |
| `SubscriptionContract`            | JSON shape: name + profileIds         |

`go test -count=1 -timeout 60s -run TestTorture_
./internal/xray/...` — pass, 4 ms.

## Boundary check

Management eth0 (`172.18.4.149/16`, default via
`172.18.1.254`, sshd pid 758) was not touched — these tests
are pure Go, no binary, no network.
