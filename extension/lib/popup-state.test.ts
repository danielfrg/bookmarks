import assert from "node:assert/strict";
import { test } from "node:test";
import {
  CloudflareLoginError,
  TokenError,
  UnreachableError,
  ApiError,
  type Bookmark,
  type CheckResult,
} from "./api.ts";
import {
  capabilityMessage,
  checkFailure,
  doneHandling,
  initialForm,
  isBookmarkableURL,
  progressText,
  saveErrorText,
  viewFromStatus,
} from "./popup-state.ts";

const settings = { unreadByDefault: false };
const bookmark: Bookmark = {
  id: 7,
  url: "https://a.test/x",
  title: "Saved title",
  description: "Saved desc",
  notes: "Saved notes",
  tags: ["go", "web"],
  unread: true,
  date_added: "2026-11-07T10:00:00Z",
  basic_capture_only: false,
  archive_count: 1,
};
const meta = { title: "Meta title", description: "Meta desc", preview_image_url: "" };
const fresh: CheckResult = {
  bookmark: null,
  basic_capture_only: false,
  metadata: meta,
  auto_tags: ["news"],
};
const ok = { ok: true } as const;

test("initialForm prefills a new bookmark from the tab, metadata and settings", () => {
  const f = initialForm({ settings, check: fresh, tabTitle: "Tab title", capture: ok });
  assert.deepEqual(f.form, {
    title: "Tab title",
    description: "Meta desc",
    notes: "",
    tags: ["news"],
    unread: false,
  });
  assert.equal(f.existing, null);
  assert.deepEqual(f.actions, { saveLabel: "Save", saveArchives: true, archiveLabel: null });
  assert.equal(f.archiveDisabledReason, null);
  assert.deepEqual(f.notices, []);
});

test("initialForm falls back to metadata title and honours the unread default", () => {
  const f = initialForm({
    settings: { unreadByDefault: true },
    check: fresh,
    tabTitle: "  ",
    capture: ok,
  });
  assert.equal(f.form.title, "Meta title");
  assert.equal(f.form.unread, true);
});

test("initialForm uses the existing bookmark's fields and an Already saved notice", () => {
  const f = initialForm({
    settings,
    check: { ...fresh, bookmark },
    tabTitle: "Tab title",
    capture: ok,
  });
  assert.equal(f.existing, bookmark);
  assert.deepEqual(f.form, {
    title: "Saved title",
    description: "Saved desc",
    notes: "Saved notes",
    tags: ["go", "web"],
    unread: true,
  });
  assert.deepEqual(f.notices, [{ kind: "already-saved", dateAdded: bookmark.date_added }]);
  assert.deepEqual(f.actions, {
    saveLabel: "Update",
    saveArchives: false,
    archiveLabel: "Re-archive",
  });
});

test("initialForm offers Archive for a saved bookmark that has no archive", () => {
  const f = initialForm({
    settings: { unreadByDefault: false },
    check: { ...fresh, bookmark: { ...bookmark, archive_count: 0 } },
    tabTitle: "t",
    capture: ok,
  });
  assert.deepEqual(f.actions, {
    saveLabel: "Update",
    saveArchives: false,
    archiveLabel: "Archive",
  });
});

test("initialForm offers Archive when only a basic capture exists", () => {
  const f = initialForm({
    settings: { unreadByDefault: false },
    check: {
      ...fresh,
      bookmark: { ...bookmark, basic_capture_only: true },
      basic_capture_only: true,
    },
    tabTitle: "t",
    capture: ok,
  });
  assert.deepEqual(f.actions, {
    saveLabel: "Update",
    saveArchives: false,
    archiveLabel: "Archive",
  });
  assert.deepEqual(
    f.notices.map((n) => n.kind),
    ["already-saved", "basic-capture"],
  );
});

test("a new page that can't be archived gets a plain Save", () => {
  const f = initialForm({
    settings,
    check: fresh,
    tabTitle: "t",
    capture: { ok: false, reason: "pdf" },
  });
  assert.deepEqual(f.actions, { saveLabel: "Save", saveArchives: false, archiveLabel: null });
});

test("initialForm disables archive with a reason when the page is not capturable", () => {
  const f = initialForm({
    settings,
    check: {
      ...fresh,
      bookmark: { ...bookmark, basic_capture_only: true },
      basic_capture_only: true,
    },
    tabTitle: "t",
    capture: { ok: false, reason: "pdf" },
  });
  assert.deepEqual(f.actions, { saveLabel: "Update", saveArchives: false, archiveLabel: null });
  assert.equal(f.archiveDisabledReason, "PDFs are archived by the server");
  assert.ok(!f.notices.some((n) => n.kind === "basic-capture"));
});

test("capabilityMessage covers every reason", () => {
  assert.equal(capabilityMessage("browser-page"), "Chrome pages can't be archived");
  assert.equal(capabilityMessage("web-store"), "The Chrome Web Store can't be archived");
  assert.equal(
    capabilityMessage("file-access"),
    "Allow file access for this extension to archive local files",
  );
  assert.equal(capabilityMessage("unsupported-scheme"), "This page can't be archived");
});

test("checkFailure maps typed errors; other errors become a warning", () => {
  assert.deepEqual(checkFailure(new CloudflareLoginError("x", 302)), {
    kind: "cloudflare",
    title: "Log in to Cloudflare",
    blocking: true,
  });
  assert.deepEqual(checkFailure(new TokenError("nope")), {
    kind: "token",
    title: "Token rejected",
    blocking: true,
  });
  assert.deepEqual(checkFailure(new UnreachableError()), {
    kind: "unreachable",
    title: "Can't reach the server",
    blocking: true,
  });
  assert.deepEqual(checkFailure(new ApiError("boom", 500)), {
    kind: "other",
    title: "boom",
    blocking: false,
  });
  assert.equal(checkFailure("weird").title, "weird");
});

test("viewFromStatus maps the latest background event", () => {
  assert.deepEqual(viewFromStatus(null), { view: "form" });
  assert.deepEqual(viewFromStatus({ stage: "capturing" }), {
    view: "progress",
    stage: "capturing",
  });
  assert.deepEqual(viewFromStatus({ stage: "saving" }), { view: "progress", stage: "saving" });
  assert.deepEqual(viewFromStatus({ stage: "uploading" }), {
    view: "progress",
    stage: "uploading",
  });
  assert.deepEqual(viewFromStatus({ stage: "done", bookmarkId: 3 }), { view: "done" });
  const error = { kind: "capture" as const, message: "bad" };
  assert.deepEqual(viewFromStatus({ stage: "error", bookmarkId: 3, error }), {
    view: "error",
    error,
    bookmarkId: 3,
  });
});

test("progressText", () => {
  assert.equal(progressText("saving"), "Saving…");
  assert.equal(progressText("capturing"), "Archiving page…");
  assert.equal(progressText("uploading"), "Uploading archive…");
});

test("saveErrorText gives a short title and a one-line message", () => {
  assert.deepEqual(
    saveErrorText(
      { kind: "capture", message: "Saved, but the page couldn't be archived: capture timed out" },
      5,
    ),
    { title: "Saved, but not archived", message: "capture timed out" },
  );
  assert.deepEqual(saveErrorText({ kind: "too-large", message: "Page too large to archive" }, 5), {
    title: "Page too large to archive",
    message: "The bookmark was saved.",
  });
  assert.deepEqual(saveErrorText({ kind: "capture", message: "Boom" }, undefined), {
    title: "Couldn't archive the page",
    message: "Boom",
  });
  assert.deepEqual(saveErrorText({ kind: "token", message: "Rejected" }, 5), {
    title: "Token rejected",
    message: "Rejected",
  });
  assert.deepEqual(saveErrorText({ kind: "other", message: "Boom" }, undefined), {
    title: "Save failed",
    message: "Boom",
  });
});

test("isBookmarkableURL accepts only http(s)", () => {
  assert.equal(isBookmarkableURL("https://a.test"), true);
  assert.equal(isBookmarkableURL("http://a.test"), true);
  assert.equal(isBookmarkableURL("chrome://extensions"), false);
  assert.equal(isBookmarkableURL("file:///x.html"), false);
  assert.equal(isBookmarkableURL(undefined), false);
  assert.equal(isBookmarkableURL("nope"), false);
});

test("doneHandling shows a waiting result as a notice and closes after a live one", () => {
  assert.equal(doneHandling(true), "notice");
  assert.equal(doneHandling(false), "close");
});
