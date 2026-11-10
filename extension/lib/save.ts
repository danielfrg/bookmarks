import {
  CloudflareLoginError,
  TokenError,
  TooLargeError,
  UnreachableError,
  type client,
} from "./api.ts";
import type { Capturability } from "./capturability.ts";
import type { SaveErrorKind, SaveEvent, SaveRequest } from "./messages.ts";

export type { SaveErrorKind, SaveEvent, SaveRequest, SaveStage } from "./messages.ts";

export interface SaveDeps {
  api: ReturnType<typeof client>;
  /** Injects the capture script and collects the HTML; rejects on error or timeout. */
  capture(tabId: number): Promise<{ html: string; title: string }>;
  canCapture(url: string, tabId: number): Promise<Capturability>;
  /** To the popup port if connected; may throw when it is gone (ignored). */
  emit(e: SaveEvent): void;
  badge(state: "busy" | "ok" | "error"): void;
}

/** Letters, digits, space, '-' and '_' only (Unicode letters allowed), max 80 chars, plus ".html". */
export function archiveFilename(title: string): string {
  const clean = Array.from(title.replace(/[^\p{L}\p{N} _-]/gu, "").trim())
    .slice(0, 80)
    .join("")
    .trim();
  return `${clean || "page"}.html`;
}

function kindOf(err: unknown): SaveErrorKind | null {
  if (err instanceof TokenError) return "token";
  if (err instanceof CloudflareLoginError) return "cloudflare";
  if (err instanceof UnreachableError) return "unreachable";
  if (err instanceof TooLargeError) return "too-large";
  return null;
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export async function runSave(req: SaveRequest, deps: SaveDeps): Promise<SaveEvent> {
  // A gone popup port must never affect the save.
  const emit = (e: SaveEvent) => {
    try {
      deps.emit(e);
    } catch {
      /* Popup closed. */
    }
  };
  const fail = (event: SaveEvent): SaveEvent => {
    deps.badge("error");
    emit(event);
    return event;
  };

  deps.badge("busy");
  let capturable = false;
  if (req.archive) {
    try {
      capturable = (await deps.canCapture(req.url, req.tabId)).ok;
    } catch {
      capturable = false;
    }
  }

  emit({ stage: "saving" });
  let bookmarkId: number;
  try {
    const { bookmark } = await deps.api.save(
      {
        url: req.url,
        title: req.title,
        description: req.description,
        notes: req.notes,
        tags: req.tags,
        unread: req.unread,
      },
      capturable ? "none" : "server",
    );
    bookmarkId = bookmark.id;
  } catch (err) {
    const kind = kindOf(err) ?? "other";
    const message = kind === "too-large" ? "Page too large to archive" : messageOf(err);
    return fail({ stage: "error", error: { kind, message }, message });
  }

  if (capturable) {
    try {
      emit({ stage: "capturing", bookmarkId });
      // Build the Blob right away so the HTML string can be collected.
      const blob = new Blob([(await deps.capture(req.tabId)).html], { type: "text/html" });
      emit({ stage: "uploading", bookmarkId });
      await deps.api.uploadArchive(bookmarkId, blob, archiveFilename(req.title));
    } catch (err) {
      // The bookmark stays; "Capture again on server" in the web UI covers it.
      // The bookmark exists, so only "too large" keeps its own kind.
      const tooLarge = err instanceof TooLargeError;
      const kind: SaveErrorKind = tooLarge ? "too-large" : "capture";
      const message = tooLarge
        ? "Page too large to archive"
        : `Saved, but the page couldn't be archived: ${messageOf(err)}`;
      return fail({ stage: "error", bookmarkId, error: { kind, message }, message });
    }
  }

  const done: SaveEvent = { stage: "done", bookmarkId };
  deps.badge("ok");
  emit(done);
  return done;
}
