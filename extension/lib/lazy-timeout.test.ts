import { test } from "node:test";
import assert from "node:assert/strict";
import { LAZY_CLEAR, LAZY_SET, LazyTimeouts, isLazyTimeoutMessage } from "./lazy-timeout.ts";

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

test("recognizes SingleFile lazy-timeout messages", () => {
  assert.ok(isLazyTimeoutMessage({ method: LAZY_SET, type: "idleTimeout", delay: 3000 }));
  assert.ok(isLazyTimeoutMessage({ method: LAZY_CLEAR, type: "idleTimeout" }));
  assert.ok(!isLazyTimeoutMessage({ method: LAZY_SET, type: "idleTimeout" }));
  assert.ok(!isLazyTimeoutMessage({ method: LAZY_SET, type: "x", delay: Number.NaN }));
  assert.ok(!isLazyTimeoutMessage({ method: LAZY_CLEAR }));
  assert.ok(!isLazyTimeoutMessage({ type: "relay-fetch", url: "x" }));
  assert.ok(!isLazyTimeoutMessage(null));
});

test("fires a set timer back to the frame that set it", async () => {
  const fired: unknown[] = [];
  const t = new LazyTimeouts((...a) => fired.push(a));
  t.handle(7, 0, { method: LAZY_SET, type: "loadTimeout", delay: 5 });
  assert.equal(fired.length, 0);
  await wait(20);
  assert.deepEqual(fired, [[7, 0, "loadTimeout"]]);
  assert.equal(t.size, 0);
});

test("setting the same type again replaces the timer; clear cancels it", async () => {
  const fired: unknown[] = [];
  const t = new LazyTimeouts((...a) => fired.push(a));
  t.handle(1, 0, { method: LAZY_SET, type: "idleTimeout", delay: 5 });
  t.handle(1, 0, { method: LAZY_SET, type: "idleTimeout", delay: 30 });
  await wait(15);
  assert.equal(fired.length, 0, "first timer replaced");
  t.handle(1, 0, { method: LAZY_CLEAR, type: "idleTimeout" });
  await wait(30);
  assert.equal(fired.length, 0, "cleared");
});

test("timers are per tab and clearTab drops only that tab", async () => {
  const fired: unknown[] = [];
  const t = new LazyTimeouts((...a) => fired.push(a));
  t.handle(1, 0, { method: LAZY_SET, type: "idleTimeout", delay: 5 });
  t.handle(2, 0, { method: LAZY_SET, type: "idleTimeout", delay: 5 });
  t.clearTab(1);
  await wait(20);
  assert.deepEqual(fired, [[2, 0, "idleTimeout"]]);
});
