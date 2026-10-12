/**
 * DuckOS V1 — per-kind intent builder tests.
 *
 * Exercises the type-safe round-trip from a typed `V` value
 * through each `KindBinding` to a `ConfigIntent.objects[]`
 * suitable for `POST /api/v1/config/diff` then `/apply`.
 *
 * Covers every binding exposed by `intentBuilders.tsx` and
 * validates:
 *   - create intent shape
 *   - update intent includes revision CAS when present
 *   - delete intent uses `desired: null`
 *   - validate() returns false on empty values
 */
import { describe, expect, it } from "vitest";
import { VLAN_BINDING, bindingForKind } from "../configurators/intentBuilders";
import { SUPPORTED_OBJECT_KINDS } from "../api/types";
import { messages } from "../i18n/messages";

describe("bindingForKind", () => {
  it("returns a binding for every supported object kind", () => {
    for (const kind of SUPPORTED_OBJECT_KINDS) {
      const b = bindingForKind(kind);
      // xray.profile and xray.subscription are explicitly read-only
      // at the V1 product level, so the registry may leave them as undefined.
      if (kind === "xray.profile" || kind === "xray.subscription") {
        expect(b).toBeUndefined();
        continue;
      }
      expect(b, `binding missing for kind ${kind}`).toBeDefined();
      expect(b!.createTitle(messages.en).length).toBeGreaterThan(0);
      expect(b!.editTitle(messages.en).length).toBeGreaterThan(0);
      expect(b!.deleteTitle(messages.en).length).toBeGreaterThan(0);
    }
  });

  it("returns undefined for xray templates", () => {
    expect(bindingForKind("xray.profile")).toBeUndefined();
    expect(bindingForKind("xray.subscription")).toBeUndefined();
  });

  it("the registry has no duplicate binding slots", () => {
    const seen = new Set<string>();
    for (const k of SUPPORTED_OBJECT_KINDS) {
      expect(seen.has(k), `duplicate registry slot for ${k}`).toBe(false);
      seen.add(k);
      if (bindingForKind(k)) seen.add(k);
    }
    // VLAN_BINDING is a separate exposed binding for the VLAN pages.
    expect(VLAN_BINDING).toBeDefined();
  });
});

describe("intent builders — create / update / delete", () => {
  it("interface binding produces the right objects[] shape", () => {
    const b = bindingForKind("network.interface")!;
    const v = {
      name: "eth0",
      adminState: "up" as const,
      mtu: 1500,
      ipv4: "10.0.0.1",
      ipv4Prefix: 24,
      ipv6: "",
      ipv6Prefix: 64,
    };
    const create = b.intentForCreate(v);
    expect(create.objects?.[0]?.kind).toBe("network.interface");
    expect(create.objects?.[0]?.id).toBeUndefined();
    expect((create.objects?.[0]?.desired as { name: string }).name).toBe(
      "eth0",
    );

    const upd = b.intentForUpdate("eth0", v, "rev-2");
    expect(upd.objects?.[0]?.id).toBe("eth0");
    expect(upd.objects?.[0]?.revision).toBe("rev-2");

    const del = b.intentForDelete("eth0", "rev-2");
    expect(del.objects?.[0]?.desired).toBeNull();
    expect(del.objects?.[0]?.revision).toBe("rev-2");
  });

  it("wireguard server binding marks private keys as not the dashboard's concern", () => {
    const b = bindingForKind("wireguard.server")!;
    const v = {
      displayName: "wg-edge",
      interface: "wg0",
      listenPort: 51820,
      addresses: ["10.66.66.1/24"],
      dns: [],
      mtu: 1420,
    };
    const intent = b.intentForCreate(v);
    expect(intent.objects?.[0]?.kind).toBe("wireguard.server");
    // The form does not request a private key — the Engine generates one.
    const desired = intent.objects?.[0]?.desired as Record<string, unknown>;
    expect(desired).not.toHaveProperty("privateKey");
  });

  it("bgp peer binding passes the password through unchanged (never logged, never round-tripped after apply)", () => {
    const b = bindingForKind("bgp.peer")!;
    const v = {
      displayName: "peer1",
      remoteAddr: "10.0.0.2",
      remoteAs: 64512,
      localAs: 64512,
      password: "secret-not-shown",
      enabled: true,
    };
    const intent = b.intentForCreate(v);
    const desired = intent.objects?.[0]?.desired as { password: string };
    expect(desired.password).toBe("secret-not-shown");
  });

  it("firewall rule delete intent uses null desired", () => {
    const b = bindingForKind("network.firewall.rule")!;
    const intent = b.intentForDelete("rule-1");
    expect(intent.objects?.[0]?.desired).toBeNull();
  });

  it("route map validate rejects empty destination", () => {
    const b = bindingForKind("route.map")!;
    expect(
      b.validate({
        destination: "",
        gateway: "10.0.0.1",
        metric: 100,
        iface: "lan",
        enabled: true,
      }),
    ).toBe(false);
    expect(
      b.validate({
        destination: "10.0.0.0/24",
        gateway: "10.0.0.1",
        metric: 100,
        iface: "lan",
        enabled: true,
      }),
    ).toBe(true);
  });

  it("vlan binding tags desired with kind=vlan", () => {
    const v = {
      vlanId: 100,
      name: "guest",
      parent: "lan",
      ipv4: "10.10.10.1",
      ipv4Prefix: 24,
    };
    const intent = VLAN_BINDING.intentForCreate(v);
    const desired = intent.objects?.[0]?.desired as Record<string, unknown>;
    expect(desired.kind).toBe("vlan");
    expect(desired.vlanId).toBe(100);
    expect(VLAN_BINDING.validate(v)).toBe(true);
    expect(
      VLAN_BINDING.validate({
        vlanId: 0,
        name: "",
        parent: "lan",
        ipv4: "",
        ipv4Prefix: 24,
      }),
    ).toBe(false);
  });

  it("dhcp subnet validate requires non-empty range endpoints", () => {
    const b = bindingForKind("dhcp.subnet")!;
    expect(
      b.validate({
        subnet: "10.20.0.0/24",
        rangeStart: "",
        rangeEnd: "10.20.0.254",
        gateway: "",
        leaseSeconds: 3600,
      }),
    ).toBe(false);
    expect(
      b.validate({
        subnet: "10.20.0.0/24",
        rangeStart: "10.20.0.1",
        rangeEnd: "10.20.0.254",
        gateway: "",
        leaseSeconds: 3600,
      }),
    ).toBe(true);
  });

  it("ospf area binding requires areaId", () => {
    const b = bindingForKind("ospf.area")!;
    expect(
      b.validate({ displayName: "backbone", areaId: "", enabled: true }),
    ).toBe(false);
    expect(
      b.validate({ displayName: "backbone", areaId: "0.0.0.0", enabled: true }),
    ).toBe(true);
  });
});
