export type Shortcut = "new" | "search" | "next" | "prev" | "open" | "view" | "edit" | "help";

const KEYS: Record<string, Shortcut> = {
  n: "new",
  s: "search",
  "/": "search",
  j: "next",
  ArrowDown: "next",
  k: "prev",
  ArrowUp: "prev",
  Enter: "open",
  o: "open",
  v: "view",
  e: "edit",
  "?": "help",
};

type KeyEvent = Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "target"> & {
  shiftKey?: boolean;
};

/** The shortcut a keydown triggers, or null when it should be left alone. */
export function shortcutFor(e: KeyEvent): Shortcut | null {
  if (e.metaKey || e.ctrlKey || e.altKey) return null;
  if (e.shiftKey && e.key !== "?") return null;
  const t = e.target as HTMLElement | null;
  if (t) {
    if (/^(INPUT|TEXTAREA|SELECT)$/i.test(t.tagName) || t.isContentEditable) return null;
    if (t.closest?.("[role=dialog]")) return null;
  }
  return Object.hasOwn(KEYS, e.key) ? KEYS[e.key] : null;
}
