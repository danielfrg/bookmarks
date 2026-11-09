import type { Bookmark, BulkAction, BulkRequest, Tag } from "./api.ts";
import type { View } from "./view.ts";

/** Adds "#tag" to the query, or removes every "#tag" token (case-insensitive). */
export function toggleTagInQuery(q: string, tag: string): string {
  const token = `#${tag.replace(/^#/, "")}`;
  const tokens = q.split(/\s+/).filter(Boolean);
  const kept = tokens.filter((t) => t.toLowerCase() !== token.toLowerCase());
  if (kept.length === tokens.length) kept.push(token);
  return kept.join(" ");
}

export function displayTitle(b: Pick<Bookmark, "title" | "url">): string {
  return b.title.trim() || b.url;
}

export function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return "";
  }
}

/** Tags grouped by first letter (A-Z); anything else goes to a trailing "#" group. */
export function groupTags(tags: Tag[]): { letter: string; tags: Tag[] }[] {
  const sorted = [...tags].sort((a, b) =>
    a.name.localeCompare(b.name, "en", { sensitivity: "base" }),
  );
  const groups = new Map<string, Tag[]>();
  for (const tag of sorted) {
    const first = tag.name.normalize("NFD").charAt(0).toUpperCase();
    const letter = /^[A-Z]$/.test(first) ? first : "#";
    groups.set(letter, [...(groups.get(letter) ?? []), tag]);
  }
  const letters = [...groups.keys()].sort((a, b) =>
    a === "#" ? 1 : b === "#" ? -1 : a.localeCompare(b),
  );
  return letters.map((letter) => {
    const list = groups.get(letter)!;
    // Digits and symbols: plain code-point order, not locale order.
    if (letter === "#") list.sort((a, b) => (a.name.toLowerCase() < b.name.toLowerCase() ? -1 : 1));
    return { letter, tags: list };
  });
}

/** Lower-cased tag names referenced as "#tag" tokens in the query. */
export function activeTagsOf(q: string): string[] {
  return q
    .split(/\s+/)
    .filter((t) => t.startsWith("#") && t.length > 1)
    .map((t) => t.slice(1).toLowerCase());
}

/**
 * The search box text after the committed query changes. Typing that has not
 * been committed yet (or differs only by surrounding whitespace) is kept.
 */
export function syncSearchText(prevQ: string, nextQ: string, text: string): string {
  if (nextQ === prevQ) return text;
  return nextQ === text.trim() ? text : nextQ;
}

/** "0 B", "512 B", "1.0 KB", "2.4 MB", "1.1 GB" (base 1024). */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 1024) return `${Math.max(0, Math.round(n || 0))} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (i < units.length - 1 && Number(v.toFixed(1)) >= 1024) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

/**
 * Commits typed tag text: splits on whitespace/commas, lower-cases, drops a
 * leading "#" and empty parts, and appends tags not already present.
 */
export function commitTagText(current: string[], text: string): string[] {
  const result = [...current];
  for (const raw of text.split(/[\s,]+/)) {
    const tag = raw.replace(/^#+/, "").toLowerCase();
    if (tag && !result.includes(tag)) result.push(tag);
  }
  return result;
}

/**
 * Builds the bulk API request for a selection. "All matching" uses the current
 * search (q + scope + unread). Returns null when that would be unsafe: a
 * match-all delete (the server rejects it) or any all-matching inside a bundle
 * (the bulk API has no bundle parameter).
 */
export function bulkRequest(
  sel: { ids: number[]; allMatching: boolean },
  v: View,
  action: BulkAction,
  tags?: string[],
): BulkRequest | null {
  const req: BulkRequest = { action };
  if (sel.allMatching) {
    if (v.bundle !== null) return null;
    if (action === "delete" && !v.q.trim() && v.filter !== "unread") return null;
    req.query = {
      q: v.q,
      scope: v.filter === "archived" ? "archived" : "active",
      ...(v.filter === "unread" ? { unread: "yes" } : {}),
    };
  } else {
    req.ids = sel.ids;
  }
  if (tags) req.tags = tags;
  // Stable key order: selection first.
  return req.ids ? { ids: req.ids, action, ...(tags ? { tags } : {}) } : req;
}
