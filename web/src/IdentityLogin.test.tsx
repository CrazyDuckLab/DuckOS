import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IdentityLogin } from "./IdentityLogin";
import { Startup } from "./Startup";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const response = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

describe("local identity", () => {
  it("enrolls before login and keeps credentials out of browser storage", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(response({ id: "test-owner" }, 201))
      .mockResolvedValueOnce(response({ token: "a".repeat(64) }));
    vi.stubGlobal("fetch", fetcher);
    const connected = vi.fn();
    render(<IdentityLogin enroll error="" onConnect={connected} />);
    fireEvent.change(screen.getByLabelText("Installation bootstrap token"), {
      target: { value: "synthetic-bootstrap" },
    });
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "owner" },
    });
    fireEvent.change(screen.getByLabelText("Password", { exact: true }), {
      target: { value: "Synthetic UI password 42!" },
    });
    fireEvent.change(screen.getByLabelText("Confirm password"), {
      target: { value: "Synthetic UI password 42!" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Create Owner/ }));
    await waitFor(() => expect(connected).toHaveBeenCalledWith("a".repeat(64)));
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/auth/enroll");
    expect(fetcher.mock.calls[1][0]).toBe("/api/v1/auth/login");
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });
  it("does not submit mismatched enrollment passwords", () => {
    const fetcher = vi.fn();
    vi.stubGlobal("fetch", fetcher);
    render(<IdentityLogin enroll error="" onConnect={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Password", { exact: true }), {
      target: { value: "Synthetic UI password 42!" },
    });
    fireEvent.submit(
      screen.getByRole("button", { name: /Create Owner/ }).closest("form")!,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Passwords do not match",
    );
    expect(fetcher).not.toHaveBeenCalled();
  });
  it("shows rate limiting without reflecting server credential data", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(response({ password: "must not reflect" }, 429)),
    );
    render(<IdentityLogin enroll={false} error="" onConnect={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "owner" },
    });
    fireEvent.change(screen.getByLabelText("Password", { exact: true }), {
      target: { value: "Synthetic UI password 42!" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Open workspace" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Too many attempts",
    );
    expect(screen.queryByText("must not reflect")).not.toBeInTheDocument();
  });
  it("selects enrollment from real server capability response", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          response({ mode: "local-sessions", enrolled: false }),
        ),
    );
    render(<Startup />);
    expect(
      await screen.findByRole("heading", { name: "Create your Owner" }),
    ).toBeInTheDocument();
  });
  it("does not downgrade on identity service failure", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response({}, 503)));
    render(<Startup />);
    expect(
      await screen.findByText(/No fallback credentials/),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Administrator token"),
    ).not.toBeInTheDocument();
  });
});
