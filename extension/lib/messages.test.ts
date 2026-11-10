import { test } from "node:test";
import assert from "node:assert/strict";
import { captureIdFromPort, capturePortName, isRelayFetchRequest } from "./messages.ts";

test("capture port names round-trip", () => {
  assert.equal(capturePortName("abc-1"), "capture:abc-1");
  assert.equal(captureIdFromPort("capture:abc-1"), "abc-1");
  assert.equal(captureIdFromPort("capture:"), null);
  assert.equal(captureIdFromPort("other"), null);
});

test("recognizes relay-fetch requests", () => {
  assert.ok(isRelayFetchRequest({ type: "relay-fetch", url: "https://a/b.css" }));
  assert.ok(isRelayFetchRequest({ type: "relay-fetch", url: "https://a/b.css", extra: 1 }));
  assert.ok(!isRelayFetchRequest({ type: "relay-fetch" }));
  assert.ok(!isRelayFetchRequest({ type: "relay-fetch", url: 1 }));
  assert.ok(!isRelayFetchRequest({ type: "other", url: "x" }));
  assert.ok(!isRelayFetchRequest(null));
  assert.ok(!isRelayFetchRequest("relay-fetch"));
});
