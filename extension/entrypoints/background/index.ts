import { client } from "../../lib/api.ts";
import { capturability } from "../../lib/capturability.ts";
import {
  SAVE_PORT,
  captureIdFromPort,
  isRelayFetchRequest,
  isSavePortIn,
  type RelayFetchResponse,
  type SaveEvent,
  type SaveRequest,
  type SavePortOut,
} from "../../lib/messages.ts";
import { isLazyTimeoutMessage } from "../../lib/lazy-timeout.ts";
import { relayFetch } from "../../lib/relay.ts";
import { runSave } from "../../lib/save.ts";
import { loadSettings } from "../../lib/settings.ts";
import { setBadge } from "./badge.ts";
import {
  captureTab,
  handleLazyTimeout,
  hasActiveCapture,
  isExpectedCapturePort,
} from "./capture.ts";

const KEEPALIVE_MS = 20_000;
const TERMINAL_EVENT_TTL_MS = 30_000;

/** Latest SaveEvent per tab, so a reopened popup can show a save in flight. */
const latest = new Map<number, SaveEvent>();
const running = new Set<number>();
/** Popup ports listening to a tab's events. */
const listeners = new Map<number, Set<chrome.runtime.Port>>();

let keepaliveTimer: ReturnType<typeof setInterval> | undefined;

function startKeepalive() {
  keepaliveTimer ??= setInterval(() => void browser.runtime.getPlatformInfo(), KEEPALIVE_MS);
}

function stopKeepaliveIfIdle() {
  if (running.size === 0 && keepaliveTimer) {
    clearInterval(keepaliveTimer);
    keepaliveTimer = undefined;
  }
}

function post(port: chrome.runtime.Port, msg: SavePortOut) {
  try {
    port.postMessage(msg);
  } catch {
    /* Popup closed; the save carries on. */
  }
}

function emit(tabId: number, event: SaveEvent) {
  latest.set(tabId, event);
  for (const port of listeners.get(tabId) ?? []) post(port, { type: "event", tabId, event });
  if (event.stage === "done" || event.stage === "error") {
    setTimeout(() => {
      if (latest.get(tabId) === event) latest.delete(tabId);
    }, TERMINAL_EVENT_TTL_MS);
  }
}

function listen(tabId: number, port: chrome.runtime.Port) {
  let set = listeners.get(tabId);
  if (!set) listeners.set(tabId, (set = new Set()));
  set.add(port);
}

async function startSave(request: SaveRequest, port: chrome.runtime.Port) {
  const { tabId } = request;
  if (running.has(tabId)) {
    // Only the requester hears this; the healthy save's state is untouched.
    const message = "A save is already running for this tab";
    post(port, {
      type: "event",
      tabId,
      event: { stage: "error", error: { kind: "other", message }, message },
    });
    return;
  }
  running.add(tabId);
  startKeepalive();
  try {
    const settings = await loadSettings();
    await runSave(request, {
      api: client(settings),
      capture: captureTab,
      canCapture: async (url) =>
        capturability(url, {
          fileAccess: await browser.extension.isAllowedFileSchemeAccess(),
        }),
      emit: (e) => emit(tabId, e),
      badge: (state) => setBadge(tabId, state),
    });
  } catch (err) {
    // runSave maps its own errors; this is a last resort (e.g. settings unreadable).
    const message = err instanceof Error ? err.message : String(err);
    setBadge(tabId, "error");
    emit(tabId, { stage: "error", error: { kind: "other", message }, message });
  } finally {
    running.delete(tabId);
    stopKeepaliveIfIdle();
  }
}

export default defineBackground({
  type: "module",
  main() {
    // A worker that died mid-save leaves "…" badges behind.
    void browser.action.setBadgeText({ text: "" }).catch(() => {});

    // Popup protocol: connect "save", post one SaveRequest (or {status}).
    // The save runs independently of the port.
    browser.runtime.onConnect.addListener((port) => {
      // A stale capture script (timed out, or from an earlier worker) exits.
      if (
        captureIdFromPort(port.name) !== null &&
        !isExpectedCapturePort(port.name, port.sender?.tab?.id)
      ) {
        port.disconnect();
        return;
      }
      if (port.name !== SAVE_PORT || port.sender?.id !== browser.runtime.id || port.sender.tab) {
        return;
      }
      const mine = new Set<number>();
      port.onMessage.addListener((m: unknown) => {
        if (!isSavePortIn(m)) return;
        if (m.type === "status") {
          listen(m.tabId, port);
          mine.add(m.tabId);
          post(port, { type: "event", tabId: m.tabId, event: latest.get(m.tabId) ?? null });
          return;
        }
        listen(m.request.tabId, port);
        mine.add(m.request.tabId);
        void startSave(m.request, port);
      });
      port.onDisconnect.addListener(() => {
        for (const tabId of mine) {
          const set = listeners.get(tabId);
          set?.delete(port);
          if (set?.size === 0) listeners.delete(tabId);
        }
      });
    });

    // SingleFile's lazy loader runs its timers here and waits for the reply;
    // without it a capture stalls before it starts (lib/lazy-timeout.ts).
    browser.runtime.onMessage.addListener((message, sender, sendResponse) => {
      if (
        !isLazyTimeoutMessage(message) ||
        sender.id !== browser.runtime.id ||
        sender.tab?.id === undefined ||
        sender.frameId === undefined
      ) {
        return false;
      }
      handleLazyTimeout(sender.tab.id, sender.frameId, message);
      sendResponse({});
      return false;
    });

    // Resource relay for the capture script: fetches what the page's CORS
    // rules block, using the extension's <all_urls> host permission.
    browser.runtime.onMessage.addListener((message, sender, sendResponse) => {
      if (
        !isRelayFetchRequest(message) ||
        sender.id !== browser.runtime.id ||
        sender.tab?.id === undefined ||
        sender.frameId !== 0 || // capture runs in the top frame only
        !hasActiveCapture(sender.tab.id) // only while we are capturing that tab
      ) {
        return false;
      }
      // relayFetch keeps public pages away from loopback/LAN hosts (homelab
      // admin UIs), including via redirects; see lib/relay.ts.
      const senderURL = sender.url ?? sender.tab.url;
      relayFetch(message, (url, init) => fetch(url, init), { senderURL }).then(
        (res) => sendResponse(res),
        (error: unknown) =>
          sendResponse({ ok: false, error: String(error) } satisfies RelayFetchResponse),
      );
      return true; // keep sendResponse alive for the async reply
    });
  },
});
