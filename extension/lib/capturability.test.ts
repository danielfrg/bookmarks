import { test } from "node:test";
import assert from "node:assert/strict";
import { capturability } from "./capturability.ts";

const cases: [string, ReturnType<typeof capturability>, Parameters<typeof capturability>[1]?][] = [
  ["https://example.com/article", { ok: true }],
  ["http://example.com/", { ok: true }],
  ["HTTPS://EXAMPLE.COM/", { ok: true }],
  ["https://example.com/report.pdf.html", { ok: true }],
  ["chrome://settings", { ok: false, reason: "browser-page" }],
  ["chrome-extension://abc/popup.html", { ok: false, reason: "browser-page" }],
  ["edge://flags", { ok: false, reason: "browser-page" }],
  ["about:blank", { ok: false, reason: "browser-page" }],
  ["view-source:https://example.com/", { ok: false, reason: "browser-page" }],
  ["devtools://devtools/bundled/inspector.html", { ok: false, reason: "browser-page" }],
  ["https://chromewebstore.google.com/detail/foo/abc", { ok: false, reason: "web-store" }],
  ["https://chrome.google.com/webstore/detail/foo", { ok: false, reason: "web-store" }],
  ["https://chrome.google.com/search", { ok: true }],
  ["https://example.com/paper.pdf", { ok: false, reason: "pdf" }],
  ["https://example.com/paper.PDF?download=1#page=2", { ok: false, reason: "pdf" }],
  ["https://example.com/viewer?id=1", { ok: false, reason: "pdf" }, { mimeIsPdf: true }],
  ["file:///Users/me/page.html", { ok: false, reason: "file-access" }],
  ["file:///Users/me/page.html", { ok: false, reason: "file-access" }, { fileAccess: false }],
  ["file:///Users/me/page.html", { ok: true }, { fileAccess: true }],
  ["file:///Users/me/doc.pdf", { ok: false, reason: "pdf" }, { fileAccess: true }],
  ["ftp://example.com/file", { ok: false, reason: "unsupported-scheme" }],
  ["data:text/html,hi", { ok: false, reason: "unsupported-scheme" }],
  ["blob:https://example.com/123", { ok: false, reason: "unsupported-scheme" }],
  ["", { ok: false, reason: "unsupported-scheme" }],
  ["not a url", { ok: false, reason: "unsupported-scheme" }],
];

for (const [url, want, opts] of cases) {
  test(`capturability(${JSON.stringify(url)}${opts ? ", " + JSON.stringify(opts) : ""})`, () => {
    assert.deepEqual(capturability(url, opts), want);
  });
}
