const timers = new Map<number, ReturnType<typeof setTimeout>>();

const STYLES = {
  busy: { text: "…", color: "#6b7280", clearMs: 0 },
  ok: { text: "✓", color: "#16a34a", clearMs: 4_000 },
  error: { text: "!", color: "#dc2626", clearMs: 10_000 },
} as const;

/** Per-tab action badge: busy stays, ok clears after 4 s, error after 10 s. */
export function setBadge(tabId: number, state: keyof typeof STYLES): void {
  const old = timers.get(tabId);
  if (old) clearTimeout(old);
  timers.delete(tabId);
  const s = STYLES[state];
  const swallow = () => {}; // tab may be gone
  void browser.action.setBadgeText({ tabId, text: s.text }).catch(swallow);
  void browser.action.setBadgeBackgroundColor({ tabId, color: s.color }).catch(swallow);
  if (s.clearMs > 0) {
    timers.set(
      tabId,
      setTimeout(() => {
        timers.delete(tabId);
        void browser.action.setBadgeText({ tabId, text: "" }).catch(swallow);
      }, s.clearMs),
    );
  }
}
