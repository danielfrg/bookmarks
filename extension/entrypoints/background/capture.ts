import { STAGE_INJECTING, STAGE_STARTING, captureTimeoutMessage } from "../../lib/capture-stage.ts";
import { ChunkAssembler } from "../../lib/chunks.ts";
import { LAZY_FIRE, LazyTimeouts, type LazyTimeoutMessage } from "../../lib/lazy-timeout.ts";
import { capturePortName } from "../../lib/messages.ts";
import type { CaptureMsg } from "../../lib/messages.ts";

export const CAPTURE_TIMEOUT_MS = 120_000;

/** tabId → capture id of the capture in flight. */
const active = new Map<number, string>();

/** True if `name` is the port of a capture in flight for the sender's tab. */
export function isExpectedCapturePort(name: string, tabId: number | undefined): boolean {
  return (
    tabId !== undefined &&
    active.get(tabId) !== undefined &&
    name === capturePortName(active.get(tabId)!)
  );
}

export function hasActiveCapture(tabId: number): boolean {
  return active.has(tabId);
}

const lazyTimeouts = new LazyTimeouts((tabId, frameId, type) => {
  browser.tabs.sendMessage(tabId, { method: LAZY_FIRE, type }, { frameId }).catch(() => {
    /* Tab closed or navigated: the capture fails on its own. */
  });
});

/** SingleFile lazy-loader timers (lib/lazy-timeout.ts), only for a capture in flight. */
export function handleLazyTimeout(tabId: number, frameId: number, msg: LazyTimeoutMessage): void {
  if (active.has(tabId)) lazyTimeouts.handle(tabId, frameId, msg);
}

/**
 * Injects capture.js into the tab and collects the HTML it streams back.
 * Rejects on timeout, port disconnect before `done`, a script error, a chunk
 * protocol violation or an executeScript failure. If the id handshake is
 * missing the script stays silent and the timeout fires. Timeout and disconnect
 * errors name the last stage the script reported.
 */
export function captureTab(tabId: number): Promise<{ html: string; title: string }> {
  if (active.has(tabId)) {
    return Promise.reject(new Error("A capture is already running for this tab"));
  }
  const id = crypto.randomUUID();
  active.set(tabId, id);
  const portName = capturePortName(id);

  return new Promise((resolve, reject) => {
    const assembler = new ChunkAssembler();
    let port: chrome.runtime.Port | undefined;
    let settled = false;
    const startedAt = Date.now();
    let stage = STAGE_INJECTING;
    let progressAt = startedAt;
    const advance = (next: string) => {
      stage = next;
      progressAt = Date.now();
    };

    const finish = (err: Error | null, value?: { html: string; title: string }) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      browser.runtime.onConnect.removeListener(onConnect);
      if (active.get(tabId) === id) {
        active.delete(tabId);
        lazyTimeouts.clearTab(tabId);
      }
      try {
        port?.disconnect();
      } catch {
        /* Already gone. */
      }
      if (err) reject(err);
      else resolve(value!);
    };

    const timer = setTimeout(() => {
      const now = Date.now();
      const message = captureTimeoutMessage({
        stage,
        elapsedMs: now - startedAt,
        sinceProgressMs: now - progressAt,
      });
      finish(new Error(message));
    }, CAPTURE_TIMEOUT_MS);

    const onConnect = (p: chrome.runtime.Port) => {
      // Only the capture script of the tab we injected into, once.
      if (p.name !== portName || port || p.sender?.tab?.id !== tabId) return;
      port = p;
      advance("starting SingleFile");
      p.onMessage.addListener((m: CaptureMsg) => {
        try {
          if (m.type === "progress") advance(m.stage);
          else if (m.type === "chunk") {
            assembler.add(m.index, m.total, m.data);
            advance(`receiving the archive (${m.index + 1}/${m.total})`);
          } else if (m.type === "done") finish(null, { html: assembler.finish(), title: m.title });
          else if (m.type === "error") finish(new Error(m.message));
        } catch (err) {
          finish(err instanceof Error ? err : new Error(String(err)));
        }
      });
      p.onDisconnect.addListener(() =>
        finish(new Error(`the page closed or navigated while ${stage}`)),
      );
    };
    browser.runtime.onConnect.addListener(onConnect);

    browser.scripting
      .executeScript({
        target: { tabId },
        func: (captureId: string) => {
          (window as unknown as { __bookmarksCaptureId?: string }).__bookmarksCaptureId = captureId;
        },
        args: [id],
      })
      .then(() => browser.scripting.executeScript({ target: { tabId }, files: ["/capture.js"] }))
      .then(() => {
        if (!port) advance(STAGE_STARTING);
      })
      .catch((err: unknown) =>
        finish(new Error(`could not read the page: ${err instanceof Error ? err.message : err}`)),
      );
  });
}
