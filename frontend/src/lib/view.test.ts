import test from "node:test";
import assert from "node:assert/strict";
import { listParams, nextURL, readView, writeView, type View } from "./view.ts";

const base: View = {
  q: "",
  filter: "all",
  sort: "added_desc",
  saved: null,
  page: 1,
  view: null,
  edit: null,
  create: null,
  read: null,
  pageName: "list",
  section: "general",
};
/** Reads a path+search, e.g. "?q=a" (the list) or "settings/api". */
const read = (url: string) => readView(new URL(`http://b/${url.replace(/^\//, "")}`));

test("defaults read from an empty URL and write nothing", () => {
  assert.deepEqual(read(""), base);
  assert.equal(writeView(base), "/");
});

test("readView/writeView round trip for each param", () => {
  const cases: [Partial<View>, string][] = [
    [{ q: "foo #go" }, "?q=foo+%23go"],
    [{ filter: "unread" }, "?filter=unread"],
    [{ sort: "title_asc" }, "?sort=title_asc"],
    [{ saved: 3 }, "?saved=3"],
    [{ page: 4 }, "?page=4"],
    [{ view: 12 }, "?view=12"],
    [{ edit: 7 }, "?edit=7"],
    [{ read: 9 }, "?read=9"],
    [{ pageName: "tags" }, "/tags"],
    [{ pageName: "saved" }, "/saved"],
    [{ pageName: "settings" }, "/settings/general"],
    [{ pageName: "settings", section: "api" }, "/settings/api"],
    [{ pageName: "settings", section: "import-export" }, "/settings/import-export"],
    [{ pageName: "tags", view: 4 }, "/tags?view=4"],
    [{ create: { url: "", title: "" } }, "?new=1"],
    [
      { create: { url: "https://a.test/x?y=1", title: "A & B" } },
      "?new=1&url=https%3A%2F%2Fa.test%2Fx%3Fy%3D1&title=A+%26+B",
    ],
  ];
  for (const [patch, url] of cases) {
    const v = { ...base, ...patch };
    const want = url.startsWith("/") ? url : `/${url}`;
    assert.equal(writeView(v), want);
    assert.deepEqual(read(want), v);
  }
});

test("writeView uses a stable param order", () => {
  const v: View = {
    q: "x",
    filter: "unread",
    sort: "added_asc",
    saved: 2,
    page: 3,
    view: 4,
    edit: 5,
    create: { url: "u", title: "t" },
    read: 6,
    pageName: "list",
    section: "general",
  };
  const keys = [...new URLSearchParams(writeView(v).split("?")[1]).keys()];
  assert.deepEqual(keys, [
    "q",
    "filter",
    "sort",
    "saved",
    "page",
    "view",
    "edit",
    "new",
    "url",
    "title",
    "read",
  ]);
});

test("garbage params fall back to defaults", () => {
  assert.deepEqual(read("?view=abc&page=-3&filter=nope"), base);
  assert.deepEqual(read("?saved=0&edit=1.5&read=&page=0&sort=bogus&page_name=x"), {
    ...base,
    sort: "added_desc",
  });
  assert.deepEqual(read("settings/bogus"), { ...base, pageName: "settings" });
  assert.deepEqual(read("nope"), base);
});

test("pages keep only their own state in the URL", () => {
  assert.equal(
    writeView({ ...base, q: "x", page: 2, filter: "unread", pageName: "tags" }),
    "/tags",
  );
});

test("?page_name= is not a page anymore", () => {
  assert.deepEqual(read("?page_name=tags"), base);
});

test("retired filters (active, archived) load as All", () => {
  assert.deepEqual(read("?filter=archived"), base);
  assert.deepEqual(read("?filter=active"), base);
  assert.deepEqual(read("?q=x&filter=archived"), { ...base, q: "x" });
  assert.equal(writeView(read("?filter=archived&sort=title_asc")), "/?sort=title_asc");
  assert.equal(writeView({ ...base, filter: "all" }), "/");
});

test("listParams maps the filters and offsets", () => {
  assert.deepEqual(listParams({ ...base, q: "a" }, 30), {
    q: "a",
    sort: "added_desc",
    limit: 30,
    offset: 0,
  });
  assert.deepEqual(listParams({ ...base, filter: "unread", page: 3, saved: 5 }, 20), {
    q: "",
    unread: "yes",
    sort: "added_desc",
    savedSearch: 5,
    limit: 20,
    offset: 40,
  });
  assert.deepEqual(listParams({ ...base, filter: "all", sort: "title_asc" }, 10), {
    q: "",
    sort: "title_asc",
    limit: 10,
    offset: 0,
  });
});

test("nextURL merges a patch, keeps unrelated state and resets the page on search changes", () => {
  const cur = new URL("http://b/?q=a&page=3&view=2");
  assert.equal(nextURL(cur, { view: null }), "/?q=a&page=3");
  assert.equal(nextURL(cur, { page: 4 }), "/?q=a&page=4&view=2");
  assert.equal(nextURL(cur, { q: "b" }), "/?q=b&view=2");
  assert.equal(nextURL(cur, { filter: "unread" }), "/?q=a&filter=unread&view=2");
  assert.equal(nextURL(new URL("http://b/?filter=unread&page=2"), { filter: "all" }), "/");
  assert.equal(nextURL(new URL("http://b/tags?edit=1"), { edit: null }), "/tags");
  assert.equal(
    nextURL(new URL("http://b/settings/api"), { section: "metadata" }),
    "/settings/metadata",
  );
  assert.equal(nextURL(new URL("http://b/settings/api"), { pageName: "list" }), "/");
});
