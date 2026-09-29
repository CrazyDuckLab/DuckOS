/**
 * DuckOS V1 — generic configurator shell.
 *
 * Every mutation-capable V1 page has the same lifecycle:
 *   list → form (create / edit) → diff → apply → confirm | rollback
 *
 * This shell renders that lifecycle. Per-kind configurators
 * pass:
 *   - the typed list page (ObjectsPage)
 *   - the per-kind create/edit form (a React node)
 *   - the diff intent builder (kind → ConfigIntent)
 *
 * The shell owns:
 *   - the modal wrapper (Dialog / ConfirmDialog)
 *   - the Safe Apply phase indicator
 *   - the "Apply" / "Confirm" / "Rollback" buttons
 *   - the result / failure surfaces
 *
 * Use SafeApplyApi from useSafeApply(client).
 */
import { useCallback, useState, type ReactNode } from "react";
import { APIClient, type FetchResult } from "../api/client";
import type { ConfigIntent } from "../api/types";
import type { Messages } from "../i18n/messages";
import { Dialog } from "../components/forms/Dialog";
import type { SafeApplyApi } from "../state/safeApply";

export type ConfiguratorMode = "create" | "edit" | "delete";

export interface ConfiguratorShellProps {
  api: SafeApplyApi;
  /** i18n bundle (reserved for future localisation of modal copy). */
  bundle?: Messages;
  /** APIClient (reserved for direct non-Safe-Apply fallbacks). */
  client?: APIClient;
  /** Initial intent that the form has built. */
  buildIntent: () => ConfigIntent;
  /** Whether the operator can currently act. */
  canApply: boolean;
  /** Title for the action modal. */
  actionTitle: string;
  /** Body shown above the form (explains what will happen). */
  actionBody?: ReactNode;
  /** Form rendered inside the modal. */
  form: ReactNode;
  /** Close handler for the modal. */
  onClose: () => void;
  /** Optional override for the apply note. */
  applyNote?: string;
  /** When set, use a direct (non-Safe-Apply) call (e.g. for destructive
   * actions that bypass the transaction). */
  direct?: {
    label: string;
    action: () => Promise<FetchResult<unknown>>;
    dangerous?: boolean;
  };
}

export function ConfiguratorShell({
  api,
  buildIntent,
  canApply,
  actionTitle,
  actionBody,
  form,
  onClose,
  applyNote,
  direct,
}: ConfiguratorShellProps) {
  const [open, setOpen] = useState(true);
  const [directBusy, setDirectBusy] = useState(false);
  const [directError, setDirectError] = useState<string | null>(null);

  const close = useCallback(() => {
    setOpen(false);
    onClose();
  }, [onClose]);

  async function apply() {
    const intent = buildIntent();
    await api.diffAndApply(intent, { note: applyNote });
  }

  const phase = api.state.phase;
  const remaining = api.state.remainingSeconds ?? 0;
  const phaseLabel = phaseLabelFor(phase, remaining);

  return (
    <Dialog
      open={open}
      onClose={close}
      title={actionTitle}
      description={
        actionBody ?? (
          <p className="v1-mute">
            Changes go through the Safe Apply workflow:
            <strong> validate → apply → confirm</strong>. Confirm within the
            commit-confirmed window or the transaction is automatically
            rolled back.
          </p>
        )
      }
      primary={
        phase === "applied" || phase === "rolled-back"
          ? { label: "Done", onAction: close }
          : phase === "awaiting-confirmation"
            ? {
                label: remaining > 0
                  ? `Confirm (${remaining}s left)`
                  : "Window expired",
                onAction: () => {
                  void api.confirm();
                },
                variant: "primary",
                disabled: remaining <= 0,
              }
            : {
                label: "Validate & apply",
                onAction: () => {
                  void apply();
                },
                variant: "primary",
                disabled:
                  !canApply ||
                  phase === "diffing" ||
                  phase === "applying" ||
                  phase === "confirming" ||
                  phase === "rolling-back",
              }
      }
      secondary={
        phase === "awaiting-confirmation"
          ? {
              label: "Rollback",
              onAction: () => {
                void api.rollback("operator changed mind");
              },
            }
          : { label: "Cancel", onAction: close }
      }
    >
      <div className="v1-stack">
        {phase !== "idle" && phase !== "applied" && phase !== "rolled-back" && (
          <div className="v1-pill" data-tone={phaseTone(phase)} aria-live="polite">
            {phaseLabel}
          </div>
        )}
        {form}
        {api.state.error && (
          <div className="v1-pill" data-state="bad">
            {api.state.error.stage}: {api.state.error.message}
          </div>
        )}
        {phase === "applied" && (
          <div className="v1-pill" data-state="good">
            Confirmed — the system is at the new revision.
          </div>
        )}
        {phase === "rolled-back" && (
          <div className="v1-pill" data-state="warn">
            Rolled back — the system is at the original revision.
          </div>
        )}
        {direct && (
          <>
            {directError && (
              <div className="v1-pill" data-state="bad">{directError}</div>
            )}
            <button
              type="button"
              className="v1-btn"
              data-variant={direct.dangerous ? "danger" : "primary"}
              disabled={directBusy || !canApply}
              onClick={async () => {
                setDirectBusy(true);
                setDirectError(null);
                const r = await direct.action();
                setDirectBusy(false);
                if (!r.ok) setDirectError(r.message);
                else close();
              }}
            >
              {direct.label}
            </button>
          </>
        )}
      </div>
    </Dialog>
  );
}

function phaseLabelFor(phase: import("../state/safeApply").Phase, remainingSeconds: number): string {
  switch (phase) {
    case "idle":
      return "Idle";
    case "diffing":
      return "Diffing…";
    case "diffed":
      return "Diffed";
    case "applying":
      return "Applying…";
    case "awaiting-confirmation":
      return `Awaiting confirmation — ${remainingSeconds}s remaining`;
    case "confirming":
      return "Confirming…";
    case "rolling-back":
      return "Rolling back…";
    case "applied":
      return "Applied";
    case "rolled-back":
      return "Rolled back";
    case "failed":
      return "Failed";
  }
}

function phaseTone(phase: import("../state/safeApply").Phase): "good" | "warn" | "bad" | "unknown" {
  switch (phase) {
    case "awaiting-confirmation":
      return "warn";
    case "diffing":
    case "applying":
    case "confirming":
    case "rolling-back":
      return "unknown";
    case "failed":
      return "bad";
    default:
      return "unknown";
  }
}