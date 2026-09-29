import QRCode from "qrcode";

export type ExportAction = "qr" | "copy" | "download";
export type SecretExport = {
  id: string;
  revision: string;
  kind: "wireguard" | "subscription";
  filename: string;
  payload: string;
};

export const MAX_QR_BYTES = 2331;

export function exportBytes(payload: string): Uint8Array {
  const bytes = new TextEncoder().encode(payload);
  if (
    !bytes.length ||
    bytes.length > MAX_QR_BYTES ||
    new TextDecoder().decode(bytes) !== payload
  ) {
    throw new Error(
      "This configuration cannot be represented by a supported QR code.",
    );
  }
  return bytes;
}

export function qrMatrix(payload: string) {
  return QRCode.create([{ data: exportBytes(payload), mode: "byte" }], {
    errorCorrectionLevel: "M",
  }).modules;
}

export function validateExport(
  value: SecretExport,
  id: string,
  kind: SecretExport["kind"],
): SecretExport {
  if (
    !value ||
    value.id !== id ||
    value.kind !== kind ||
    !/^[0-9a-f]{32}$/.test(value.revision) ||
    !/^[a-z][a-z0-9-]{0,47}\.(conf|txt)$/.test(value.filename) ||
    typeof value.payload !== "string" ||
    (kind === "wireguard" && !value.filename.endsWith(".conf"))
  ) {
    throw new Error("The client export could not be verified.");
  }
  exportBytes(value.payload);
  return value;
}

export async function fetchClientExport(
  token: string,
  id: string,
  kind: SecretExport["kind"],
  action: ExportAction,
  signal: AbortSignal,
): Promise<SecretExport> {
  const response = await fetch("/api/v1/clients/export", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ id, kind, action }),
    cache: "no-store",
    credentials: "omit",
    redirect: "error",
    referrerPolicy: "no-referrer",
    signal,
  });
  if (
    !response.ok ||
    response.headers.get("Content-Type") !== "application/json" ||
    !response.body
  ) {
    await response.body?.cancel();
    throw new Error("Client export is unavailable or access has changed.");
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > 16384)
        throw new Error("Client export exceeds response limits.");
      chunks.push(value);
    }
    const bytes = new Uint8Array(size);
    let offset = 0;
    for (const chunk of chunks) {
      bytes.set(chunk, offset);
      offset += chunk.length;
    }
    return validateExport(
      JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)),
      id,
      kind,
    );
  } catch {
    await reader.cancel().catch(() => undefined);
    throw new Error("Client export could not be verified.");
  } finally {
    reader.releaseLock();
  }
}
