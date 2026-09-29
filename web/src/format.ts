export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "Unavailable";
  if (bytes === 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  return `${(bytes / 1024 ** index).toLocaleString("en-US", { maximumFractionDigits: index > 0 ? 1 : 0 })} ${units[index]}`;
}

export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "Unavailable";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function percentage(used: number, total: number): number | null {
  if (
    !Number.isFinite(used) ||
    !Number.isFinite(total) ||
    used < 0 ||
    total <= 0
  )
    return null;
  return Math.min(100, (used / total) * 100);
}

export function percentLabel(value: number | null): string {
  return value === null || !Number.isFinite(value)
    ? "Unavailable"
    : `${value.toFixed(1)}%`;
}

export function formatTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "Unavailable"
    : date.toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
}
