/** capture script → background (runtime.sendMessage): fetch a resource the page cannot (CORS). */
export type RelayFetchRequest = { type: "relay-fetch"; url: string };

export type RelayFetchResponse =
  | { ok: true; status: number; url: string; contentType: string; base64: string }
  | { ok: false; error: string };

/** capture script → background over a runtime Port named `capture:<captureId>`. */
export type CaptureMsg =
  /** `stage` is human-readable (lib/capture-stage.ts); shown when a capture fails. */
  | { type: "progress"; stage: string }
  /** Keeps the service worker alive; not progress. */
  | { type: "keepalive" }
  | { type: "chunk"; index: number; total: number; data: string }
  | { type: "done"; title: string }
  | { type: "error"; message: string };

export const CAPTURE_PORT_PREFIX = "capture:";

export function capturePortName(captureId: string): string {
  return CAPTURE_PORT_PREFIX + captureId;
}

/** The capture id from a port name, or null if the port is not a capture port. */
export function captureIdFromPort(name: string): string | null {
  return name.startsWith(CAPTURE_PORT_PREFIX) && name.length > CAPTURE_PORT_PREFIX.length
    ? name.slice(CAPTURE_PORT_PREFIX.length)
    : null;
}

export function isRelayFetchRequest(m: unknown): m is RelayFetchRequest {
  if (typeof m !== "object" || m === null) return false;
  const r = m as Record<string, unknown>;
  return r.type === "relay-fetch" && typeof r.url === "string";
}

/** Max bytes the background relays for one resource (SingleFile maxResourceSize: 10 MB). */
export const RELAY_MAX_BYTES = 10 * 1024 * 1024;

/** popup → background: what to save. */
export type SaveRequest = {
  tabId: number;
  url: string;
  title: string;
  description: string;
  notes: string;
  tags: string[];
  unread: boolean;
  archive: boolean;
};

export type SaveStage = "saving" | "capturing" | "uploading" | "done" | "error";

export type SaveErrorKind =
  | "token"
  | "cloudflare"
  | "unreachable"
  | "too-large"
  | "capture"
  | "other";

export type SaveEvent = {
  stage: SaveStage;
  bookmarkId?: number;
  message?: string;
  error?: { kind: SaveErrorKind; message: string };
};

/** The popup connects a runtime Port with this name. */
export const SAVE_PORT = "save";

/** popup → background over the "save" port. */
export type SavePortIn = { type: "save"; request: SaveRequest } | { type: "status"; tabId: number };

/** background → popup over the "save" port. `event` is null when nothing is known for the tab. */
export type SavePortOut = { type: "event"; tabId: number; event: SaveEvent | null };

export function isSavePortIn(m: unknown): m is SavePortIn {
  if (typeof m !== "object" || m === null) return false;
  const r = m as Record<string, unknown>;
  if (r.type === "status") return typeof r.tabId === "number";
  if (r.type !== "save" || typeof r.request !== "object" || r.request === null) return false;
  const q = r.request as Record<string, unknown>;
  return (
    typeof q.tabId === "number" &&
    typeof q.url === "string" &&
    typeof q.title === "string" &&
    typeof q.description === "string" &&
    typeof q.notes === "string" &&
    Array.isArray(q.tags) &&
    q.tags.every((t) => typeof t === "string") &&
    typeof q.unread === "boolean" &&
    typeof q.archive === "boolean"
  );
}
