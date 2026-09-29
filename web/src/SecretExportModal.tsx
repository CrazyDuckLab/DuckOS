import { useEffect, useMemo, useRef, useState } from "react";
import { DuckMark } from "./icons";
import {
  qrMatrix,
  validateExport,
  type ExportAction,
  type SecretExport,
} from "./secretExport";
import "./SecretExportModal.css";

type Props = {
  id: string;
  revision: string;
  title: string;
  kind: SecretExport["kind"];
  load: (action: ExportAction, signal: AbortSignal) => Promise<SecretExport>;
  onClose: () => void;
};

// The owning client page supplies a stable authenticated, audited loader. Changing
// the resource revision clears stale material before resolving the new export.
export function SecretExportModal(props: Props) {
  return (
    <SecretExportSession
      key={`${props.kind}:${props.id}:${props.revision}`}
      {...props}
    />
  );
}

function SecretExportSession({
  id,
  revision,
  title,
  kind,
  load,
  onClose,
}: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const request = useRef<AbortController | null>(null);
  const [document, setDocument] = useState<SecretExport | null>(null);
  const [busy, setBusy] = useState(true);
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const previous = window.document.activeElement;
    const element = dialog.current;
    if (element && !element.open) {
      if (typeof element.showModal === "function") element.showModal();
      else element.setAttribute("open", "");
    }
    return () => {
      request.current?.abort();
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    request.current?.abort();
    request.current = controller;
    setDocument(null);
    setBusy(true);
    setFailed(false);
    setMessage("");
    load("qr", controller.signal)
      .then((value) => {
        if (!controller.signal.aborted)
          setDocument(validateExport(value, id, kind));
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setFailed(true);
          setMessage(
            "The export is unavailable. Check your access or reopen this client.",
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [id, revision, kind, load]);

  const matrix = useMemo(
    () => (document ? qrMatrix(document.payload) : null),
    [document],
  );
  const path = useMemo(() => {
    if (!matrix) return "";
    const parts: string[] = [];
    for (let y = 0; y < matrix.size; y++)
      for (let x = 0; x < matrix.size; x++) {
        if (matrix.data[y * matrix.size + x])
          parts.push(`M${x + 4} ${y + 4}h1v1h-1z`);
      }
    return parts.join("");
  }, [matrix]);

  function close() {
    request.current?.abort();
    setDocument(null);
    dialog.current?.close?.();
    onClose();
  }

  async function act(action: "copy" | "download") {
    const controller = new AbortController();
    request.current?.abort();
    request.current = controller;
    setBusy(true);
    setMessage("");
    try {
      const fresh = validateExport(
        await load(action, controller.signal),
        id,
        kind,
      );
      if (controller.signal.aborted) return;
      setDocument(fresh);
      if (action === "copy") {
        await navigator.clipboard.writeText(fresh.payload);
        if (!controller.signal.aborted)
          setMessage("Copied. Keep your clipboard private.");
      } else {
        const url = URL.createObjectURL(
          new Blob([fresh.payload], { type: "text/plain;charset=utf-8" }),
        );
        const link = window.document.createElement("a");
        link.href = url;
        link.download = fresh.filename;
        link.click();
        // Give the browser time to consume the blob, then release its secret bytes.
        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
        setMessage("Download requested. Store the file securely.");
      }
    } catch {
      if (!controller.signal.aborted) {
        setDocument(null);
        setFailed(true);
        setMessage(
          "The export could not be completed. Reopen this client to try again.",
        );
      }
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className="secret-export"
      aria-labelledby="secret-export-title"
      onCancel={(event) => {
        event.preventDefault();
        close();
      }}
    >
      <header className="secret-export__header">
        <DuckMark />
        <div>
          <p className="secret-export__eyebrow">Private client export</p>
          <h2 id="secret-export-title">{title}</h2>
        </div>
        <button
          type="button"
          className="secret-export__close"
          onClick={close}
          aria-label="Close private export"
        >
          ×
        </button>
      </header>
      <p className="secret-export__instruction">
        {kind === "wireguard"
          ? "Open WireGuard on your phone and add a tunnel by scanning this code."
          : "Scan this code with your subscription client to add this connection."}
      </p>
      <div className="secret-export__qr">
        {matrix ? (
          <svg
            viewBox={`0 0 ${matrix.size + 8} ${matrix.size + 8}`}
            role="img"
            aria-label="Private connection QR code"
            shapeRendering="crispEdges"
          >
            <rect width="100%" height="100%" fill="white" />
            <path d={path} fill="black" />
          </svg>
        ) : (
          <p role="status">
            {failed ? "QR unavailable" : "Preparing your secure QR…"}
          </p>
        )}
      </div>
      <p className="secret-export__warning">
        Anyone who scans this code or obtains the exported{" "}
        {kind === "wireguard" ? "configuration" : "URL"} can use this
        connection. Do not share it publicly.
      </p>
      <div className="secret-export__actions">
        <button
          type="button"
          disabled={busy || !document}
          onClick={() => void act("copy")}
        >
          {kind === "wireguard" ? "Copy config" : "Copy URL"}
        </button>
        <button
          type="button"
          disabled={busy || !document}
          onClick={() => void act("download")}
        >
          {kind === "wireguard" ? "Download .conf" : "Export"}
        </button>
      </div>
      <p className="secret-export__status" role="status">
        {message}
      </p>
      <details>
        <summary>Export details</summary>
        <p>
          QR, copy and download use the same generated content. Each export
          checks your access again. Closing this window removes the displayed
          QR; downloaded files and clipboard content remain your responsibility.
        </p>
      </details>
    </dialog>
  );
}
