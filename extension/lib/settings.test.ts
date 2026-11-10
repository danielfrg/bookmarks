import { test } from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULTS,
  isConfigured,
  loadSettings,
  normalizeServerURL,
  saveSettings,
  type StorageLike,
} from "./settings.ts";

function memoryStorage(initial: Record<string, unknown> = {}): StorageLike {
  const data = { ...initial };
  return {
    async get(keys) {
      return Object.fromEntries(keys.filter((k) => k in data).map((k) => [k, data[k]]));
    },
    async set(items) {
      Object.assign(data, items);
    },
  };
}

test("loads defaults when storage is empty", async () => {
  assert.deepEqual(await loadSettings(memoryStorage()), DEFAULTS);
  assert.deepEqual(DEFAULTS, {
    serverURL: "",
    token: "",
    unreadByDefault: false,
  });
});

test("round-trips saved settings", async () => {
  const storage = memoryStorage();
  const s = {
    serverURL: "https://b.example.com",
    token: "abc",
    unreadByDefault: true,
  };
  await saveSettings(s, storage);
  assert.deepEqual(await loadSettings(storage), s);
});

test("ignores wrongly typed stored values", async () => {
  const s = await loadSettings(memoryStorage({ token: 5, unreadByDefault: "no" }));
  assert.deepEqual(s, DEFAULTS);
});

test("normalizeServerURL strips path and trailing slash", () => {
  assert.equal(normalizeServerURL("https://b.example.com/api/"), "https://b.example.com");
  assert.equal(normalizeServerURL("  http://localhost:8080/  "), "http://localhost:8080");
});

test("normalizeServerURL rejects invalid input", () => {
  assert.equal(normalizeServerURL("ftp://x"), null);
  assert.equal(normalizeServerURL("nope"), null);
  assert.equal(normalizeServerURL(""), null);
});

test("isConfigured needs a valid URL and a token", () => {
  const ok = { ...DEFAULTS, serverURL: "https://b.example.com", token: "t" };
  assert.equal(isConfigured(ok), true);
  assert.equal(isConfigured({ ...ok, token: "" }), false);
  assert.equal(isConfigured({ ...ok, serverURL: "nope" }), false);
  assert.equal(isConfigured(DEFAULTS), false);
});
