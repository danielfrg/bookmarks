import test from "node:test";
import assert from "node:assert/strict";
import {
  activeTagsOf,
  bulkRequest,
  commitTagText,
  syncSearchText,
  displayTitle,
  formatBytes,
  groupTags,
  hostOf,
  toggleTagInQuery,
} from "./query.ts";

test("toggleTagInQuery adds and removes tag tokens", () => {
  assert.equal(toggleTagInQuery("", "go"), "#go");
  assert.equal(toggleTagInQuery("foo", "go"), "foo #go");
  assert.equal(toggleTagInQuery("foo #Go bar", "go"), "foo bar");
  assert.equal(toggleTagInQuery("foo #gopher", "go"), "foo #gopher #go");
  assert.equal(toggleTagInQuery("  foo   #GO  ", "Go"), "foo");
  assert.equal(toggleTagInQuery("  foo   bar ", "go"), "foo bar #go");
  assert.equal(toggleTagInQuery("#go #go x", "go"), "x");
});

test("displayTitle falls back to the URL", () => {
  assert.equal(displayTitle({ title: "Hi", url: "https://a.test" }), "Hi");
  assert.equal(displayTitle({ title: "  ", url: "https://a.test" }), "https://a.test");
});

test("hostOf strips www and tolerates invalid URLs", () => {
  assert.equal(hostOf("https://www.example.com/x"), "example.com");
  assert.equal(hostOf("http://sub.example.com:8080/"), "sub.example.com");
  assert.equal(hostOf("not a url"), "");
});

test("groupTags buckets A-Z with a # bucket for non-letters", () => {
  const t = (id: number, name: string) => ({ id, name, count: 1 });
  const groups = groupTags([
    t(1, "zeta"),
    t(2, "Alpha"),
    t(3, "2fa"),
    t(4, "apple"),
    t(5, "élan"),
    t(6, "_x"),
  ]);
  assert.deepEqual(
    groups.map((g) => [g.letter, g.tags.map((x) => x.name)]),
    [
      ["A", ["Alpha", "apple"]],
      ["E", ["élan"]],
      ["Z", ["zeta"]],
      ["#", ["2fa", "_x"]],
    ],
  );
});

test("activeTagsOf lists lower-cased #tag tokens", () => {
  assert.deepEqual(activeTagsOf('foo #Go "x y" #sql #'), ["go", "sql"]);
  assert.deepEqual(activeTagsOf(""), []);
});

test("syncSearchText only follows committed query changes", () => {
  // Typing while the committed query is unchanged is never overwritten.
  assert.equal(syncSearchText("", "", "foo "), "foo ");
  assert.equal(syncSearchText("", "", "foo bar"), "foo bar");
  // Debounce commits the trimmed text: keep the trailing space.
  assert.equal(syncSearchText("", "foo", "foo "), "foo ");
  // External change (tag click, back/forward) replaces the text.
  assert.equal(syncSearchText("foo", "foo #go", "foo"), "foo #go");
  assert.equal(syncSearchText("foo", "", "foo"), "");
});

test("formatBytes uses base 1024 with one decimal from KB up", () => {
  assert.equal(formatBytes(0), "0 B");
  assert.equal(formatBytes(512), "512 B");
  assert.equal(formatBytes(1024), "1.0 KB");
  assert.equal(formatBytes(2.4 * 1024 * 1024), "2.4 MB");
  assert.equal(formatBytes(1.1 * 1024 ** 3), "1.1 GB");
  assert.equal(formatBytes(1023), "1023 B");
  assert.equal(formatBytes(-5), "0 B");
  assert.equal(formatBytes(1048575), "1.0 MB");
  assert.equal(formatBytes(1048575 - 1024), "1023.0 KB");
});

test("commitTagText splits, lower-cases and dedupes", () => {
  assert.deepEqual(commitTagText([], "Go"), ["go"]);
  assert.deepEqual(commitTagText(["go"], "rust, Go  web"), ["go", "rust", "web"]);
  assert.deepEqual(commitTagText(["a"], "b,b c"), ["a", "b", "c"]);
  assert.deepEqual(commitTagText(["a"], "  ,, "), ["a"]);
  assert.deepEqual(commitTagText(["a"], "#Tag"), ["a", "tag"]);
  const cur = ["a"];
  commitTagText(cur, "b");
  assert.deepEqual(cur, ["a"]);
});

const baseView = {
  q: "",
  filter: "active",
  sort: "added_desc",
  saved: null,
  page: 1,
  view: null,
  edit: null,
  create: null,
  read: null,
  pageName: null,
} as unknown as import("./view.ts").View;

test("bulkRequest sends ids for an explicit selection", () => {
  assert.deepEqual(bulkRequest({ ids: [3, 1], allMatching: false }, baseView, "archive"), {
    ids: [3, 1],
    action: "archive",
  });
  assert.deepEqual(bulkRequest({ ids: [2], allMatching: false }, baseView, "tag", ["go"]), {
    ids: [2],
    action: "tag",
    tags: ["go"],
  });
});

test("bulkRequest sends the current search for all matching", () => {
  assert.deepEqual(bulkRequest({ ids: [], allMatching: true }, { ...baseView, q: "#go" }, "read"), {
    query: { q: "#go", scope: "active" },
    action: "read",
  });
  assert.deepEqual(
    bulkRequest({ ids: [], allMatching: true }, { ...baseView, filter: "unread" }, "archive"),
    { query: { q: "", scope: "active", unread: "yes" }, action: "archive" },
  );
  assert.deepEqual(
    bulkRequest({ ids: [], allMatching: true }, { ...baseView, filter: "archived" }, "unarchive"),
    { query: { q: "", scope: "archived" }, action: "unarchive" },
  );
});

test("bulk delete all matching is disabled without a filter", () => {
  const sel = { ids: [], allMatching: true };
  assert.equal(bulkRequest(sel, baseView, "delete"), null);
  assert.equal(bulkRequest(sel, { ...baseView, q: "  " }, "delete"), null);
  assert.equal(bulkRequest(sel, { ...baseView, filter: "archived" }, "delete"), null);
  assert.notEqual(bulkRequest(sel, { ...baseView, q: "foo" }, "delete"), null);
  assert.notEqual(bulkRequest(sel, { ...baseView, filter: "unread" }, "delete"), null);
  // Explicit ids may always be deleted.
  assert.notEqual(bulkRequest({ ids: [1], allMatching: false }, baseView, "delete"), null);
});

test("bulkRequest refuses all matching inside a saved search", () => {
  assert.equal(
    bulkRequest({ ids: [], allMatching: true }, { ...baseView, saved: 4 }, "read"),
    null,
  );
});
