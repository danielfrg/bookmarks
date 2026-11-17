import { fileURLToPath, URL } from "node:url";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { preferencesScript } from "@ui-sdk/core";
import { defineConfig } from "vite";
import { colorScheme } from "./src/lib/color-scheme.ts";

const apiTarget = process.env.BOOKMARKS_API_TARGET ?? "http://127.0.0.1:8080";
// Preserve the browser-facing Host so the API can validate its Origin.
const proxied = { target: apiTarget, changeOrigin: false };

export default defineConfig(({ command }) => ({
  plugins: [
    svelte(),
    {
      // Applies the saved color scheme before the app loads, so dark never flashes light.
      name: "preferences-script",
      transformIndexHtml: () => [
        { tag: "script", children: preferencesScript([colorScheme]), injectTo: "head-prepend" },
      ],
    },
  ],
  // The built app is served from /assets/ by the Go binary; the dev server serves it at the root.
  base: command === "build" ? "/assets/" : "/",
  // Emit files at /assets/<hash>.js rather than /assets/assets/<hash>.js.
  build: { assetsDir: "" },
  resolve: { alias: { $lib: fileURLToPath(new URL("./src/lib", import.meta.url)) } },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": { ...proxied },
      "/archives": { ...proxied },
      "/favicons": { ...proxied },
      "/previews": { ...proxied },
    },
  },
}));
