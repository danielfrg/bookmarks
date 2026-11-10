// Injected on demand into the top frame (ISOLATED world) by the background:
//   1. executeScript({ func: (id) => { window.__bookmarksCaptureId = id }, args: [id] })
//   2. executeScript({ files: ["capture.js"] })
// Captures the page with SingleFile and streams the HTML back over a runtime
// Port named "capture:<id>" (see lib/messages.ts). SingleFile's lazy loader
// runs its timers through the background (lib/lazy-timeout.ts).
import { getPageData, init } from "single-file-core/single-file.js";
import { STAGE_DEFERRED, StageTracker, type SingleFileProgress } from "../lib/capture-stage.ts";
import { splitString } from "../lib/chunks.ts";
import { capturePortName, type CaptureMsg, type RelayFetchResponse } from "../lib/messages.ts";
import { createResourceFetch } from "../lib/relay.ts";

const SINGLEFILE_OPTIONS = {
  removeHiddenElements: true,
  removeUnusedStyles: true,
  removeUnusedFonts: true,
  compressHTML: true,
  loadDeferredContent: true,
  loadDeferredContentMaxIdleTime: 1500,
  blockScripts: true,
  blockVideos: true,
  removeAlternativeFonts: true,
  removeAlternativeMedias: true,
  removeAlternativeImages: true,
  groupDuplicateImages: true,
  removeFrames: true,
  maxResourceSizeEnabled: true,
  maxResourceSize: 10,
  saveOriginalURLs: false,
};

// Without traffic the service worker idles out after 30 s; a long capture with
// no relayed resources would otherwise lose its port.
const KEEPALIVE_MS = 10_000;

type CaptureWindow = Window & {
  __bookmarksCaptureId?: string;
  __bookmarksCaptureRun?: (id: string) => Promise<void>;
};

export default defineUnlistedScript(() => {
  const w = window as CaptureWindow;
  const id = w.__bookmarksCaptureId;
  delete w.__bookmarksCaptureId;
  // A second capture on the same page reuses the first injection's run().
  w.__bookmarksCaptureRun ??= run;
  if (id) void w.__bookmarksCaptureRun(id);
});

let initialized = false;

function initSingleFile(): void {
  if (initialized) return;
  const resourceFetch = createResourceFetch({
    fetch: (url, init) => fetch(url, init),
    relay: (req) => browser.runtime.sendMessage(req) as Promise<RelayFetchResponse | undefined>,
  });
  init({ fetch: resourceFetch, frameFetch: resourceFetch });
  initialized = true;
}

async function run(id: string): Promise<void> {
  let connected = true;
  let port: Browser.runtime.Port;
  try {
    port = browser.runtime.connect({ name: capturePortName(id) });
  } catch {
    return; // extension reloaded or context invalidated: nobody is listening
  }
  port.onDisconnect.addListener(() => {
    connected = false;
  });
  const post = (msg: CaptureMsg): boolean => {
    if (!connected) return false;
    try {
      port.postMessage(msg);
      return true;
    } catch {
      connected = false;
      return false;
    }
  };

  const keepalive = setInterval(() => post({ type: "keepalive" }), KEEPALIVE_MS);
  const tracker = new StageTracker();
  let stage = "starting SingleFile";
  const progress = (next: string) => {
    if (next === stage) return;
    stage = next;
    post({ type: "progress", stage });
  };
  post({ type: "progress", stage });
  try {
    initSingleFile();
    // getPageData loads lazy content before its first progress event.
    progress(STAGE_DEFERRED);
    const pageData = await getPageData(
      {
        ...SINGLEFILE_OPTIONS,
        onprogress: (event: SingleFileProgress) => {
          const next = tracker.update(event);
          if (next) progress(next);
        },
      },
      {},
      document,
      window,
    );
    clearInterval(keepalive);
    progress("sending the archive");
    const parts = splitString(pageData.content);
    for (const [index, data] of parts.entries()) {
      if (!post({ type: "chunk", index, total: parts.length, data })) return;
    }
    post({ type: "done", title: pageData.title || document.title });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    post({ type: "error", message: `${message} (while ${stage})` });
  } finally {
    clearInterval(keepalive);
  }
}
