import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SecretExportModal } from "./SecretExportModal";
import type { SecretExport } from "./secretExport";

const original: SecretExport = {
  id: "a".repeat(32),
  revision: "b".repeat(32),
  kind: "subscription",
  filename: "subscription.txt",
  payload: "https://vpn.example.invalid/s?token=private-test-token",
};

afterEach(() => vi.restoreAllMocks());

function props(load = vi.fn().mockResolvedValue(original)) {
  return { ...original, title: "Phone subscription", load, onClose: vi.fn() };
}

it("renders only a private QR and authorizes copy again with fresh content", async () => {
  const fresh = {
    ...original,
    revision: "c".repeat(32),
    payload: original.payload + "-rotated",
  };
  const load = vi
    .fn()
    .mockResolvedValueOnce(original)
    .mockResolvedValueOnce(fresh);
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  const view = render(<SecretExportModal {...props(load)} />);
  await screen.findByRole("img", { name: "Private connection QR code" });
  expect(view.container.innerHTML).not.toContain(original.payload);
  expect(screen.getByText(/Anyone who scans/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Copy URL" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(fresh.payload));
  expect(load.mock.calls.map(([action]) => action)).toEqual(["qr", "copy"]);
});

it("removes the old QR during rotation and ignores an aborted response", async () => {
  let resolveOld!: (value: SecretExport) => void;
  const load = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<SecretExport>((resolve) => {
          resolveOld = resolve;
        }),
    )
    .mockResolvedValueOnce({
      ...original,
      revision: "c".repeat(32),
      payload: original.payload + "-new",
    });
  const initial = props(load);
  const view = render(<SecretExportModal {...initial} />);
  const oldSignal = load.mock.calls[0][1] as AbortSignal;
  view.rerender(<SecretExportModal {...initial} revision={"c".repeat(32)} />);
  expect(oldSignal.aborted).toBe(true);
  await screen.findByRole("img");
  const path = view.container.querySelector("svg path")?.getAttribute("d");
  resolveOld(original);
  await waitFor(() =>
    expect(view.container.querySelector("svg path")?.getAttribute("d")).toBe(
      path,
    ),
  );
});

it("does not expose failed response secrets and aborts on close", async () => {
  const load = vi.fn().mockRejectedValue(new Error(original.payload));
  const initial = props(load);
  const view = render(<SecretExportModal {...initial} />);
  await screen.findByText("QR unavailable");
  expect(view.container.innerHTML).not.toContain(original.payload);
  expect(screen.getByRole("button", { name: "Copy URL" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Close private export" }));
  expect((load.mock.calls[0][1] as AbortSignal).aborted).toBe(true);
  expect(initial.onClose).toHaveBeenCalledOnce();
});

it("downloads precisely the fresh payload and releases its temporary URL", async () => {
  const fresh = { ...original, payload: original.payload + "-rotated" };
  const load = vi
    .fn()
    .mockResolvedValueOnce(original)
    .mockResolvedValueOnce(fresh);
  let blob: Blob | undefined;
  const createObjectURL = vi.fn((value: Blob) => {
    blob = value;
    return "blob:private-export-test";
  });
  const revokeObjectURL = vi.fn();
  vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.download).toBe(fresh.filename);
    });
  render(<SecretExportModal {...props(load)} />);
  await screen.findByRole("img");
  fireEvent.click(screen.getByRole("button", { name: "Export" }));
  await waitFor(() => expect(click).toHaveBeenCalledOnce());
  const content = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = reject;
    reader.readAsText(blob!);
  });
  expect(content).toBe(fresh.payload);
  expect(load.mock.calls.map(([action]) => action)).toEqual(["qr", "download"]);
  await waitFor(
    () =>
      expect(revokeObjectURL).toHaveBeenCalledWith("blob:private-export-test"),
    { timeout: 2000 },
  );
});
