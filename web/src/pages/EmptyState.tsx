/**
 * DuckOS V1 — V4 EmptyState.
 *
 * One empty-state component with strong defaults; per-domain copy
 * comes from the bundle. Anti-patterns: generic "{kind} of this kind
 * are currently visible" wording, debug counters.
 */
import { Icon } from "../icons";

export interface EmptyStateProps {
  icon: string;
  title: string;
  body: string;
  action?: string;
  onAction?: () => void;
  density?: "card" | "inline";
}

export function EmptyState(props: EmptyStateProps) {
  return (
    <div className={props.density === "inline" ? "duck-empty" : "duck-empty"}>
      <div className="e-icon">
        <Icon name={props.icon} size={28} />
      </div>
      <h3>{props.title}</h3>
      <p>{props.body}</p>
      {props.action && props.onAction && (
        <button type="button" onClick={props.onAction}>
          {props.action}
        </button>
      )}
    </div>
  );
}
