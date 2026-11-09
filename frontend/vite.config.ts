import { fileURLToPath, URL } from "node:url";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

const apiTarget = process.env.BOOKMARKS_API_TARGET ?? "http://127.0.0.1:8080";
// Preserve the browser-facing Host so the API can validate its Origin.
const proxied = { target: apiTarget, changeOrigin: false };

export default defineConfig({
  plugins: [svelte()],
  // Kit UI ships Svelte/TypeScript source; compile it with the app.
  optimizeDeps: { exclude: ["@kenn-io/kit-ui"] },
  base: "/_/assets/",
  resolve: { alias: { $lib: fileURLToPath(new URL("./src/lib", import.meta.url)) } },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: {
      "/_/api": { ...proxied },
      "/_/archives": { ...proxied },
      "/_/favicons": { ...proxied },
      "/_/previews": { ...proxied },
    },
  },
});
