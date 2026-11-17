import type { TableSort } from "./context.svelte";

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

function compare(a: unknown, b: unknown): number {
  if (a == null && b == null) return 0;
  if (a == null) return 1;
  if (b == null) return -1;
  if (a instanceof Date && b instanceof Date) return a.getTime() - b.getTime();
  if (typeof a === "number" && typeof b === "number") return a - b;
  return collator.compare(String(a), String(b));
}

// Returns a sorted copy. Empty values sort last in both directions.
export function sortRows<T>(
  rows: readonly T[],
  sort: TableSort | null,
  value: (row: T, key: string) => unknown = (row, key) => (row as Record<string, unknown>)[key],
): T[] {
  if (!sort) return [...rows];
  const sign = sort.direction === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => {
    const x = value(a, sort.key);
    const y = value(b, sort.key);
    if (x == null || y == null) return compare(x, y);
    return sign * compare(x, y);
  });
}
