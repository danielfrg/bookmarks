import assert from "node:assert/strict";
import { test } from "node:test";
import { joinChunks } from "./reader.ts";

test("joinChunks adds a paragraph break when needed", () => {
  assert.equal(joinChunks("a", "b"), "a\n\nb");
});
test("joinChunks does not double a trailing newline", () => {
  assert.equal(joinChunks("a\n\n", "b"), "a\n\nb");
  assert.equal(joinChunks("a\n", "b"), "a\nb");
});
test("joinChunks handles empty sides", () => {
  assert.equal(joinChunks("", "b"), "b");
  assert.equal(joinChunks("a", ""), "a");
});
