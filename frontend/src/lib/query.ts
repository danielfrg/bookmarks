import type { Bookmark, Tag } from "./api.ts";

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
