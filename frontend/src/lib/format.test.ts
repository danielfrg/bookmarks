import test from "node:test";
import assert from "node:assert/strict";
import { formatRelativeTime, formatTimestamp } from "./format.ts";

const now = Date.parse("2026-03-10T12:00:00Z");
const ago = (seconds: number) => new Date(now - seconds * 1000).toISOString();

test("formatRelativeTime counts minutes, hours and days", () => {
  assert.equal(formatRelativeTime(ago(5), now), "just now");
  assert.equal(formatRelativeTime(ago(5 * 60), now), "5m ago");
  assert.equal(formatRelativeTime(ago(3 * 3600), now), "3h ago");
  assert.equal(formatRelativeTime(ago(2 * 86400), now), "2d ago");
});

test("formatRelativeTime falls back to a date after a week and a dash when empty", () => {
  assert.notEqual(formatRelativeTime(ago(30 * 86400), now), "30d ago");
  assert.equal(formatRelativeTime("", now), "—");
  assert.equal(formatRelativeTime(null, now), "—");
});

test("formatTimestamp handles empty values", () => {
  assert.equal(formatTimestamp(undefined), "—");
  assert.match(formatTimestamp("2026-03-10T12:00:00Z"), /\d/);
});
