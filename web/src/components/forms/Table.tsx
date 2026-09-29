/**
 * DuckOS V1 — table primitives.
 *
 * `DataTable` is the dense professional table for Full mode. It
 * supports sorting, pagination, and column-level selection.
 * `KeyValueList` is the small description list used in cards and
 * inspectors.
 */
import {
  type ReactNode,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { messages } from "../../i18n/messages";
import { PrefsContext } from "../../state/store";

export interface DataColumn<T> {
  /** Stable key — also used for sorting. */
  key: string;
  /** Header label. */
  label: string;
  /** Sortable? Defaults to true. */
  sortable?: boolean;
  /** Custom renderer. */
  render?: (row: T, index: number) => ReactNode;
  /** Value accessor for sorting. Defaults to row[key]. */
  sortValue?: (row: T) => string | number;
  /** Right-align? Defaults to false. */
  numeric?: boolean;
  /** Initial width hint. */
  width?: string;
  /** Render with monospace font. */
  mono?: boolean;
}

/** Sort a copy of rows in place by `key` and direction. */
function sortRows<T>(rows: T[], key: string, dir: "asc" | "desc", col: DataColumn<T>) {
  const valueOf = col.sortValue ?? ((r: T) => (r as Record<string, unknown>)[key] as string | number);
  const sorted = [...rows].sort((a, b) => {
    const va = valueOf(a);
    const vb = valueOf(b);
    if (typeof va === "number" && typeof vb === "number") return va - vb;
    return String(va ?? "").localeCompare(String(vb ?? ""));
  });
  return dir === "asc" ? sorted : sorted.reverse();
}

export function DataTable<T>({
  rows,
  columns,
  initialSort,
  pageSize,
  rowKey,
  emptyState,
  bundle,
  onRowClick,
  selectedRowKey,
}: {
  rows: ReadonlyArray<T>;
  columns: ReadonlyArray<DataColumn<T>>;
  initialSort?: { key: string; direction: "asc" | "desc" };
  pageSize?: number;
  rowKey: (row: T, index: number) => string;
  emptyState?: ReactNode;
  bundle?: import("../../i18n/messages").Messages;
  /**
   * Optional row-click handler. When provided, every row becomes a
   * clickable surface that calls back with the row and its global index;
   * the page can use this to drive the contextual Inspector via the
   * prefs state (selectObject).
   */
  onRowClick?: (row: T, index: number) => void;
  /**
   * The row key currently highlighted by the Inspector. The matching
   * <tr> receives aria-selected="true" so screen readers and styling can
   * reflect the selection state without the page needing to re-render.
   */
  selectedRowKey?: string | null;
}) {
  const [sort, setSort] = useState<{ key: string; direction: "asc" | "desc" } | null>(
    initialSort ?? null,
  );
  const [page, setPage] = useState(0);

  const sorted = useMemo(() => {
    if (!sort) return rows;
    const col = columns.find((c) => c.key === sort.key);
    if (!col) return rows;
    return sortRows([...rows], sort.key, sort.direction, col);
  }, [rows, sort, columns]);

  const totalPages = pageSize ? Math.max(1, Math.ceil(sorted.length / pageSize)) : 1;
  const pageStart = pageSize ? page * pageSize : 0;
  const pageEnd = pageSize ? Math.min(sorted.length, pageStart + pageSize) : sorted.length;
  const visible = pageSize ? sorted.slice(pageStart, pageEnd) : sorted;

  useEffect(() => {
    if (page > totalPages - 1) setPage(0);
  }, [page, totalPages]);

  function toggleSort(key: string, sortable: boolean | undefined) {
    if (sortable === false) return;
    setSort((s) => {
      if (!s || s.key !== key) return { key, direction: "asc" };
      if (s.direction === "asc") return { key, direction: "desc" };
      return null;
    });
  }

  return (
    <div className="v1-table-wrap">
      <div className="v1-table-scroll" role="region" aria-label="Data table">
          <table className="v1-table">
            <thead>
              <tr>
                {columns.map((c) => {
                  const isSorted = sort?.key === c.key;
                  return (
                    <th
                      key={c.key}
                      aria-sort={
                        isSorted
                          ? sort?.direction === "asc"
                            ? "ascending"
                            : "descending"
                          : "none"
                      }
                      onClick={() => toggleSort(c.key, c.sortable)}
                      style={{ width: c.width, cursor: c.sortable === false ? "default" : "pointer" }}
                      className={c.numeric ? "v1-num" : undefined}
                    >
                      {c.label}
                      {isSorted && (
                        <span aria-hidden="true">{sort?.direction === "asc" ? " ▲" : " ▼"}</span>
                      )}
                    </th>
                  );
                })}
              </tr>
            </thead>
            <tbody>
              {visible.length === 0 ? (
                <tr>
                  <td colSpan={columns.length} className="table-empty">
                    {emptyState ?? (bundle ? bundle.common.emptyRows : "No rows.")}
                  </td>
                </tr>
              ) : (
                visible.map((row, idx) => {
                  const k = rowKey(row, pageStart + idx);
                  const isSelected = selectedRowKey !== undefined && selectedRowKey === k;
                  const clickable = onRowClick !== undefined;
                  return (
                    <tr
                      key={k}
                      className={isSelected ? "v1-selected" : undefined}
                      aria-selected={isSelected ? "true" : undefined}
                      onClick={clickable ? () => onRowClick!(row, pageStart + idx) : undefined}
                      onKeyDown={
                        clickable
                          ? (e) => {
                              if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                onRowClick!(row, pageStart + idx);
                              }
                            }
                          : undefined
                      }
                      tabIndex={clickable ? 0 : undefined}
                      role={clickable ? "button" : undefined}
                    >
                      {columns.map((c) => {
                        const cls = c.numeric ? "v1-num" : c.mono ? "v1-mono" : undefined;
                        return (
                          <td key={c.key} className={cls}>
                            {c.render
                              ? c.render(row, pageStart + idx)
                              : String((row as Record<string, unknown>)[c.key] ?? "—")}
                          </td>
                        );
                      })}
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      {pageSize && totalPages > 1 && (
        <div className="v1-table-pagination" role="navigation" aria-label="Pagination">
          <button
            type="button"
            className="v1-btn"
            disabled={page === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}
          >
            ‹
          </button>
          <span className="v1-subtle v1-fs-sm">
            {page + 1} / {totalPages} · {sorted.length} rows
          </span>
          <button
            type="button"
            className="v1-btn"
            disabled={page >= totalPages - 1}
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
          >
            ›
          </button>
        </div>
      )}
    </div>
  );
}

export function KeyValueList({
  items,
}: {
  items: Array<{ key: string; value: ReactNode; mono?: boolean }>;
}) {
  return (
    <dl className="v1-kv">
      {items.map((item) => (
        <div key={item.key} style={{ display: "contents" }}>
          <dt>{item.key}</dt>
          <dd className={item.mono ? "v1-mono" : undefined}>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function FilterBar({
  placeholder,
  value,
  onChange,
  right,
}: {
  placeholder?: string;
  value: string;
  onChange: (next: string) => void;
  right?: ReactNode;
}) {
  // The FilterBar lives inside AppShell's PrefsContext, so we can pull
  // the active bundle directly instead of threading it through every
  // callsite. This keeps the configurators small and ensures the
  // placeholder/aria-label always reflect the current locale.
  const { state } = useContext(PrefsContext)!;
  const bundle = messages[state.locale];
  const filterLabel = bundle.common.filter;
  return (
    <div className="v1-page-toolbar">
      <input
        type="search"
        className="v1-input"
        value={value}
        placeholder={placeholder ?? filterLabel}
        onChange={(event) => onChange(event.target.value)}
        aria-label={placeholder ?? filterLabel}
      />
      <div className="toolbar-tools">{right}</div>
    </div>
  );
}