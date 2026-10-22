// Xray Center capability matrix mirror tests: the client refuses
// unsupported protocol/transport/security combinations before any
// network round-trip, matching the Go compatibility engine.
import { describe, expect, it } from "vitest";
import { APIClient } from "../api/client";

describe("xray capability mirror", () => {
  it("accepts the canonical protocol and transport sets", () => {
    expect(APIClient.XRAY_PROTOCOLS).toContain("vless");
    expect(APIClient.XRAY_PROTOCOLS).toContain("shadowsocks");
    expect(APIClient.XRAY_TRANSPORTS).toContain("xhttp");
    expect(APIClient.XRAY_TRANSPORTS).toContain("httpupgrade");
  });

  it("gates REALITY to vless over tcp or xhttp", () => {
    expect(APIClient.xrayRealityAllowed("vless", "tcp")).toBe(true);
    expect(APIClient.xrayRealityAllowed("vless", "xhttp")).toBe(true);
    expect(APIClient.xrayRealityAllowed("vless", "ws")).toBe(false);
    expect(APIClient.xrayRealityAllowed("trojan", "tcp")).toBe(false);
    expect(APIClient.xrayRealityAllowed("vmess", "xhttp")).toBe(false);
  });

  it("gates the vision flow to vless + tcp + REALITY", () => {
    expect(APIClient.xrayFlowAllowed("vless", "tcp", "reality")).toBe(true);
    expect(APIClient.xrayFlowAllowed("vless", "tcp", "tls")).toBe(false);
    expect(APIClient.xrayFlowAllowed("vless", "grpc", "reality")).toBe(false);
    expect(APIClient.xrayFlowAllowed("vmess", "tcp", "reality")).toBe(false);
  });

  it("exposes the AEAD and 2022 shadowsocks ciphers only", () => {
    expect(APIClient.XRAY_SS_METHODS).toContain("2022-blake3-aes-256-gcm");
    expect(APIClient.XRAY_SS_METHODS).not.toContain("rc4-md5");
  });
});
