/**
 * DuckOS V1 — typed state panels.
 *
 * Every page renders one of these instead of inventing a state.
 * They share the same vocabulary across the dashboard so a
 * "loading" pill in one place behaves identically to a "loading"
 * panel elsewhere.
 */
import { type ReactNode } from "react";

export type State =
  | "loading"
  | "empty"
  | "error"
  | "degraded"
  | "permission"
  | "missing-contract"
  | "stale"
  | "unsupported"
  | "intentionally-disabled";

interface StatePanelProps {
  state: State;
  title?: string;
  body?: ReactNode;
  actions?: ReactNode;
  detail?: ReactNode;
}

export function StatePanel({
  state,
  title,
  body,
  actions,
  detail,
}: StatePanelProps) {
  return (
    <div className="v1-state" data-state={state}>
      <div className="state-head">
        <span className="v1-dot" data-state={dotFor(state)} />
        <h3>{title ?? defaultTitle(state)}</h3>
      </div>
      <div className="state-body">{body ?? defaultBody(state)}</div>
      {detail && <div className="state-detail">{detail}</div>}
      {actions && <div className="state-actions">{actions}</div>}
    </div>
  );
}

function dotFor(state: State): "good" | "warn" | "bad" | "unknown" {
  switch (state) {
    case "loading":
      return "unknown";
    case "empty":
      return "unknown";
    case "error":
      return "bad";
    case "degraded":
      return "warn";
    case "permission":
      return "bad";
    case "missing-contract":
      return "warn";
    case "stale":
      return "warn";
    case "unsupported":
      return "warn";
    case "intentionally-disabled":
      return "unknown";
  }
}

function defaultTitle(state: State): string {
  switch (state) {
    case "loading":
      return "Loading";
    case "empty":
      return "Nothing here yet";
    case "error":
      return "Could not reach the system";
    case "degraded":
      return "Some components are stale";
    case "permission":
      return "Permission denied";
    case "missing-contract":
      return "API contract not implemented yet";
    case "stale":
      return "Observation is stale";
    case "unsupported":
      return "Unsupported on this platform";
    case "intentionally-disabled":
      return "Intentionally disabled";
  }
}

function defaultBody(state: State): string {
  switch (state) {
    case "loading":
      return "Reading the system…";
    case "empty":
      return "No objects to display.";
    case "error":
      return "We could not reach the API. Check the Engine and Agent.";
    case "degraded":
      return "Some objects could not be observed. Values shown may be partial.";
    case "permission":
      return "Your role does not include this view.";
    case "missing-contract":
      return "The page exists in the V1 catalog but the public API does not yet expose the data needed to populate it. The UI stays empty on purpose — no fake success.";
    case "stale":
      return "We have an older snapshot, but no fresh observation has arrived within the freshness window. Values shown reflect what was last seen.";
    case "unsupported":
      return "This feature requires capabilities not present on this system.";
    case "intentionally-disabled":
      return "An operator has turned this off. It is not a fault.";
  }
}
