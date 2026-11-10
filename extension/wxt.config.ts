import { defineConfig } from "wxt";

export default defineConfig({
  modules: ["@wxt-dev/module-svelte"],
  vite: () => ({
    // Kit UI ships Svelte/TypeScript source; compile it with the extension.
    optimizeDeps: { exclude: ["@kenn-io/kit-ui"] },
  }),
  manifest: {
    name: "Bookmarks",
    description: "Save the current page to your self-hosted Bookmarks server.",
    minimum_chrome_version: "120",
    permissions: ["storage", "activeTab", "scripting", "tabs"],
    host_permissions: ["<all_urls>"],
    action: { default_title: "Save to Bookmarks" },
    commands: {
      _execute_action: { suggested_key: { default: "Alt+Shift+B" } },
    },
    content_security_policy: {
      extension_pages:
        "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https: http:; font-src 'self' https://fonts.gstatic.com data:; connect-src *; object-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'",
    },
  },
});
