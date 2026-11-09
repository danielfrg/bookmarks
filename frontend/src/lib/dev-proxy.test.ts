import test from "node:test";
import assert from "node:assert/strict";
import type { UserConfig } from "vite";

// The query string forces a fresh evaluation so each case sees its own env.
const loadConfig = async (tag: string): Promise<UserConfig> =>
  (await import(`../../vite.config.ts?${tag}`)).default;

test("vite config serves under /_/assets/ and proxies the API paths unchanged", async () => {
  const previous = process.env.BOOKMARKS_API_TARGET;
  try {
    delete process.env.BOOKMARKS_API_TARGET;
    const config = await loadConfig("default");
    assert.equal(config.base, "/_/assets/");
    const proxy = config.server?.proxy ?? {};
    assert.deepEqual(Object.keys(proxy).sort(), [
      "/_/api",
      "/_/archives",
      "/_/favicons",
      "/_/previews",
    ]);
    for (const rule of Object.values(proxy)) {
      assert.deepEqual(rule, { target: "http://127.0.0.1:8080", changeOrigin: false });
    }
    process.env.BOOKMARKS_API_TARGET = "http://127.0.0.1:9999";
    const custom = await loadConfig("custom");
    for (const rule of Object.values(custom.server?.proxy ?? {})) {
      assert.deepEqual(rule, { target: "http://127.0.0.1:9999", changeOrigin: false });
    }
  } finally {
    if (previous === undefined) delete process.env.BOOKMARKS_API_TARGET;
    else process.env.BOOKMARKS_API_TARGET = previous;
  }
});
