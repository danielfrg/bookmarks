import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Settings, Tag } from "./api.ts";
import {
  clampItemsPerPage,
  filterTags,
  importSummary,
  mergeSettings,
  moveItem,
  normalizeTagName,
  wouldMerge,
} from "./pages.ts";

const tags: Tag[] = [
  { id: 1, name: "go", count: 3 },
  { id: 2, name: "Svelte", count: 1 },
  { id: 3, name: "rust", count: 0 },
];

describe("clampItemsPerPage", () => {
  it("clamps and rounds", () => {
    assert.equal(clampItemsPerPage(5), 10);
    assert.equal(clampItemsPerPage(500), 100);
    assert.equal(clampItemsPerPage(25.6), 26);
    assert.equal(clampItemsPerPage("40"), 40);
  });
  it("falls back to 30 for non-numbers", () => {
    assert.equal(clampItemsPerPage(""), 30);
    assert.equal(clampItemsPerPage("abc"), 30);
    assert.equal(clampItemsPerPage(NaN), 30);
  });
});

describe("moveItem", () => {
  it("moves up and down", () => {
    assert.deepEqual(moveItem([1, 2, 3], 1, -1), [2, 1, 3]);
    assert.deepEqual(moveItem([1, 2, 3], 1, 1), [1, 3, 2]);
  });
  it("is a no-op at the ends and does not mutate", () => {
    const ids = [1, 2, 3];
    assert.deepEqual(moveItem(ids, 0, -1), [1, 2, 3]);
    assert.deepEqual(moveItem(ids, 2, 1), [1, 2, 3]);
    assert.notEqual(moveItem(ids, 1, 1), ids);
    assert.deepEqual(ids, [1, 2, 3]);
  });
});

describe("wouldMerge", () => {
  it("finds another tag case-insensitively", () => {
    assert.equal(wouldMerge(tags, 1, "#SVELTE")?.id, 2);
  });
  it("ignores itself, blanks and new names", () => {
    assert.equal(wouldMerge(tags, 1, "GO"), null);
    assert.equal(wouldMerge(tags, 1, "  "), null);
    assert.equal(wouldMerge(tags, 1, "zig"), null);
  });
});

describe("misc", () => {
  it("normalizes tag names", () => {
    assert.equal(normalizeTagName(" #Foo "), "foo");
  });
  it("filters tags", () => {
    assert.deepEqual(
      filterTags(tags, "#sv").map((t) => t.id),
      [2],
    );
    assert.equal(filterTags(tags, "").length, 3);
  });
  it("summarizes imports", () => {
    assert.equal(
      importSummary({ created: 1, updated: 2, skipped: 3 }),
      "Created 1, updated 2, skipped 3",
    );
  });
});

describe("mergeSettings", () => {
  const base: Settings = {
    favicons_enabled: true,
    previews_enabled: true,
    wayback_enabled: false,
    default_unread: false,
    auto_tag_rules: "a b",
    ui: { items_per_page: 30, theme: "x" },
  };
  it("applies a flat patch without touching the base", () => {
    const m = mergeSettings(base, { default_unread: true });
    assert.equal(m.default_unread, true);
    assert.equal(m.auto_tag_rules, "a b");
    assert.equal(base.default_unread, false);
  });
  it("merges ui keys", () => {
    const m = mergeSettings(base, { ui: { items_per_page: 50 } });
    assert.deepEqual(m.ui, { items_per_page: 50, theme: "x" });
  });
  it("a later patch on confirmed state does not carry an earlier failed one", () => {
    const failed = { favicons_enabled: false };
    const later = mergeSettings(base, { wayback_enabled: true });
    assert.equal(later.favicons_enabled, true);
    assert.equal(mergeSettings(base, failed).favicons_enabled, false);
  });
});
