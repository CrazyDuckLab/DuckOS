import { expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { APIClient } from "../api/client";
import { PagesRouter } from "../pages/Router";
import { messages } from "../i18n/messages";

for (const locale of ["en", "ru", "de", "es"] as const) {
  it(`beta.4 Xray Center route reaches authenticated production client: ${locale}`, async () => {
    document.documentElement.setAttribute("data-locale", locale);
    const client = new APIClient({ token: "synthetic-route-test", locale });
    const requests = vi.fn(async (_path: string) => ({
      ok: false as const,
      reason: "forbidden" as const,
      message: "forbidden",
      status: 403,
    }));
    vi.spyOn(client, "fetch").mockImplementation(requests);
    render(
      <PagesRouter
        route="/tunnels/xray/profiles"
        navigate={() => {}}
        bundle={messages[locale]}
        client={client}
        token="synthetic-route-test"
      />,
    );
    await waitFor(() => expect(requests.mock.calls.length).toBeGreaterThan(0));
    const paths = requests.mock.calls.map((args) => String(args[0]));
    expect(paths.some((path) => path.startsWith("/api/v1/xray"))).toBe(true);
    expect(paths.some((path) => path.startsWith("/api/v1/objects"))).toBe(
      false,
    );
  });
}
