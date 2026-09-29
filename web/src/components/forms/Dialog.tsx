/**
 * DuckOS V1 — dialogs.
 *
 * Modal/dialog with focus trap, Escape-to-close, and proper ARIA.
 * Used for confirmation, destructive actions, and detail views.
 */
import {
  type ReactNode,
  useEffect,
  useRef,
} from "react";

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: ReactNode;
  children?: ReactNode;
  primary?: {
    label: string;
    onAction: () => void;
    variant?: "primary" | "danger";
    disabled?: boolean;
  };
  secondary?: {
    label: string;
    onAction: () => void;
  };
}

export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  primary,
  secondary,
}: DialogProps) {
  const ref = useRef<HTMLDivElement | null>(null);
  const lastFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    lastFocused.current = document.activeElement as HTMLElement;
    function handler(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
      if (event.key === "Tab") {
        const root = ref.current;
        if (!root) return;
        const focusables = root.querySelectorAll<HTMLElement>(
          'a, button, input, textarea, select, [tabindex]:not([tabindex="-1"])',
        );
        if (focusables.length === 0) return;
        const first = focusables[0];
        const last = focusables[focusables.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", handler);
    // initial focus
    const root = ref.current;
    const first = root?.querySelector<HTMLElement>(
      'a, button, input, textarea, select, [tabindex]:not([tabindex="-1"])',
    );
    first?.focus();
    return () => {
      document.removeEventListener("keydown", handler);
      lastFocused.current?.focus?.();
    };
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div className="v1-dialog-backdrop" role="presentation" onClick={onClose}>
      <div
        className="v1-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="v1-dialog-title"
        ref={ref}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="v1-dialog-head">
          <h2 id="v1-dialog-title">{title}</h2>
          <button
            type="button"
            className="v1-btn"
            data-variant="ghost"
            aria-label="Close"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        {description && <div className="v1-dialog-body">{description}</div>}
        {children && <div className="v1-dialog-content">{children}</div>}
        <div className="v1-dialog-actions">
          {secondary && (
            <button
              type="button"
              className="v1-btn"
              onClick={secondary.onAction}
            >
              {secondary.label}
            </button>
          )}
          {primary && (
            <button
              type="button"
              className="v1-btn"
              data-variant={primary.variant ?? "primary"}
              disabled={primary.disabled}
              onClick={primary.onAction}
            >
              {primary.label}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger,
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  description: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}) {
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title}
      description={description}
      primary={{
        label: confirmLabel,
        onAction: onConfirm,
        variant: danger ? "danger" : "primary",
      }}
      secondary={{ label: cancelLabel, onAction: onClose }}
    />
  );
}