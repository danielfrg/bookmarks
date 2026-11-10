import {
  CloudflareLoginError,
  TokenError,
  UnreachableError,
  type Bookmark,
  type CheckResult,
} from "./api.ts";
import type { Capturability, CapturabilityReason } from "./capturability.ts";
import type { SaveErrorKind, SaveEvent, SaveStage } from "./messages.ts";
import type { ExtSettings } from "./settings.ts";

export interface FormState {
  title: string;
  description: string;
  notes: string;
  tags: string[];
  unread: boolean;
}

export type FormNotice = { kind: "already-saved"; dateAdded: string } | { kind: "basic-capture" };

export interface InitialForm {
  form: FormState;
  existing: Bookmark | null;
  notices: FormNotice[];
  actions: SaveActions;
  /** Set when the page cannot be archived; there is no archive button. */
  archiveDisabledReason: string | null;
}

/**
 * The popup's buttons. The main one (Enter) saves the form; on a new page it
 * also archives (a failed archive still leaves the bookmark saved). A saved
 * bookmark gets a second button that saves and archives (again).
 */
export interface SaveActions {
  saveLabel: "Save" | "Update";
  /** The main button archives too. */
  saveArchives: boolean;
  /** The secondary archive button, or null for none. */
  archiveLabel: "Archive" | "Re-archive" | null;
}

const CAPABILITY_MESSAGES: Record<CapturabilityReason, string> = {
  "browser-page": "Chrome pages can't be archived",
  "web-store": "The Chrome Web Store can't be archived",
  pdf: "PDFs are archived by the server",
  "file-access": "Allow file access for this extension to archive local files",
  "unsupported-scheme": "This page can't be archived",
};

export function capabilityMessage(reason: CapturabilityReason): string {
  return CAPABILITY_MESSAGES[reason];
}

/** The form's starting values from settings, the check result and the tab. */
export function initialForm(input: {
  settings: Pick<ExtSettings, "unreadByDefault">;
  check: CheckResult;
  tabTitle: string | undefined;
  capture: Capturability;
}): InitialForm {
  const { settings, check, tabTitle, capture } = input;
  const existing = check.bookmark;
  const notices: FormNotice[] = [];
  if (existing) notices.push({ kind: "already-saved", dateAdded: existing.date_added });

  const capturable = capture.ok;
  const basicOnly = check.basic_capture_only && capturable;
  if (basicOnly) notices.push({ kind: "basic-capture" });

  const fullyArchived = existing !== null && existing.archive_count > 0 && !basicOnly;
  const actions: SaveActions = {
    saveLabel: existing ? "Update" : "Save",
    saveArchives: !existing && capturable,
    archiveLabel: !existing || !capturable ? null : fullyArchived ? "Re-archive" : "Archive",
  };
  const form: FormState = existing
    ? {
        title: existing.title,
        description: existing.description,
        notes: existing.notes,
        tags: [...existing.tags],
        unread: existing.unread,
      }
    : {
        title: tabTitle?.trim() ? tabTitle : check.metadata.title,
        description: check.metadata.description,
        notes: "",
        tags: [...check.auto_tags],
        unread: settings.unreadByDefault,
      };
  return {
    form,
    existing,
    notices,
    actions,
    archiveDisabledReason: capture.ok ? null : capabilityMessage(capture.reason),
  };
}

export interface CheckFailure {
  kind: "cloudflare" | "token" | "unreachable" | "other";
  title: string;
  /** Save would fail the same way, so the popup shows this instead of the form. */
  blocking: boolean;
}

export function checkFailure(err: unknown): CheckFailure {
  if (err instanceof CloudflareLoginError) {
    return { kind: "cloudflare", title: "Log in to Cloudflare", blocking: true };
  }
  if (err instanceof TokenError) return { kind: "token", title: "Token rejected", blocking: true };
  if (err instanceof UnreachableError) {
    return { kind: "unreachable", title: "Can't reach the server", blocking: true };
  }
  return {
    kind: "other",
    title: err instanceof Error ? err.message : String(err),
    blocking: false,
  };
}

export type StatusView =
  | { view: "form" }
  | { view: "progress"; stage: "saving" | "capturing" | "uploading" }
  | { view: "done" }
  | { view: "error"; error?: { kind: SaveErrorKind; message: string }; bookmarkId?: number };

/** What to show for the latest background event of this tab (null: nothing known). */
export function viewFromStatus(event: SaveEvent | null): StatusView {
  if (!event) return { view: "form" };
  if (event.stage === "done") return { view: "done" };
  if (event.stage === "error") {
    return { view: "error", error: event.error, bookmarkId: event.bookmarkId };
  }
  return { view: "progress", stage: event.stage };
}

export function progressText(stage: Exclude<SaveStage, "done" | "error">): string {
  switch (stage) {
    case "saving":
      return "Saving…";
    case "capturing":
      return "Archiving page…";
    case "uploading":
      return "Uploading archive…";
  }
}

const SAVED_NOT_ARCHIVED = "Saved, but the page couldn't be archived: ";

const ERROR_TITLES: Record<SaveErrorKind, string> = {
  token: "Token rejected",
  cloudflare: "Cloudflare login expired",
  unreachable: "Server unreachable",
  "too-large": "Page too large to archive",
  capture: "Couldn't archive the page",
  other: "Save failed",
};

/** A failed save as a short title and a one-line message for the popup. */
export function saveErrorText(
  error: { kind: SaveErrorKind; message: string },
  bookmarkId: number | undefined,
): { title: string; message: string } {
  const saved = bookmarkId !== undefined;
  if (error.kind === "capture" && saved) {
    const reason = error.message.startsWith(SAVED_NOT_ARCHIVED)
      ? error.message.slice(SAVED_NOT_ARCHIVED.length)
      : error.message;
    return { title: "Saved, but not archived", message: reason };
  }
  if (error.kind === "too-large") {
    return {
      title: ERROR_TITLES["too-large"],
      message: saved ? "The bookmark was saved." : "Nothing was saved.",
    };
  }
  return { title: ERROR_TITLES[error.kind], message: error.message };
}

/** The server only stores http(s) URLs. */
export function isBookmarkableURL(url: string | undefined): url is string {
  if (!url) return false;
  try {
    const p = new URL(url).protocol;
    return p === "http:" || p === "https:";
  } catch {
    return false;
  }
}

/**
 * A "done" event seen first (already waiting when the popup opened) is shown as
 * a notice only; a live one closes the popup shortly after.
 */
export function doneHandling(firstEventSinceOpen: boolean): "notice" | "close" {
  return firstEventSinceOpen ? "notice" : "close";
}
