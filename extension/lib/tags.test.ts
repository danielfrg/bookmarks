import assert from "node:assert/strict";
import { test } from "node:test";
import { commitTagText } from "./tags.ts";

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
