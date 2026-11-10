/**
 * single-file-core's lazy loader (processors/lazy/content/content-lazy-loader.js)
 * runs its idle timers through the extension background whenever the content
 * script can see `globalThis.browser`, as it can in Chrome: it sends
 * `singlefile.lazyTimeout.setTimeout` / `clearTimeout` and waits for the
 * background to send `singlefile.lazyTimeout.onTimeout` back to the frame.
 * Without that reply `loadDeferredContent` never finishes. SingleFile's own
 * extension does this because background timers are not throttled in hidden
 * tabs; this is the same protocol.
 */

export const LAZY_SET = "singlefile.lazyTimeout.setTimeout";
export const LAZY_CLEAR = "singlefile.lazyTimeout.clearTimeout";
export const LAZY_FIRE = "singlefile.lazyTimeout.onTimeout";

export type LazyTimeoutMessage =
  | { method: typeof LAZY_SET; type: string; delay: number }
  | { method: typeof LAZY_CLEAR; type: string };

export function isLazyTimeoutMessage(m: unknown): m is LazyTimeoutMessage {
  if (typeof m !== "object" || m === null) return false;
  const r = m as Record<string, unknown>;
  if (typeof r.type !== "string") return false;
  if (r.method === LAZY_CLEAR) return true;
  return r.method === LAZY_SET && typeof r.delay === "number" && Number.isFinite(r.delay);
}

type Timer = ReturnType<typeof setTimeout>;

/** Lazy-loader timers per tab, frame and timer type. */
export class LazyTimeouts {
  #timers = new Map<string, { tabId: number; timer: Timer }>();
  #fire: (tabId: number, frameId: number, type: string) => void;

  constructor(fire: (tabId: number, frameId: number, type: string) => void) {
    this.#fire = fire;
  }

  handle(tabId: number, frameId: number, msg: LazyTimeoutMessage): void {
    const key = `${tabId}:${frameId}:${msg.type}`;
    const previous = this.#timers.get(key);
    if (previous) clearTimeout(previous.timer);
    this.#timers.delete(key);
    if (msg.method === LAZY_CLEAR) return;
    const timer = setTimeout(
      () => {
        this.#timers.delete(key);
        this.#fire(tabId, frameId, msg.type);
      },
      Math.max(0, msg.delay),
    );
    this.#timers.set(key, { tabId, timer });
  }

  /** Drops every timer of a tab (its capture ended). */
  clearTab(tabId: number): void {
    for (const [key, { tabId: t, timer }] of this.#timers) {
      if (t !== tabId) continue;
      clearTimeout(timer);
      this.#timers.delete(key);
    }
  }

  get size(): number {
    return this.#timers.size;
  }
}
