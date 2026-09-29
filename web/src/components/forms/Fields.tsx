/**
 * DuckOS V1 — typed form input primitives.
 *
 * Every input is a controlled React component that emits validated
 * values through `onChange`. Composing inputs in forms is always done
 * through `useFormState` so we have one source of truth for validation,
 * dirty state, and submit.
 */
import {
  type KeyboardEvent,
  type ReactNode,
  useCallback,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  validateCidr,
  validateHostname,
  validateIpv4,
  validateIpv6,
  validateMac,
  validatePort,
  validatePortRange,
  validateSeconds,
  validateVlanId,
  type Validation,
} from "./validators";
import { t, type Messages } from "../../i18n/messages";

/* — Field wrapper with label + help + inline error — */
export function Field({
  label,
  help,
  error,
  children,
  required,
}: {
  label: string;
  help?: ReactNode;
  error?: string;
  children: ReactNode;
  required?: boolean;
}) {
  return (
    <div className={`v1-form-field ${error ? "v1-form-field-error" : ""}`}>
      <label>
        {label}
        {required ? <span aria-hidden="true" className="v1-required"> *</span> : null}
      </label>
      {children}
      {help && !error && <div className="help">{help}</div>}
      {error && (
        <div className="v1-form-error" role="alert">
          {error}
        </div>
      )}
    </div>
  );
}

/* — TextInput — generic free text with optional validator — */
export function TextInput({
  value,
  onChange,
  validator,
  type = "text",
  placeholder,
  autoComplete,
  spellCheck,
  disabled,
  ariaLabel,
  mono,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  validator?: (raw: string) => Validation<unknown>;
  type?: "text" | "password" | "search";
  placeholder?: string;
  autoComplete?: string;
  spellCheck?: boolean;
  disabled?: boolean;
  ariaLabel?: string;
  mono?: boolean;
}) {
  const [raw, setRaw] = useState(value);
  const id = useId();
  return (
    <input
      id={id}
      className={`v1-input ${mono ? "v1-mono" : ""}`}
      type={type}
      autoComplete={autoComplete}
      spellCheck={spellCheck}
      disabled={disabled}
      aria-label={ariaLabel}
      placeholder={placeholder}
      value={raw}
      onChange={(event) => {
        const next = event.target.value;
        setRaw(next);
        if (validator) {
          const r = validator(next);
          onChange(r.ok ? String(r.value) : next, r.ok);
        } else {
          onChange(next, true);
        }
      }}
    />
  );
}

/* — Ipv4Input — typed IPv4 — */
export function Ipv4Input({
  value,
  onChange,
  disabled,
  ariaLabel,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
}) {
  return (
    <TextInput
      value={value}
      onChange={onChange}
      validator={validateIpv4}
      placeholder="192.0.2.1"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel={ariaLabel ?? "IPv4 address"}
      mono
    />
  );
}

/* — Ipv6Input — */
export function Ipv6Input({
  value,
  onChange,
  disabled,
  ariaLabel,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
}) {
  return (
    <TextInput
      value={value}
      onChange={onChange}
      validator={validateIpv6}
      placeholder="2001:db8::1"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel={ariaLabel ?? "IPv6 address"}
      mono
    />
  );
}

/* — CidrInput — */
export function CidrInput({
  value,
  onChange,
  family,
  disabled,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  family: "ipv4" | "ipv6";
  disabled?: boolean;
}) {
  return (
    <TextInput
      value={value}
      onChange={onChange}
      validator={(raw) => validateCidr(raw, family)}
      placeholder={family === "ipv4" ? "192.0.2.0/24" : "2001:db8::/64"}
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel={family === "ipv4" ? "IPv4 CIDR" : "IPv6 CIDR"}
      mono
    />
  );
}

/* — MacInput — */
export function MacInput({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <TextInput
      value={value}
      onChange={onChange}
      validator={validateMac}
      placeholder="aa:bb:cc:dd:ee:ff"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel="MAC address"
      mono
    />
  );
}

/* — HostnameInput — */
export function HostnameInput({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <TextInput
      value={value}
      onChange={onChange}
      validator={validateHostname}
      placeholder="router.lan"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel="Hostname"
    />
  );
}

/* — VlanIdInput — */
export function VlanIdInput({
  value,
  onChange,
  disabled,
}: {
  value: number | "";
  onChange: (next: number | "", ok: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <TextInput
      value={value === "" ? "" : String(value)}
      onChange={(next, ok) => onChange(next === "" ? "" : Number(next), ok)}
      validator={(raw) => (raw === "" ? { ok: false, reason: "Empty" } : validateVlanId(raw))}
      placeholder="20"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel="VLAN id"
      mono
    />
  );
}

/* — PortInput — */
export function PortInput({
  value,
  onChange,
  disabled,
}: {
  value: number | "";
  onChange: (next: number | "", ok: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <TextInput
      value={value === "" ? "" : String(value)}
      onChange={(next, ok) => onChange(next === "" ? "" : Number(next), ok)}
      validator={(raw) => (raw === "" ? { ok: false, reason: "Empty" } : validatePort(raw))}
      placeholder="51820"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel="Port"
      mono
    />
  );
}

/* — PortRangeInput — */
export function PortRangeInput({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (next: string, ok: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <TextInput
      value={value}
      onChange={onChange}
      validator={validatePortRange}
      placeholder="1000-2000"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel="Port range"
      mono
    />
  );
}

/* — SecondsInput — */
export function SecondsInput({
  value,
  onChange,
  disabled,
  ariaLabel,
}: {
  value: number | "";
  onChange: (next: number | "", ok: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
}) {
  return (
    <TextInput
      value={value === "" ? "" : String(value)}
      onChange={(next, ok) => onChange(next === "" ? "" : Number(next), ok)}
      validator={(raw) => (raw === "" ? { ok: false, reason: "Empty" } : validateSeconds(raw))}
      placeholder="30"
      spellCheck={false}
      autoComplete="off"
      disabled={disabled}
      ariaLabel={ariaLabel ?? "Seconds"}
      mono
    />
  );
}

/* — Select — typed enum-like selector — */
export function Select<T extends string>({
  value,
  onChange,
  options,
  ariaLabel,
  disabled,
}: {
  value: T;
  onChange: (next: T) => void;
  options: Array<{ value: T; label: string }>;
  ariaLabel?: string;
  disabled?: boolean;
}) {
  return (
    <select
      className="v1-select"
      aria-label={ariaLabel}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value as T)}
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

/* — Checkbox — */
export function Checkbox({
  checked,
  onChange,
  label,
  disabled,
  ariaLabel,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: ReactNode;
  disabled?: boolean;
  ariaLabel?: string;
}) {
  return (
    <label className="v1-checkbox">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        disabled={disabled}
        aria-label={ariaLabel}
      />
      <span>{label}</span>
    </label>
  );
}

/* — MultiSelect — checkbox list — */
export function MultiSelect<T extends string>({
  options,
  selected,
  onChange,
  ariaLabel,
}: {
  options: Array<{ value: T; label: string; description?: string }>;
  selected: ReadonlyArray<T>;
  onChange: (next: T[]) => void;
  ariaLabel?: string;
}) {
  function toggle(v: T) {
    const set = new Set(selected);
    if (set.has(v)) set.delete(v);
    else set.add(v);
    onChange([...set]);
  }
  return (
    <div className="v1-multi-select" role="group" aria-label={ariaLabel}>
      {options.map((o) => (
        <label key={o.value} className="v1-multi-select-row">
          <input
            type="checkbox"
            checked={selected.includes(o.value)}
            onChange={() => toggle(o.value)}
          />
          <span>
            <strong>{o.label}</strong>
            {o.description && <div className="v1-subtle v1-fs-xs">{o.description}</div>}
          </span>
        </label>
      ))}
    </div>
  );
}

/* — ListEditor — string list with add/remove — */
export function ListEditor<T>({
  items,
  onChange,
  renderItem,
  addLabel,
  emptyLabel,
  validator,
  newItem,
  canRemove = () => true,
}: {
  items: ReadonlyArray<T>;
  onChange: (next: T[]) => void;
  renderItem: (item: T, set: (next: T) => void) => ReactNode;
  addLabel: string;
  emptyLabel: string;
  newItem: () => T;
  validator?: (item: T) => Validation<T>;
  canRemove?: (item: T) => boolean;
}) {
  function update(idx: number, next: T) {
    onChange(items.map((it, i) => (i === idx ? next : it)));
  }
  function remove(idx: number) {
    onChange(items.filter((_, i) => i !== idx));
  }
  function add() {
    onChange([...items, newItem()]);
  }
  return (
    <div className="v1-list-editor">
      {items.length === 0 && <p className="v1-mute v1-fs-sm">{emptyLabel}</p>}
      {items.map((it, idx) => {
        const ok = validator ? validator(it).ok : true;
        return (
          <div key={idx} className={`v1-list-row ${ok ? "" : "v1-list-row-error"}`}>
            <div className="v1-list-row-body">{renderItem(it, (next) => update(idx, next))}</div>
            {canRemove(it) && (
              <button
                type="button"
                className="v1-btn"
                data-variant="ghost"
                aria-label="Remove"
                onClick={() => remove(idx)}
              >
                ×
              </button>
            )}
          </div>
        );
      })}
      <button type="button" className="v1-btn" onClick={add}>
        {addLabel}
      </button>
    </div>
  );
}

/* — useFormState — tiny form helper used by every configurator. */
export function useFormState<T extends Record<string, unknown>>(
  initial: T,
  validators: Partial<Record<keyof T, (v: unknown) => Validation<unknown>>>,
) {
  const [values, setValues] = useState<T>(initial);
  const [dirty, setDirty] = useState<Set<keyof T>>(new Set());
  const set = useCallback(<K extends keyof T>(key: K, value: T[K]) => {
    setValues((s) => ({ ...s, [key]: value }));
    setDirty((d) => new Set(d).add(key));
  }, []);
  const errors = useMemo(() => {
    const out: Partial<Record<keyof T, string>> = {};
    for (const key of Object.keys(validators) as Array<keyof T>) {
      const v = validators[key];
      if (!v) continue;
      const r = v(values[key]);
      if (!r.ok) out[key] = r.reason;
    }
    return out;
  }, [values, validators]);
  const valid = Object.keys(errors).length === 0;
  const reset = useCallback(() => {
    setValues(initial);
    setDirty(new Set());
  }, [initial]);
  return { values, set, dirty, errors, valid, reset };
}

/* — Severity & state pills — existing CSS already supports these — */
export function SeverityPill({ state, label }: { state: "good" | "warn" | "bad" | "unknown"; label: string }) {
  return (
    <span className="v1-pill" data-state={state}>
      <span className="v1-dot" data-state={state} />
      {label}
    </span>
  );
}

export function RiskBadge({ risk, bundle }: { risk: "R" | "C" | "I"; bundle?: Messages }) {
  const label =
    risk === "R"
      ? "Read-only"
      : risk === "C"
        ? "Configurable"
        : bundle
          ? t(bundle, "candidate.noConfirmSupport").replace(".", "")
          : "Irreversible";
  return (
    <span className="v1-risk" data-risk={risk} title={label}>
      {risk}
    </span>
  );
}

/* — Number stepper — */
export function NumberStepper({
  value,
  onChange,
  min,
  max,
  step = 1,
  ariaLabel,
  disabled,
}: {
  value: number;
  onChange: (next: number) => void;
  min?: number;
  max?: number;
  step?: number;
  ariaLabel?: string;
  disabled?: boolean;
}) {
  return (
    <div className="v1-stepper" role="group" aria-label={ariaLabel}>
      <button
        type="button"
        className="v1-btn"
        disabled={disabled || (min !== undefined && value - step < min)}
        onClick={() => onChange(value - step)}
        aria-label="Decrement"
      >
        −
      </button>
      <span className="v1-mono v1-stepper-value">{value}</span>
      <button
        type="button"
        className="v1-btn"
        disabled={disabled || (max !== undefined && value + step > max)}
        onClick={() => onChange(value + step)}
        aria-label="Increment"
      >
        +
      </button>
    </div>
  );
}

/* — Press-to-confirm number editor with arrow keys — */
export function NumericInput({
  value,
  onChange,
  min,
  max,
  ariaLabel,
  disabled,
}: {
  value: number | "";
  onChange: (next: number | "", ok: boolean) => void;
  min?: number;
  max?: number;
  ariaLabel?: string;
  disabled?: boolean;
}) {
  const ref = useRef<HTMLInputElement | null>(null);
  return (
    <input
      ref={ref}
      className="v1-input v1-mono"
      type="number"
      min={min}
      max={max}
      value={value === "" ? "" : String(value)}
      aria-label={ariaLabel}
      disabled={disabled}
      onChange={(event) => {
        const raw = event.target.value;
        if (raw === "") {
          onChange("", false);
          return;
        }
        const n = Number(raw);
        if (Number.isFinite(n)) onChange(n, true);
        else onChange(raw as unknown as number, false);
      }}
      onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
        if (event.key === "ArrowUp" || event.key === "ArrowDown") {
          event.preventDefault();
          const delta = event.key === "ArrowUp" ? 1 : -1;
          const cur = typeof value === "number" ? value : 0;
          const next = cur + delta;
          if ((min === undefined || next >= min) && (max === undefined || next <= max)) {
            onChange(next, true);
          }
        }
      }}
    />
  );
}