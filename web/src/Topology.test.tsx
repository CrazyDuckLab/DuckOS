import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { Topology } from "./Topology";
import { overviewFixture } from "./test/fixture";

it("shows observed relationships without probing or inferring interface roles", () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  render(<Topology data={overviewFixture()} stale={false} />);
  expect(screen.getByText("Choose an interface")).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: "Inspect interface test-lan" }),
  );
  const details = within(
    screen.getByRole("complementary", { name: "Selected topology object" }),
  );
  expect(details.getByText("192.0.2.1/24")).toBeVisible();
  expect(details.getByText("2001:db8::1/64")).toBeVisible();
  expect(details.getByText("192.0.2.254")).toBeVisible();
  expect(
    details.getByText(/does not prove Internet reachability/),
  ).toBeVisible();
  expect(fetch).not.toHaveBeenCalled();
  expect(screen.getByText(/not inferred from interface names/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Full" }));
  expect(details.getByText("MTU")).toBeVisible();
  expect(details.getByText(/metric 100/)).toBeVisible();
});

it("marks partial stale data and drops selection when an interface changes identity", () => {
  const data = overviewFixture();
  data.issues = [
    {
      component: "routes",
      code: "partial",
      message: "Synthetic partial fixture",
    },
  ];
  const view = render(<Topology data={data} stale={true} />);
  expect(screen.getByText("Stale snapshot · incomplete")).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: "Inspect interface test-lan" }),
  );
  const changed = structuredClone(data);
  changed.interfaces[0].mac = "02:00:00:00:00:ff";
  view.rerender(<Topology data={changed} stale={false} />);
  expect(
    screen.getByText("This interface is no longer in the snapshot"),
  ).toBeVisible();
  expect(screen.queryByText("192.0.2.1/24")).not.toBeInTheDocument();
});

it("does not fabricate links when observations are absent", () => {
  const data = overviewFixture();
  data.interfaces = [];
  data.routes = [];
  render(<Topology data={data} stale={false} />);
  expect(screen.getByText(/No interfaces were reported/)).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /Inspect interface/ }),
  ).not.toBeInTheDocument();
  expect(screen.queryByText("Internet connected")).not.toBeInTheDocument();
});

it("filters observed objects without changing host data", () => {
  render(<Topology data={overviewFixture()} stale={false} />);
  fireEvent.change(
    screen.getByRole("searchbox", { name: "Find an interface" }),
    { target: { value: "2001:db8" } },
  );
  expect(screen.getByText("1 of 2 observed interfaces")).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "Inspect interface test-idle" }),
  ).not.toBeInTheDocument();
});
