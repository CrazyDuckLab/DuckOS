import { describe, expect, it, vi } from "vitest";
import jsQR from "jsqr";
import {
  exportBytes,
  fetchClientExport,
  MAX_QR_BYTES,
  qrMatrix,
} from "./secretExport";

function scan(payload: string) {
  const matrix = qrMatrix(payload);
  const scale = 4;
  const width = (matrix.size + 8) * scale;
  const pixels = new Uint8ClampedArray(width * width * 4).fill(255);
  for (let y = 0; y < width; y++) {
    for (let x = 0; x < width; x++) {
      const mx = Math.floor(x / scale) - 4;
      const my = Math.floor(y / scale) - 4;
      if (
        mx >= 0 &&
        my >= 0 &&
        mx < matrix.size &&
        my < matrix.size &&
        matrix.data[my * matrix.size + mx]
      ) {
        const offset = (y * width + x) * 4;
        pixels[offset] = pixels[offset + 1] = pixels[offset + 2] = 0;
      }
    }
  }
  return jsQR(pixels, width, width);
}

describe("secret QR encoding", () => {
  it.each([
    "https://vpn.example.invalid/subscription?token=test%2Bvalue%2F&name=phone",
    "[Interface]\nPrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\nAddress = 10.10.0.2/32\n\n[Peer]\nPublicKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = vpn.example.invalid:51820\n",
  ])("decodes to the exact export bytes", (payload) => {
    const result = scan(payload);
    expect(result?.data).toBe(payload);
    expect(result?.binaryData).toEqual(Array.from(exportBytes(payload)));
  });

  it("supports the maximum byte-mode M payload", () => {
    expect(qrMatrix("a".repeat(MAX_QR_BYTES)).size).toBe(177);
  });

  it("rejects unrepresentable payloads without echoing their contents", () => {
    for (const payload of ["", "secret".repeat(500), "secret\ud800"]) {
      expect(() => exportBytes(payload)).toThrow("supported QR code");
    }
  });
});

it("loads secrets through a bounded non-caching authenticated POST", async () => {
  const doc = {
    id: "a".repeat(32),
    revision: "b".repeat(32),
    kind: "subscription" as const,
    filename: "subscription.txt",
    payload: "https://vpn.example.invalid/s?token=private-test",
  };
  const fetch = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(doc), {
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetch);
  const signal = new AbortController().signal;
  expect(
    await fetchClientExport("session-test", doc.id, doc.kind, "qr", signal),
  ).toEqual(doc);
  expect(fetch).toHaveBeenCalledWith(
    "/api/v1/clients/export",
    expect.objectContaining({
      method: "POST",
      cache: "no-store",
      redirect: "error",
      credentials: "omit",
      signal,
      headers: expect.objectContaining({
        Authorization: "Bearer session-test",
      }),
    }),
  );
  expect(fetch.mock.calls[0][1].body).not.toContain(doc.payload);
});

it("refuses oversized and malformed secret responses without echoing them", async () => {
  for (const body of [
    "private-secret".repeat(2000),
    "private-secret-invalid-json",
  ]) {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(body, {
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    await expect(
      fetchClientExport(
        "session-test",
        "a".repeat(32),
        "subscription",
        "qr",
        new AbortController().signal,
      ),
    ).rejects.toThrow("Client export could not be verified.");
  }
});
