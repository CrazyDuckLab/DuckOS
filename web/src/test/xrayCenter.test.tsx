// XrayCenterPage render test: capability gating, confirm dialog,
// help content, form refusal of invalid matrix combinations.
import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { XrayCenterPage } from "../xray/center";
import type { FetchResult } from "../api/client";
import type { PageMeta } from "../pages/registry";

const pageStub = {
  id: "P050",
  name: "Xray Center",
  route: "/tunnels/xray/profiles",
  question: "",
} as unknown as PageMeta;

function fakeClient(servers: unknown[] = []) {
  const deleted: string[] = [];
  const client = {
    fetchXrayServers: () =>
      Promise.resolve({
        ok: true,
        data: servers,
        age: 0,
      } as FetchResult<unknown[]>),
    fetchXrayUsers: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayCenterStats: () =>
      Promise.resolve({
        ok: true,
        data: { stale: false, ledger: {} },
        age: 0,
      } as FetchResult<{ stale: boolean; ledger?: Record<string, number> }>),
    fetchXrayDiagnostics: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<
        Array<{
          code: string;
          severity: string;
          summary: string;
          remediation: string;
        }>
      >),
    fetchXrayNodes: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayGroups: () =>
      Promise.resolve({ ok: true, data: [], age: 0 } as FetchResult<unknown[]>),
    fetchXrayStorage: () =>
      Promise.resolve({
        ok: true,
        data: { domains: {} },
        age: 0,
      } as FetchResult<unknown>),
    xrayCreateServer: vi.fn(() => Promise.resolve({ ok: true })),
    xrayDeleteServer: (id: string) => {
      deleted.push(id);
      return Promise.resolve({ ok: true });
    },
  };
  return { client, deleted };
}

describe("XrayCenterPage", () => {
  it("renders sections, offline help and the create form", () => {
    const { client } = fakeClient();
    render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        client={client as never}
      />,
    );
    expect(screen.getByText("Servers (inbounds)")).toBeTruthy();
    expect(screen.getByText("Clients")).toBeTruthy();
    expect(screen.getByText("Subscriptions")).toBeTruthy();
    // Offline help is present and expandable.
    fireEvent.click(screen.getByText("REALITY"));
    expect(screen.getByText(/steal|ordinary website|borrow/i)).toBeTruthy();
  });

  it("localizes to Russian", () => {
    const { client } = fakeClient();
    render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        locale="ru"
        client={client as never}
      />,
    );
    expect(screen.getByText("Серверы (входящие)")).toBeTruthy();
    expect(screen.getByText("Подписки")).toBeTruthy();
    expect(screen.getByText("Лимиты трафика")).toBeTruthy();
    expect(screen.getByText("Форматы подписок")).toBeTruthy();
  });

  it("gates the REALITY security option by the capability matrix", async () => {
    const { client } = fakeClient();
    const { container } = render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        client={client as never}
      />,
    );
    await vi.waitFor(() =>
      expect(screen.getAllByRole("combobox").length).toBeGreaterThan(2),
    );
    void container;
    const realityOption = () =>
      Array.from(
        screen.getAllByRole("combobox")[2].querySelectorAll("option"),
      ).find((o) => o.textContent === "reality") as HTMLOptionElement;
    // vless + tcp: REALITY allowed.
    fireEvent.change(screen.getAllByRole("combobox")[0], {
      target: { value: "vless" },
    });
    fireEvent.change(screen.getAllByRole("combobox")[1], {
      target: { value: "tcp" },
    });
    expect(realityOption().disabled).toBe(false);
    // vless + ws: REALITY refused.
    fireEvent.change(screen.getAllByRole("combobox")[1], {
      target: { value: "ws" },
    });
    expect(realityOption().disabled).toBe(true);
    // trojan + tcp: REALITY refused (protocol gate).
    fireEvent.change(screen.getAllByRole("combobox")[0], {
      target: { value: "trojan" },
    });
    fireEvent.change(screen.getAllByRole("combobox")[1], {
      target: { value: "tcp" },
    });
    expect(realityOption().disabled).toBe(true);
  });

  it("requires confirmation before deleting a server", async () => {
    const { client, deleted } = fakeClient([
      {
        ref: { id: "srv-1" },
        displayName: "primary",
        protocol: "vless",
        listen: "127.0.0.1",
        port: 443,
        userCount: 2,
      },
    ]);
    render(
      <XrayCenterPage
        page={pageStub}
        bundle={{} as never}
        client={client as never}
      />,
    );
    await vi.waitFor(() =>
      expect(screen.getAllByText("primary").length).toBeGreaterThan(0),
    );
    fireEvent.click(screen.getByText("Delete"));
    expect(deleted).toEqual([]);
    fireEvent.click(screen.getByRole("dialog").querySelector("button")!);
    await vi.waitFor(() => expect(deleted).toEqual(["srv-1"]));
  });
});
