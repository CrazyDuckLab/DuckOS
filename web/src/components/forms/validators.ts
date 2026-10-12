/**
 * DuckOS V1 — typed network validators.
 *
 * Every input that touches a network address, hostname, port, etc. uses
 * these. They never throw; they return `{ ok: true, value }` or
 * `{ ok: false, reason }` so forms can compose them with
 * `combine()` for whole-form validation.
 */

export type Validation<T> =
  { ok: true; value: T } | { ok: false; reason: string };

/** Combine several validators for the same field — all must succeed. */
export function combine<T>(...vs: Array<Validation<T>>): Validation<T> {
  for (const v of vs) if (!v.ok) return v;
  return vs[0];
}

/* — IPv4 — */
const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

export function validateIpv4(input: string): Validation<string> {
  const trimmed = input.trim();
  const m = IPV4.exec(trimmed);
  if (!m) return { ok: false, reason: "Not an IPv4 address." };
  for (const part of m.slice(1)) {
    const n = Number(part);
    if (n < 0 || n > 255)
      return { ok: false, reason: "IPv4 octet out of range." };
  }
  return { ok: true, value: trimmed };
}

/* — IPv6 — basic; full canonicalisation lives in the agent — */
const IPV6 = /^[0-9a-fA-F:]+$/;

export function validateIpv6(input: string): Validation<string> {
  const v = input.trim();
  if (v.length === 0) return { ok: false, reason: "Empty IPv6 address." };
  if (!IPV6.test(v))
    return { ok: false, reason: "IPv6 contains illegal characters." };
  if (v.includes(":::") || v.split("::").length > 2)
    return { ok: false, reason: "IPv6 :: appears more than once." };
  const parts = v.split("::");
  const left = (parts[0] ?? "").split(":").filter((g) => g.length > 0);
  const right = (parts.length === 2 ? (parts[1] ?? "") : "")
    .split(":")
    .filter((g) => g.length > 0);
  // Either an exact 8-group address, or one that uses :: to mean ≥ 1 zero group.
  if (parts.length === 1) {
    if (left.length !== 8)
      return { ok: false, reason: "IPv6 must be 8 groups." };
  } else {
    if (left.length + right.length >= 8)
      return { ok: false, reason: "IPv6 cannot exceed 8 groups." };
  }
  for (const g of [...left, ...right]) {
    if (g.length > 4)
      return { ok: false, reason: "IPv6 group longer than 4 hex digits." };
  }
  return { ok: true, value: v };
}

/* — CIDR — */
export function validateCidr(
  input: string,
  family: "ipv4" | "ipv6",
): Validation<{ address: string; prefix: number }> {
  const idx = input.indexOf("/");
  if (idx < 0) return { ok: false, reason: "CIDR requires /prefix." };
  const addr = input.slice(0, idx);
  const pfx = Number(input.slice(idx + 1));
  if (!Number.isInteger(pfx))
    return { ok: false, reason: "Prefix is not an integer." };
  const max = family === "ipv4" ? 32 : 128;
  if (pfx < 0 || pfx > max)
    return { ok: false, reason: `Prefix out of range (0-${max}).` };
  const av = family === "ipv4" ? validateIpv4(addr) : validateIpv6(addr);
  if (!av.ok) return { ok: false, reason: av.reason };
  return { ok: true, value: { address: av.value, prefix: pfx } };
}

/* — MAC — */
const MAC = /^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$/;

export function validateMac(input: string): Validation<string> {
  const v = input.trim().toLowerCase();
  if (!MAC.test(v))
    return { ok: false, reason: "MAC must be 6 colon-separated hex octets." };
  return { ok: true, value: v };
}

/* — Hostname — RFC 1123 host name (without trailing dot). */
const HOSTNAME =
  /^(?=.{1,253}$)([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;

export function validateHostname(input: string): Validation<string> {
  const v = input.trim();
  if (v.length === 0) return { ok: false, reason: "Hostname is empty." };
  if (!HOSTNAME.test(v)) return { ok: false, reason: "Invalid hostname." };
  return { ok: true, value: v };
}

/* — VLAN ID — */
export function validateVlanId(input: string | number): Validation<number> {
  const n = typeof input === "number" ? input : Number(input);
  if (!Number.isInteger(n) || n < 1 || n > 4094)
    return { ok: false, reason: "VLAN id must be 1–4094." };
  return { ok: true, value: n };
}

/* — Port / port-range — */
export function validatePort(input: string | number): Validation<number> {
  const n = typeof input === "number" ? input : Number(input);
  if (!Number.isInteger(n) || n < 1 || n > 65535)
    return { ok: false, reason: "Port must be 1–65535." };
  return { ok: true, value: n };
}

export function validatePortRange(
  input: string,
): Validation<{ start: number; end: number }> {
  const trimmed = input.trim();
  if (!trimmed) return { ok: false, reason: "Empty port range." };
  const parts = trimmed.split(/[:-]/).map((p) => p.trim());
  if (parts.length !== 2)
    return { ok: false, reason: "Port range must be START-END." };
  const start = validatePort(parts[0]);
  const end = validatePort(parts[1]);
  if (!start.ok) return { ok: false, reason: start.reason };
  if (!end.ok) return { ok: false, reason: end.reason };
  if (end.value < start.value)
    return { ok: false, reason: "End must be ≥ start." };
  return { ok: true, value: { start: start.value, end: end.value } };
}

/* — Duration (seconds) — */
export function validateSeconds(input: string | number): Validation<number> {
  const n = typeof input === "number" ? input : Number(input);
  if (!Number.isFinite(n) || n < 0)
    return { ok: false, reason: "Duration must be ≥ 0." };
  return { ok: true, value: Math.floor(n) };
}
