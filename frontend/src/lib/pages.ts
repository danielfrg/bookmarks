import type { Bundle, ImportResult, Settings, Tag } from "./api.ts";

export const DEFAULT_ITEMS_PER_PAGE = 30;
export const MIN_ITEMS_PER_PAGE = 10;
export const MAX_ITEMS_PER_PAGE = 100;

/** Items per page from user input: rounded, clamped to 10-100, default 30 when not a number. */
export function clampItemsPerPage(value: number | string): number {
  const n = typeof value === "number" ? value : Number(String(value).trim());
  if (String(value).trim() === "" || !Number.isFinite(n)) return DEFAULT_ITEMS_PER_PAGE;
  return Math.min(MAX_ITEMS_PER_PAGE, Math.max(MIN_ITEMS_PER_PAGE, Math.round(n)));
}

/** A copy of `ids` with the item at `index` moved one step up (-1) or down (1); unchanged at the ends. */
export function moveItem(ids: number[], index: number, dir: -1 | 1): number[] {
  const to = index + dir;
  const next = [...ids];
  if (index < 0 || index >= ids.length || to < 0 || to >= ids.length) return next;
  [next[index], next[to]] = [next[to], next[index]];
  return next;
}

/** Tag names are stored lower-case without "#". */
export function normalizeTagName(name: string): string {
  return name.trim().replace(/^#+/, "").toLowerCase();
}

/** The other tag that renaming tag `id` to `newName` would merge into, if any (case-insensitive). */
export function wouldMerge(tags: Tag[], id: number, newName: string): Tag | null {
  const target = normalizeTagName(newName);
  if (!target) return null;
  return tags.find((t) => t.id !== id && t.name.toLowerCase() === target) ?? null;
}

export function filterTags(tags: Tag[], text: string): Tag[] {
  const needle = text.trim().replace(/^#+/, "").toLowerCase();
  return needle ? tags.filter((t) => t.name.toLowerCase().includes(needle)) : tags;
}

export function byOrder(bundles: Bundle[]): Bundle[] {
  return [...bundles].sort((a, b) => a.order - b.order || a.id - b.id);
}

export function importSummary(r: ImportResult): string {
  return `Created ${r.created}, updated ${r.updated}, skipped ${r.skipped}`;
}

export type SettingsPatch = Partial<Omit<Settings, "ui">> & { ui?: Partial<Settings["ui"]> };

/** `base` with `patch` applied; `ui` keys merge so other ui settings are preserved. */
export function mergeSettings(base: Settings, patch: SettingsPatch): Settings {
  const { ui, ...rest } = patch;
  return { ...base, ...rest, ui: { ...base.ui, ...ui } };
}
