import { test } from "node:test";
import assert from "node:assert/strict";
import { archiveFilename, runSave, type SaveDeps } from "./save.ts";
import type { SaveEvent, SaveRequest } from "./messages.ts";
import {
  ApiError,
  CloudflareLoginError,
  TokenError,
  TooLargeError,
  UnreachableError,
} from "./api.ts";

const req: SaveRequest = {
  tabId: 7,
  url: "https://example.com/a",
  title: "Hello: World/1",
  description: "d",
  notes: "n",
  tags: ["x"],
  unread: false,
  archive: true,
};

function fake(over: Partial<SaveDeps> = {}) {
  const calls: string[] = [];
  const events: SaveEvent[] = [];
  const badges: string[] = [];
  const uploads: { id: number; blob: Blob; name: string }[] = [];
  const deps: SaveDeps = {
    api: {
      async save(input: unknown, capture: string) {
        calls.push(`save:${capture}`);
        return { bookmark: { id: 42 }, created: true };
      },
      async uploadArchive(id: number, blob: Blob, name: string) {
        calls.push("upload");
        uploads.push({ id, blob, name });
      },
    } as unknown as SaveDeps["api"],
    async capture() {
      calls.push("capture");
      return { html: "<html></html>", title: "T" };
    },
    async canCapture() {
      return { ok: true };
    },
    emit: (e) => events.push(e),
    badge: (s) => badges.push(s),
    ...over,
  };
  return { deps, calls, events, badges, uploads };
}

test("happy path: stages, calls and badge in order", async () => {
  const f = fake();
  const final = await runSave(req, f.deps);
  assert.deepEqual(f.calls, ["save:none", "capture", "upload"]);
  assert.deepEqual(
    f.events.map((e) => e.stage),
    ["saving", "capturing", "uploading", "done"],
  );
  assert.deepEqual(f.badges, ["busy", "ok"]);
  assert.equal(final.stage, "done");
  assert.equal(final.bookmarkId, 42);
  assert.equal(f.uploads[0]!.id, 42);
  assert.equal(f.uploads[0]!.name, "Hello World1.html");
  assert.equal(f.uploads[0]!.blob.type, "text/html");
  assert.equal(await f.uploads[0]!.blob.text(), "<html></html>");
});

test("archive off: server capture, no capture call", async () => {
  const f = fake();
  const final = await runSave({ ...req, archive: false }, f.deps);
  assert.deepEqual(f.calls, ["save:server"]);
  assert.equal(final.stage, "done");
  assert.deepEqual(f.badges, ["busy", "ok"]);
});

test("uncapturable page: server capture", async () => {
  const f = fake({ canCapture: async () => ({ ok: false, reason: "pdf" }) });
  await runSave(req, f.deps);
  assert.deepEqual(f.calls, ["save:server"]);
});

test("capture failure after save keeps the bookmark and reports capture", async () => {
  const f = fake({
    capture: async () => {
      throw new Error("timed out");
    },
  });
  const final = await runSave(req, f.deps);
  assert.deepEqual(f.calls, ["save:none"]);
  assert.equal(final.stage, "error");
  assert.equal(final.bookmarkId, 42);
  assert.equal(final.error?.kind, "capture");
  assert.equal(final.error?.message, "Saved, but the page couldn't be archived: timed out");
  assert.deepEqual(f.badges, ["busy", "error"]);
});

test("upload too large", async () => {
  const f = fake();
  f.deps.api.uploadArchive = async () => {
    throw new TooLargeError("limit 10 MB");
  };
  const final = await runSave(req, f.deps);
  assert.equal(final.error?.kind, "too-large");
  assert.equal(final.error?.message, "Page too large to archive");
  assert.equal(final.bookmarkId, 42);
});

test("upload failure of another kind is a capture error", async () => {
  const f = fake();
  f.deps.api.uploadArchive = async () => {
    throw new ApiError("boom", 500);
  };
  const final = await runSave(req, f.deps);
  assert.equal(final.error?.kind, "capture");
  assert.match(final.error?.message ?? "", /^Saved, but the page couldn't be archived: boom/);
});

test("save completes after the popup port disconnects", async () => {
  const f = fake({
    emit: () => {
      throw new Error("Attempting to use a disconnected port object");
    },
  });
  const final = await runSave(req, f.deps);
  assert.deepEqual(f.calls, ["save:none", "capture", "upload"]);
  assert.equal(final.stage, "done");
});

test("error mapping on save", async () => {
  const cases: [Error, string][] = [
    [new TokenError("bad token"), "token"],
    [new CloudflareLoginError("login", 302), "cloudflare"],
    [new UnreachableError(), "unreachable"],
    [new TooLargeError("big"), "too-large"],
    [new ApiError("nope", 500), "other"],
    [new Error("weird"), "other"],
  ];
  for (const [err, kind] of cases) {
    const f = fake();
    f.deps.api.save = async () => {
      throw err;
    };
    const final = await runSave(req, f.deps);
    assert.equal(final.stage, "error");
    assert.equal(final.error?.kind, kind);
    assert.equal(final.bookmarkId, undefined);
    assert.deepEqual(f.badges, ["busy", "error"]);
    assert.deepEqual(f.calls, []);
  }
});

test("canCapture throwing falls back to server capture", async () => {
  const f = fake({
    canCapture: async () => {
      throw new Error("x");
    },
  });
  await runSave(req, f.deps);
  assert.deepEqual(f.calls, ["save:server"]);
});

test("archiveFilename sanitizes", () => {
  assert.equal(archiveFilename("Hello: World/1"), "Hello World1.html");
  assert.equal(archiveFilename("a-b_c d"), "a-b_c d.html");
  assert.equal(archiveFilename("日本語 ???"), "日本語.html");
  assert.equal(archiveFilename("???"), "page.html");
  assert.equal(archiveFilename(""), "page.html");
  assert.equal(archiveFilename("x".repeat(200)), "x".repeat(80) + ".html");
  assert.equal(archiveFilename("  spaced  "), "spaced.html");
});

test("after a successful save, upload failures are capture errors", async () => {
  for (const err of [
    new UnreachableError(),
    new TokenError("bad"),
    new CloudflareLoginError("x", 302),
  ]) {
    const f = fake();
    f.deps.api.uploadArchive = async () => {
      throw err;
    };
    const final = await runSave(req, f.deps);
    assert.equal(final.error?.kind, "capture");
    assert.match(final.error?.message ?? "", /^Saved, but the page couldn't be archived: /);
    assert.equal(final.bookmarkId, 42);
  }
});

test("archiveFilename counts code points", () => {
  assert.equal(archiveFilename("𠮷".repeat(100)), "𠮷".repeat(80) + ".html");
});
